// sys-procs-go is the TinyGo port of example/plugins/system/sys-procs.
// Same wire output (protojson v2pb.ProcessListResponse), same
// /proc walking strategy, same coverage. Field meanings + wire
// shape documented in the Rust crate's preamble; not duplicated
// here because both versions must stay in lockstep.
//
// Build: tinygo build -target wasi -o sys_procs.wasm .
package main

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/extism/go-pdk"

	platypus "github.com/WangYihang/Platypus/sdk/go/platypus-plugin"
)

const (
	procListCap = 500
	pageSize    = 4096 // x86_64 / arm64 default; 16 KiB hosts under-report
)

type ProcessListRequest struct {
	TopN   uint32 `json:"top_n"`
	SortBy string `json:"sort_by"`
}

// ProcessInfo mirrors v2pb.ProcessInfo's protojson encoding.
type ProcessInfo struct {
	PID           uint32  `json:"pid"`
	PPID          uint32  `json:"ppid"`
	User          string  `json:"user"`
	Name          string  `json:"name"`
	Cmdline       string  `json:"cmdline"`
	Status        string  `json:"status"`
	CPUPercent    float64 `json:"cpuPercent"`
	MemPercent    float64 `json:"memPercent"`
	RSSBytes      uint64  `json:"rssBytes"`
	NumThreads    uint32  `json:"numThreads"`
	CreatedAtUnix int64   `json:"createdAtUnix"`
}

type ProcessListResponse struct {
	Processes  []ProcessInfo `json:"processes"`
	TotalCount uint32        `json:"totalCount"`
	Error      string        `json:"error,omitempty"`
}

//export process_list
func processList() int32 {
	var req ProcessListRequest
	if input := pdk.Input(); len(input) > 0 {
		_ = json.Unmarshal(input, &req)
	}

	pids := listPids()
	total := uint32(len(pids))
	users := readPasswdMap()

	procs := make([]ProcessInfo, 0, len(pids))
	for _, pid := range pids {
		if pi, ok := readOneProcess(pid, users); ok {
			procs = append(procs, pi)
		}
	}

	switch req.SortBy {
	case "pid":
		sort.Slice(procs, func(i, j int) bool { return procs[i].PID < procs[j].PID })
	default:
		// "cpu" (default) or "mem"/"rss": all fall back to RSS desc.
		// cpu_percent can't be computed single-shot, so RSS is the
		// only deterministic ranking we can produce.
		sort.Slice(procs, func(i, j int) bool { return procs[i].RSSBytes > procs[j].RSSBytes })
	}

	topN := req.TopN
	if topN == 0 || topN > procListCap {
		topN = procListCap
	}
	if uint32(len(procs)) > topN {
		procs = procs[:topN]
	}

	body, err := json.Marshal(ProcessListResponse{
		Processes:  procs,
		TotalCount: total,
	})
	if err != nil {
		platypus.LogErrorf("sys-procs-go: marshal: %s", err.Error())
		return 1
	}
	pdk.OutputString(string(body))
	return 0
}

func main() {}

// listPids walks /proc and returns each numeric subdir name as a
// pid. Sorted ascending so the post-sort step has deterministic
// input order.
func listPids() []uint32 {
	entries, err := platypus.HostFSListDir("/proc")
	if err != nil {
		return nil
	}
	out := make([]uint32, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir {
			continue
		}
		v, err := strconv.ParseUint(e.Name, 10, 32)
		if err != nil {
			continue
		}
		out = append(out, uint32(v))
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// readOneProcess pulls /proc/<pid>/stat + status + cmdline + statm
// in one shot. Returns ok=false when the process disappeared between
// listdir and the per-pid reads (the universal /proc walker race).
func readOneProcess(pid uint32, users map[uint32]string) (ProcessInfo, bool) {
	pidStr := strconv.FormatUint(uint64(pid), 10)

	stat, ok := readString("/proc/" + pidStr + "/stat")
	if !ok {
		return ProcessInfo{}, false
	}
	lparen := strings.IndexByte(stat, '(')
	rparen := strings.LastIndexByte(stat, ')')
	if lparen < 0 || rparen < 0 || rparen < lparen {
		return ProcessInfo{}, false
	}
	pidField, err := strconv.ParseUint(strings.TrimSpace(stat[:lparen]), 10, 32)
	if err != nil {
		return ProcessInfo{}, false
	}
	comm := stat[lparen+1 : rparen]
	rest := strings.Fields(strings.TrimSpace(stat[rparen+1:]))
	statusChar := "?"
	if len(rest) >= 1 {
		statusChar = rest[0]
	}
	var ppid uint32
	if len(rest) >= 2 {
		if v, err := strconv.ParseUint(rest[1], 10, 32); err == nil {
			ppid = uint32(v)
		}
	}

	cmdlineRaw, _ := readString("/proc/" + pidStr + "/cmdline")
	cmdline := strings.TrimSpace(strings.ReplaceAll(cmdlineRaw, "\x00", " "))
	if len(cmdline) > 512 {
		// gopsutil truncates at 512 bytes for the wire; mirror that.
		// Cut on byte boundary — the legacy handler does the same.
		cmdline = cmdline[:512]
	}

	var rssBytes uint64
	if s, ok := readString("/proc/" + pidStr + "/statm"); ok {
		fields := strings.Fields(s)
		if len(fields) >= 2 {
			if pages, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
				rssBytes = pages * pageSize
			}
		}
	}

	var numThreads uint32
	var user string
	if status, ok := readString("/proc/" + pidStr + "/status"); ok {
		for _, line := range strings.Split(status, "\n") {
			switch {
			case strings.HasPrefix(line, "Threads:"):
				if v, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(line, "Threads:")), 10, 32); err == nil {
					numThreads = uint32(v)
				}
			case strings.HasPrefix(line, "Uid:"):
				// "Uid: <real> <eff> <saved> <fs>" — use real.
				fields := strings.Fields(strings.TrimPrefix(line, "Uid:"))
				if len(fields) >= 1 {
					if uid, err := strconv.ParseUint(fields[0], 10, 32); err == nil {
						if name, ok := users[uint32(uid)]; ok {
							user = name
						} else {
							user = fields[0]
						}
					}
				}
			}
		}
	}

	return ProcessInfo{
		PID:        uint32(pidField),
		PPID:       ppid,
		User:       user,
		Name:       comm,
		Cmdline:    cmdline,
		Status:     statusChar,
		RSSBytes:   rssBytes,
		NumThreads: numThreads,
	}, true
}

// readPasswdMap parses /etc/passwd into a {uid → username} map.
// One read per process_list call.  Best-effort — a missing
// /etc/passwd (containers with no real userdb) leaves user fields as
// numeric uid strings.
func readPasswdMap() map[uint32]string {
	out := map[uint32]string{}
	s, ok := readString("/etc/passwd")
	if !ok {
		return out
	}
	for _, line := range strings.Split(s, "\n") {
		parts := strings.SplitN(line, ":", 4)
		if len(parts) < 3 {
			continue
		}
		uid, err := strconv.ParseUint(parts[2], 10, 32)
		if err != nil {
			continue
		}
		out[uint32(uid)] = parts[0]
	}
	return out
}

func readString(path string) (string, bool) {
	s, err := platypus.HostFSReadString(path)
	if err != nil {
		return "", false
	}
	return s, true
}
