// sys-info-go is the TinyGo port of example/plugins/system/sys-info.
// Same semantics, same wire output (protojson-shaped SysInfoResponse
// with camelCase keys); only the source language differs.  See the
// Rust crate for the original prose explaining v2 coverage and the
// "sample missing fields as zero" policy that lets v3 fill them
// later without a wire break.
//
// Build:  tinygo build -target wasi -o sys_info_plugin.wasm .
// Stage:  go run ./hack/stage_system_plugins  (from repo root)
package main

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode"

	"github.com/extism/go-pdk"

	platypus "github.com/WangYihang/Platypus/sdk/go/platypus-plugin"
)

// SysInfoResponse mirrors the protojson encoding of v2pb.SysInfoResponse.
// `omitempty` drops fields the plugin couldn't fill so the bridge's
// protojson.Unmarshal sees them as the proto default — the agent UI
// already renders missing as "—".
type SysInfoResponse struct {
	OS              string  `json:"os,omitempty"`
	Arch            string  `json:"arch,omitempty"`
	Hostname        string  `json:"hostname,omitempty"`
	KernelVersion   string  `json:"kernelVersion,omitempty"`
	MemTotal        uint64  `json:"memTotal,omitempty"`
	MemUsed         uint64  `json:"memUsed,omitempty"`
	MemAvailable    uint64  `json:"memAvailable,omitempty"`
	MemFree         uint64  `json:"memFree,omitempty"`
	SwapTotal       uint64  `json:"swapTotal,omitempty"`
	SwapUsed        uint64  `json:"swapUsed,omitempty"`
	Platform        string  `json:"platform,omitempty"`
	PlatformFamily  string  `json:"platformFamily,omitempty"`
	PlatformVersion string  `json:"platformVersion,omitempty"`
	NumCPU          uint32  `json:"numCpu,omitempty"`
	CPUModel        string  `json:"cpuModel,omitempty"`
	CPUMhz          float64 `json:"cpuMhz,omitempty"`
	BootTimeUnix    uint64  `json:"bootTimeUnix,omitempty"`
	UptimeSeconds   uint64  `json:"uptimeSeconds,omitempty"`
	Load1           float64 `json:"load1,omitempty"`
	Load5           float64 `json:"load5,omitempty"`
	Load15          float64 `json:"load15,omitempty"`
	ProcessCount    uint32  `json:"processCount,omitempty"`
	MachineID       string  `json:"machineId,omitempty"`
	Timezone        string  `json:"timezone,omitempty"`
	SampledAtUnix   int64   `json:"sampledAtUnix,omitempty"`
	Error           string  `json:"error,omitempty"`
}

// sys_info is the wasm export the agent invokes for
// PluginCallRequest{method="sys_info"}.  Returns the JSON body as
// pdk.OutputString so the agent's bridge unmarshals it via
// protojson.Unmarshal into v2pb.SysInfoResponse.
//
//export sys_info
func sysInfo() int32 {
	resp := SysInfoResponse{}

	if u, err := platypus.HostUname(); err == nil {
		resp.OS = u.OS
		resp.Arch = u.Arch
	}

	if s, ok := readTrim("/etc/hostname"); ok {
		resp.Hostname = s
	} else if s, ok := readTrim("/proc/sys/kernel/hostname"); ok {
		resp.Hostname = s
	}

	if s, ok := readTrim("/proc/sys/kernel/osrelease"); ok {
		resp.KernelVersion = s
	} else if s, ok := readTrim("/proc/version"); ok {
		// Fallback: extract third whitespace-delimited word (like
		// the Rust crate does — "Linux version 5.15.0 ..." → "5.15.0").
		fields := strings.Fields(s)
		if len(fields) >= 3 {
			resp.KernelVersion = fields[2]
		}
	}

	if s, ok := readTrim("/etc/machine-id"); ok {
		resp.MachineID = s
	}
	if s, ok := readTrim("/etc/timezone"); ok {
		resp.Timezone = s
	}

	if s, ok := readString("/etc/os-release"); ok {
		id, idLike, versionID := parseOSRelease(s)
		resp.Platform = id
		resp.PlatformFamily = idLike
		resp.PlatformVersion = versionID
	}

	if s, ok := readString("/proc/meminfo"); ok {
		fillMemInfo(&resp, s)
	}

	if s, ok := readString("/proc/uptime"); ok {
		resp.UptimeSeconds = parseUptime(s)
	}

	if s, ok := readString("/proc/loadavg"); ok {
		resp.Load1, resp.Load5, resp.Load15 = parseLoadavg(s)
	}

	if s, ok := readString("/proc/cpuinfo"); ok {
		fillCPUInfo(&resp, s)
	}

	if entries, err := platypus.HostFSListDir("/proc"); err == nil {
		var n uint32
		for _, e := range entries {
			if !e.IsDir {
				continue
			}
			if isAllDigits(e.Name) {
				n++
			}
		}
		resp.ProcessCount = n
	}

	body, err := json.Marshal(resp)
	if err != nil {
		platypus.LogErrorf("sys-info-go: marshal: %s", err.Error())
		return 1
	}
	pdk.OutputString(string(body))
	return 0
}

func main() {}

// ---- /proc helpers ----------------------------------------------

// readString returns the file body. The bool is false on any host_fs
// error (denied / missing / etc).
func readString(path string) (string, bool) {
	s, err := platypus.HostFSReadString(path)
	if err != nil {
		return "", false
	}
	return s, true
}

func readTrim(path string) (string, bool) {
	s, ok := readString(path)
	if !ok {
		return "", false
	}
	t := strings.TrimSpace(s)
	if t == "" {
		return "", false
	}
	return t, true
}

// fillMemInfo fans out /proc/meminfo into the response.  Same
// derivation as the Rust crate: MemUsed = MemTotal - MemAvailable
// (gopsutil convention), SwapUsed = SwapTotal - SwapFree.
func fillMemInfo(resp *SysInfoResponse, content string) {
	var swapFree uint64
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		v, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		bytes := v * 1024
		switch fields[0] {
		case "MemTotal:":
			resp.MemTotal = bytes
		case "MemFree:":
			resp.MemFree = bytes
		case "MemAvailable:":
			resp.MemAvailable = bytes
		case "SwapTotal:":
			resp.SwapTotal = bytes
		case "SwapFree:":
			swapFree = bytes
		}
	}
	switch {
	case resp.MemTotal > 0 && resp.MemAvailable > 0:
		resp.MemUsed = resp.MemTotal - resp.MemAvailable
	case resp.MemTotal > 0 && resp.MemFree > 0:
		resp.MemUsed = resp.MemTotal - resp.MemFree
	}
	if resp.SwapTotal > swapFree {
		resp.SwapUsed = resp.SwapTotal - swapFree
	}
}

// fillCPUInfo populates NumCPU + CPUModel + CPUMhz from /proc/cpuinfo.
// Counts "processor" lines (logical cores) and grabs the first
// "model name" / "cpu MHz" row (assumes uniform cores; matches the
// Rust crate's behaviour).
func fillCPUInfo(resp *SysInfoResponse, content string) {
	var n uint32
	for _, line := range strings.Split(content, "\n") {
		switch {
		case strings.HasPrefix(line, "processor"):
			n++
		case resp.CPUModel == "" && strings.HasPrefix(line, "model name"):
			if _, v, ok := splitOnce(line, ":"); ok {
				resp.CPUModel = strings.TrimSpace(v)
			}
		case resp.CPUMhz == 0 && strings.HasPrefix(line, "cpu MHz"):
			if _, v, ok := splitOnce(line, ":"); ok {
				if mhz, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
					resp.CPUMhz = mhz
				}
			}
		}
	}
	resp.NumCPU = n
}

// parseOSRelease extracts (ID, ID_LIKE, VERSION_ID) from
// /etc/os-release content.  Lines look like `KEY=value` or
// `KEY="value"`; missing keys come back as empty strings.
func parseOSRelease(content string) (id, idLike, versionID string) {
	for _, line := range strings.Split(content, "\n") {
		k, v, ok := splitOnce(line, "=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"`)
		switch k {
		case "ID":
			id = v
		case "ID_LIKE":
			idLike = v
		case "VERSION_ID":
			versionID = v
		}
	}
	return id, idLike, versionID
}

// parseUptime returns the integer-second count from /proc/uptime
// content ("<float-seconds> <idle-float-seconds>\n"). 0 on
// malformed input.
func parseUptime(content string) uint64 {
	fields := strings.Fields(content)
	if len(fields) == 0 {
		return 0
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || secs <= 0 {
		return 0
	}
	return uint64(secs)
}

// parseLoadavg returns (load1, load5, load15) from /proc/loadavg
// content.
func parseLoadavg(content string) (l1, l5, l15 float64) {
	fields := strings.Fields(content)
	if len(fields) > 0 {
		l1, _ = strconv.ParseFloat(fields[0], 64)
	}
	if len(fields) > 1 {
		l5, _ = strconv.ParseFloat(fields[1], 64)
	}
	if len(fields) > 2 {
		l15, _ = strconv.ParseFloat(fields[2], 64)
	}
	return l1, l5, l15
}

// splitOnce splits s into (head, tail, true) at the first occurrence
// of sep; (s, "", false) if sep isn't found.
func splitOnce(s, sep string) (string, string, bool) {
	i := strings.Index(s, sep)
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+len(sep):], true
}

// isAllDigits reports whether s is non-empty and contains only
// ASCII digits — used to filter /proc/<pid> directory names.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
