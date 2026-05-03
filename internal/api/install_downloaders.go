package api

import "strings"

// downloader is one row in the registry of bootstrap one-liners we
// hand admins on the enrollment wizard's RunStep. Each entry knows
// its OS family (so the wizard can filter the dropdown by target_os)
// and a `render` function that produces the full one-liner for either
// the `script` or `bundle` install shape, with or without TLS skip.
//
// One entry powers FOUR cells per downloader (script / bundle ×
// insecure / strict) — the FE used to compose the bundle shape
// itself, which meant the per-tool insecure-flag conventions lived
// in two places (BE registry + FE bundleOneLinerFor) and could
// drift. We collapsed that by moving every clipboard-bound shape
// here so adding a new downloader is one entry that powers
// install_commands + install_commands_strict + bundle_commands +
// bundle_commands_strict.
type downloader struct {
	name     string
	osFamily string // "unix" | "windows"
	render   func(url string, opts renderOpts) string
}

// renderOpts collapses the {script, bundle} × {insecure, strict}
// matrix into two flags. Each downloader handles all four corners
// of that grid in one function.
type renderOpts struct {
	// asBundle=true wraps the fetch in `platypus-agent "$(...)"` (or
	// PowerShell equivalent) so the resulting pinst_ token is fed to
	// the agent CLI. asBundle=false pipes the fetch into `| sh` /
	// `| iex` for direct shell execution.
	asBundle bool
	// insecure=true emits the per-tool skip-cert flavour
	// (-k / --no-check-certificate / ServerCertificateValidationCallback).
	// false omits those flags for prod servers with a real cert.
	insecure bool
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

// renderInstallCommandsFor returns the per-downloader install
// commands for the given target OS in BOTH flavours: insecure
// (skip-cert-verify, default for self-signed) and strict (relies on
// the system trust store, for prod servers with a real cert). Each
// map is keyed by downloader name; the defaults are the family's
// first entry in the matching flavour. Caller wires the
// download_tls=insecure marker onto the insecure URL so the inner
// script honours the same TLS trust mode (see renderInstallCommands
// in handler_install_tokens_v1.go).
func renderInstallCommandsFor(url, targetOS string) (
	insecureCommands, strictCommands map[string]string,
	insecureDefault, strictDefault string,
) {
	return renderCommandsForURLs(url, url, targetOS, false)
}

// renderBundleCommandsFor is the bundle-shape sibling: same registry,
// same flavours, but the rendered command runs `platypus-agent` on
// the fetched pinst_ token instead of piping the body to a shell.
// Bundle URLs use the same value for both flavours — the agent CLI
// handles trust independently and ignores skip_verify.
func renderBundleCommandsFor(url, targetOS string) (
	insecureCommands, strictCommands map[string]string,
	insecureDefault, strictDefault string,
) {
	return renderCommandsForURLs(url, url, targetOS, true)
}

// renderCommandsForURLs is the shared driver. Walks the registry
// filtered by OS family, runs each entry's render twice (insecure +
// strict) against potentially-distinct URLs, records the first
// family entry's render as the family default.
func renderCommandsForURLs(insecureURL, strictURL, targetOS string, asBundle bool) (
	insecure, strict map[string]string,
	insecureDefault, strictDefault string,
) {
	family := downloaderOSFamily(targetOS)
	insecure = make(map[string]string, 4)
	strict = make(map[string]string, 4)
	for _, d := range installDownloaders {
		if d.osFamily != family {
			continue
		}
		insecureCmd := d.render(insecureURL, renderOpts{asBundle: asBundle, insecure: true})
		strictCmd := d.render(strictURL, renderOpts{asBundle: asBundle, insecure: false})
		insecure[d.name] = insecureCmd
		strict[d.name] = strictCmd
		if insecureDefault == "" {
			insecureDefault = insecureCmd
			strictDefault = strictCmd
		}
	}
	return insecure, strict, insecureDefault, strictDefault
}

// --- per-tool render helpers ---
//
// Pattern for unix tools: build the bare "fetch + print to stdout"
// command, then wrap it in `| sh` (script) or `platypus-agent "$(...)"`
// (bundle). Pattern for PowerShell tools: build the inner Command
// string with the SecurityProtocol force + (when insecure) the cert
// callback, then either pipe to `| iex` or substitute into a
// platypus-agent.exe invocation.

func renderCurl(url string, opts renderOpts) string {
	// --tlsv1.2 sidesteps macOS LibreSSL curl's TLS 1.0 ClientHello
	// downgrade on strict-1.2 servers regardless of trust mode; -k
	// is added only when the operator opted into skip-verify.
	flag := ""
	if opts.insecure {
		flag = "-k "
	}
	fetch := "curl -fsSL --tlsv1.2 " + flag + url
	return wrapUnix(fetch, opts.asBundle)
}

func renderWget(url string, opts renderOpts) string {
	flag := ""
	if opts.insecure {
		flag = "--no-check-certificate "
	}
	fetch := "wget -qO- " + flag + url
	return wrapUnix(fetch, opts.asBundle)
}

func renderPython3(url string, opts renderOpts) string {
	// Single-quoted URL to keep shell-escaping minimal — the install
	// URL is base32+token-safe so no embedded quotes can leak through.
	var fetch string
	if opts.insecure {
		fetch = "python3 -c \"import ssl,urllib.request as u;print(u.urlopen('" +
			url + "',context=ssl._create_unverified_context()).read().decode())\""
	} else {
		fetch = "python3 -c \"import urllib.request as u;print(u.urlopen('" +
			url + "').read().decode())\""
	}
	return wrapUnix(fetch, opts.asBundle)
}

func renderPHP(url string, opts renderOpts) string {
	var fetch string
	if opts.insecure {
		fetch = "php -r \"echo file_get_contents('" + url +
			"',false,stream_context_create(['ssl'=>['verify_peer'=>false,'verify_peer_name'=>false]]));\""
	} else {
		fetch = "php -r \"echo file_get_contents('" + url + "');\""
	}
	return wrapUnix(fetch, opts.asBundle)
}

func renderRuby(url string, opts renderOpts) string {
	var fetch string
	if opts.insecure {
		fetch = "ruby -ropen-uri -e \"puts URI.open('" + url +
			"',ssl_verify_mode: 0).read\""
	} else {
		fetch = "ruby -ropen-uri -e \"puts URI.open('" + url + "').read\""
	}
	return wrapUnix(fetch, opts.asBundle)
}

func renderPowerShell(url string, opts renderOpts) string {
	// Windows PowerShell 5.1 defaults SecurityProtocol to Ssl3|Tls
	// (TLS 1.0), which the ingress listener rejects (MinVersion=
	// TLS 1.2). Force Tls12 BEFORE the request regardless of trust
	// mode so the protocol layer succeeds; ServerCertificateValidationCallback
	// is added only when skipping verification.
	setup := "[Net.ServicePointManager]::SecurityProtocol=[Net.SecurityProtocolType]::Tls12;"
	if opts.insecure {
		setup += "[Net.ServicePointManager]::ServerCertificateValidationCallback={$true};"
	}
	if opts.asBundle {
		// Invoke-RestMethod returns the body as a string; parens make
		// it a positional arg to platypus-agent.exe. The whole thing
		// runs in a script block so the SecurityProtocol override is
		// scoped to this invocation only (defensive — we don't want
		// to leave the runspace's TLS state mutated).
		body := setup + "& platypus-agent.exe (Invoke-RestMethod -UseBasicParsing -Uri '" + url + "')"
		return `powershell -ExecutionPolicy Bypass -Command "& { ` + body + ` }"`
	}
	body := setup + "iwr -useb '" + url + "' | iex"
	return `powershell -ExecutionPolicy Bypass -Command "` + body + `"`
}

func renderPwsh(url string, opts renderOpts) string {
	// pwsh (PowerShell 7+) inherits system-default protocols (TLS
	// 1.2/1.3) on every supported Windows; the SecurityProtocol
	// force is defensive for hosts that disabled 1.2 at the OS
	// level. -SkipCertificateCheck is the PS 7+ replacement for the
	// callback hack.
	setup := "[Net.ServicePointManager]::SecurityProtocol=[Net.SecurityProtocolType]::Tls12;"
	skip := ""
	if opts.insecure {
		skip = "-SkipCertificateCheck "
	}
	if opts.asBundle {
		body := setup + "& platypus-agent.exe (Invoke-RestMethod " + skip + "-UseBasicParsing -Uri '" + url + "')"
		return `pwsh -ExecutionPolicy Bypass -Command "& { ` + body + ` }"`
	}
	body := setup + "iwr -useb " + skip + "'" + url + "' | iex"
	return `pwsh -ExecutionPolicy Bypass -Command "` + body + `"`
}

// wrapUnix takes a bare "fetch URL → stdout" command and wraps it in
// either `| sh` (script) or `platypus-agent "$(...)"` (bundle).
func wrapUnix(fetch string, asBundle bool) string {
	if asBundle {
		return `platypus-agent "$(` + fetch + `)"`
	}
	return fetch + " | sh"
}
