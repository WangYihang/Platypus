package api

import "strings"

// downloader is one row in the registry of bootstrap one-liners we
// hand admins on the enrollment wizard's RunStep. Each entry knows
// its OS family (so the wizard can filter the dropdown by target_os)
// and how to wrap a fully-qualified install URL into a tool-specific
// "fetch + pipe to shell" command.
//
// The render fn takes an `insecure` bool: true emits the
// skip-cert-verification flavour (for self-signed servers, the
// default for first-boot deployments), false emits the strict
// flavour that relies on the host's system trust store. Both
// flavours are pre-computed server-side and shipped together in the
// install token response so the wizard can toggle between them
// without re-issuing the (single-use) install token.
//
// Why we keep this server-side instead of letting the FE templatise
// the URL itself: the install URL ships through `install_command` /
// `install_commands` fields in the issue-install response, which
// admins copy verbatim. Generating the strings on the server keeps
// every shape that gets written to a clipboard reviewable in one
// place — adding a new downloader is a single registry entry plus a
// table-driven test row.
type downloader struct {
	name     string
	osFamily string // "unix" | "windows"
	render   func(url string, insecure bool) string
}

const (
	osFamilyUnix    = "unix"
	osFamilyWindows = "windows"
)

// installDownloaders is the v1 set we ship in the wizard. The order
// here drives the dropdown order; defaults (curl on unix, powershell
// on windows) come first within their family so the operator's
// muscle-memory "just paste it" flow stays unchanged.
var installDownloaders = []downloader{
	// --- unix ---
	{name: "curl", osFamily: osFamilyUnix, render: renderCurl},
	{name: "wget", osFamily: osFamilyUnix, render: renderWget},
	{name: "python3", osFamily: osFamilyUnix, render: renderPython3},
	{name: "php", osFamily: osFamilyUnix, render: renderPHP},
	{name: "ruby", osFamily: osFamilyUnix, render: renderRuby},

	// --- windows ---
	// The legacy Windows-PowerShell variant pokes
	// ServerCertificateValidationCallback because Windows PS 5.1's
	// Invoke-WebRequest has no -SkipCertificateCheck flag (added in
	// PS Core 6+); the callback override is the standard workaround.
	{name: "powershell", osFamily: osFamilyWindows, render: renderPowerShell},
	{name: "pwsh", osFamily: osFamilyWindows, render: renderPwsh},
}

// downloaderOSFamily maps GOOS-ish target_os strings to the family
// the registry filters by. Empty / unrecognised → unix (the wizard's
// "skip OS picker" path falls here, and the unix script the
// distributor serves is the broader default anyway).
func downloaderOSFamily(targetOS string) string {
	if strings.EqualFold(targetOS, "windows") {
		return osFamilyWindows
	}
	return osFamilyUnix
}

// renderInstallCommandsFor returns the per-downloader bootstrap
// commands for the given target OS in BOTH flavours: insecure
// (skip-cert-verify, default for self-signed) and strict (relies on
// the system trust store, for prod servers with a real cert). Each
// map is keyed by downloader name; the defaults are the family's
// first entry in the matching flavour.
func renderInstallCommandsFor(url, targetOS string) (
	insecureCommands, strictCommands map[string]string,
	insecureDefault, strictDefault string,
) {
	family := downloaderOSFamily(targetOS)
	insecureCommands = make(map[string]string, 4)
	strictCommands = make(map[string]string, 4)
	for _, d := range installDownloaders {
		if d.osFamily != family {
			continue
		}
		insecureCmd := d.render(url, true)
		strictCmd := d.render(url, false)
		insecureCommands[d.name] = insecureCmd
		strictCommands[d.name] = strictCmd
		if insecureDefault == "" {
			// First entry of the family wins — see the ordering
			// rationale in the installDownloaders comment.
			insecureDefault = insecureCmd
			strictDefault = strictCmd
		}
	}
	return insecureCommands, strictCommands, insecureDefault, strictDefault
}

// --- per-tool render helpers ---

func renderCurl(url string, insecure bool) string {
	// --tlsv1.2 sidesteps macOS LibreSSL curl's TLS 1.0 ClientHello
	// downgrade on strict-1.2 servers regardless of trust mode; -k
	// is added only when the operator opted into skip-verify.
	flag := ""
	if insecure {
		flag = "-k "
	}
	return "curl -fsSL --tlsv1.2 " + flag + url + " | sh"
}

func renderWget(url string, insecure bool) string {
	flag := ""
	if insecure {
		flag = "--no-check-certificate "
	}
	return "wget -qO- " + flag + url + " | sh"
}

func renderPython3(url string, insecure bool) string {
	// Single-quoted URL to keep shell-escaping minimal — the install
	// URL is base32+token-safe so no embedded quotes can leak through.
	if insecure {
		return "python3 -c \"import ssl,urllib.request as u;print(u.urlopen('" +
			url + "',context=ssl._create_unverified_context()).read().decode())\" | sh"
	}
	return "python3 -c \"import urllib.request as u;print(u.urlopen('" +
		url + "').read().decode())\" | sh"
}

func renderPHP(url string, insecure bool) string {
	if insecure {
		return "php -r \"echo file_get_contents('" + url +
			"',false,stream_context_create(['ssl'=>['verify_peer'=>false,'verify_peer_name'=>false]]));\" | sh"
	}
	return "php -r \"echo file_get_contents('" + url + "');\" | sh"
}

func renderRuby(url string, insecure bool) string {
	if insecure {
		return "ruby -ropen-uri -e \"puts URI.open('" + url +
			"',ssl_verify_mode: 0).read\" | sh"
	}
	return "ruby -ropen-uri -e \"puts URI.open('" + url + "').read\" | sh"
}

func renderPowerShell(url string, insecure bool) string {
	// Windows PowerShell 5.1 defaults SecurityProtocol to Ssl3|Tls
	// (TLS 1.0), which the ingress listener rejects (MinVersion=
	// TLS 1.2). Force Tls12 BEFORE iwr regardless of trust mode so
	// the protocol layer succeeds; ServerCertificateValidationCallback
	// is added only when skipping verification.
	tlsForce := "[Net.ServicePointManager]::SecurityProtocol=[Net.SecurityProtocolType]::Tls12;"
	skip := ""
	if insecure {
		skip = "[Net.ServicePointManager]::ServerCertificateValidationCallback={$true};"
	}
	return `powershell -ExecutionPolicy Bypass -Command "` + tlsForce + skip +
		`iwr -useb '` + url + `' | iex"`
}

func renderPwsh(url string, insecure bool) string {
	// pwsh (PowerShell 7+) inherits system-default protocols (TLS
	// 1.2/1.3) on every supported Windows; the SecurityProtocol
	// force is defensive for hosts that disabled 1.2 at the OS
	// level. -SkipCertificateCheck is the PS 7+ replacement for the
	// callback hack.
	tlsForce := "[Net.ServicePointManager]::SecurityProtocol=[Net.SecurityProtocolType]::Tls12;"
	skip := ""
	if insecure {
		skip = "-SkipCertificateCheck "
	}
	return `pwsh -ExecutionPolicy Bypass -Command "` + tlsForce +
		`iwr -useb ` + skip + `'` + url + `' | iex"`
}
