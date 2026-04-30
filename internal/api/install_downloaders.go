package api

import "strings"

// downloader is one row in the registry of bootstrap one-liners we
// hand admins on the enrollment wizard's RunStep. Each entry knows
// its OS family (so the wizard can filter the dropdown by target_os)
// and how to wrap a fully-qualified install URL into a tool-specific
// "fetch + pipe to shell" command.
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
	render   func(url string) string
}

const (
	osFamilyUnix    = "unix"
	osFamilyWindows = "windows"
)

// installDownloaders is the v1 set we ship in the wizard. The order
// here drives the dropdown order; defaults (curl on unix, powershell
// on windows) come first within their family so the operator's
// muscle-memory "just paste it" flow stays unchanged.
//
// All templates include a per-tool "skip TLS verification" flag
// because the install endpoint may be self-signed on first-boot
// deployments — the same self-signed cert that motivated the macOS
// LibreSSL workaround. Tools that pin via CA cert at this stage
// would need the CA bytes server-side; we leave that to the inner
// script (which already gets PLATYPUS_PROJECT_CA stamped in) and
// keep the bootstrap one-liner tool-agnostic about trust.
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

// renderInstallCommandsFor returns every downloader command for the
// given target OS, keyed by downloader name. The default for the
// family is also returned as a separate string so the legacy
// install_command field stays populated.
func renderInstallCommandsFor(url, targetOS string) (commands map[string]string, defaultCmd string) {
	family := downloaderOSFamily(targetOS)
	commands = make(map[string]string, 4)
	for _, d := range installDownloaders {
		if d.osFamily != family {
			continue
		}
		cmd := d.render(url)
		commands[d.name] = cmd
		if defaultCmd == "" {
			// First entry of the family wins — see the ordering
			// rationale in the installDownloaders comment.
			defaultCmd = cmd
		}
	}
	return commands, defaultCmd
}

// --- per-tool render helpers ---

func renderCurl(url string) string {
	// --tlsv1.2 sidesteps macOS LibreSSL curl's TLS 1.0 ClientHello
	// downgrade on strict-1.2 servers; -k skips cert verification.
	return "curl -fsSL --tlsv1.2 -k " + url + " | sh"
}

func renderWget(url string) string {
	return "wget -qO- --no-check-certificate " + url + " | sh"
}

func renderPython3(url string) string {
	// Single-quoted to keep shell-escaping minimal; the install URL
	// is base32+token-safe so no embedded quotes can leak through.
	return "python3 -c \"import ssl,urllib.request as u;print(u.urlopen('" +
		url + "',context=ssl._create_unverified_context()).read().decode())\" | sh"
}

func renderPHP(url string) string {
	return "php -r \"echo file_get_contents('" + url +
		"',false,stream_context_create(['ssl'=>['verify_peer'=>false,'verify_peer_name'=>false]]));\" | sh"
}

func renderRuby(url string) string {
	return "ruby -ropen-uri -e \"puts URI.open('" + url +
		"',ssl_verify_mode: 0).read\" | sh"
}

func renderPowerShell(url string) string {
	return `powershell -ExecutionPolicy Bypass -Command "[Net.ServicePointManager]::ServerCertificateValidationCallback={$true};iwr -useb '` +
		url + `' | iex"`
}

func renderPwsh(url string) string {
	return `pwsh -ExecutionPolicy Bypass -Command "iwr -useb -SkipCertificateCheck '` +
		url + `' | iex"`
}
