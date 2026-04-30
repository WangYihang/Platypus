package agent

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/user"
	"runtime"
	"strings"
	"time"

	cpuinfo "github.com/shirou/gopsutil/v4/cpu"
	diskinfo "github.com/shirou/gopsutil/v4/disk"
	hostinfo "github.com/shirou/gopsutil/v4/host"
	loadinfo "github.com/shirou/gopsutil/v4/load"
	meminfo "github.com/shirou/gopsutil/v4/mem"
	procinfo "github.com/shirou/gopsutil/v4/process"

	"github.com/WangYihang/Platypus/internal/link"
	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
	"github.com/WangYihang/Platypus/pkg/version"
)

// CollectSysInfo gathers a rich snapshot of the agent host: OS /
// kernel / platform, hardware totals, live CPU / memory / disk,
// network interfaces, logged-in users, etc. All gopsutil calls are
// wrapped so a single probe failure doesn't blank the whole payload
// — missing fields leave the proto defaults in place.
//
// The function is deliberately best-effort: on locked-down hosts
// where /proc is masked or a given syscall is denied we keep going
// and surface whatever *is* available. The caller uses the result
// both at enrollment (one-shot, warm) and via the SysInfo RPC
// (on-demand refresh from the Web UI).
func CollectSysInfo(ctx context.Context) *v2pb.SysInfoResponse {
	resp := &v2pb.SysInfoResponse{
		Os:                runtime.GOOS,
		Arch:              runtime.GOARCH,
		NumCpu:            uint32(runtime.NumCPU()),
		NetworkInterfaces: map[string]string{},
		SampledAtUnix:     time.Now().Unix(),
		// Build identity travels on every refresh so server-side host
		// rows stay accurate after a self-upgrade swap. Sourced from
		// pkg/version (ldflags-injected at release time).
		BuildVersion:    version.Version,
		BuildCommit:     version.Commit,
		BuildDate:       version.Date,
		ProtocolVersion: link.ProtocolVersion,
	}

	if hn, err := os.Hostname(); err == nil {
		resp.Hostname = hn
	}
	if tz, _ := time.Now().Zone(); tz != "" {
		resp.Timezone = tz
	}
	if u, err := user.Current(); err == nil {
		resp.CurrentUser = u.Username
	}

	var hostStat *hostinfo.InfoStat
	if info, err := hostinfo.InfoWithContext(ctx); err == nil && info != nil {
		hostStat = info
		if info.Hostname != "" {
			resp.Hostname = info.Hostname
		}
		resp.KernelVersion = info.KernelVersion
		resp.Platform = info.Platform
		resp.PlatformFamily = info.PlatformFamily
		resp.PlatformVersion = info.PlatformVersion
		resp.Virtualization = info.VirtualizationSystem
		resp.BootTimeUnix = info.BootTime
		resp.UptimeSeconds = info.Uptime
		resp.ProcessCount = uint32(info.Procs)
		if info.HostID != "" {
			resp.MachineId = info.HostID
		}
	}

	if cpus, err := cpuinfo.InfoWithContext(ctx); err == nil && len(cpus) > 0 {
		resp.CpuModel = cpus[0].ModelName
		resp.CpuMhz = cpus[0].Mhz
	}
	if n, err := cpuinfo.CountsWithContext(ctx, false); err == nil {
		resp.NumCpuPhysical = uint32(n)
	}
	// Short sample so we don't block the caller for a full second on
	// every snapshot; 200ms is enough to get a reasonable %.
	if percents, err := cpuinfo.PercentWithContext(ctx, 200*time.Millisecond, false); err == nil && len(percents) > 0 {
		resp.CpuPercent = percents[0]
	}

	if vm, err := meminfo.VirtualMemoryWithContext(ctx); err == nil && vm != nil {
		resp.MemTotal = vm.Total
		resp.MemUsed = vm.Used
		resp.MemFree = vm.Free
		resp.MemAvailable = vm.Available
	}
	if sw, err := meminfo.SwapMemoryWithContext(ctx); err == nil && sw != nil {
		resp.SwapTotal = sw.Total
		resp.SwapUsed = sw.Used
	}

	if la, err := loadinfo.AvgWithContext(ctx); err == nil && la != nil {
		resp.Load1 = la.Load1
		resp.Load5 = la.Load5
		resp.Load15 = la.Load15
	}

	collectDisks(ctx, resp)
	collectInterfaces(resp)

	if procs, err := procinfo.ProcessesWithContext(ctx); err == nil {
		// Override gopsutil host.Info Procs when we got a cleaner read.
		resp.ProcessCount = uint32(len(procs))
	}

	if users, err := hostinfo.UsersWithContext(ctx); err == nil {
		for _, u := range users {
			resp.Users = append(resp.Users, &v2pb.UserSession{
				User:      u.User,
				Terminal:  u.Terminal,
				Host:      u.Host,
				StartedAt: int64(u.Started),
			})
		}
	}

	resp.DefaultGateway = detectDefaultGateway()
	resp.PublicIpv4, resp.PublicIpv6 = detectPublicIPs(ctx)
	// Backfill the legacy single-family field so a server running an
	// older build still gets something. Prefer v4 (current schema's
	// previous behaviour); fall through to v6.
	if resp.PublicIpv4 != "" {
		resp.PublicIp = resp.PublicIpv4
	} else {
		resp.PublicIp = resp.PublicIpv6
	}

	// Hardware / chassis / machine classification. Pulled after the
	// gopsutil host probe so we can reuse its VirtualizationSystem
	// field without a second read.
	mach := collectMachineSnapshot(hostStat)
	resp.MachineType = mach.MachineType
	resp.ContainerRuntime = mach.ContainerRuntime
	resp.ChassisType = mach.ChassisType
	resp.ProductVendor = mach.ProductVendor
	resp.ProductName = mach.ProductName
	resp.BiosVendor = mach.BIOSVendor
	resp.BiosVersion = mach.BIOSVersion

	// GPU enumeration is opt-in via ghw; nvidia-smi runtime stats
	// are added best-effort on top. Deliberately *not* blocking on
	// the 1s nvidia-smi timeout — collectGPUs already caps it.
	resp.Gpus = collectGPUs(ctx)

	return resp
}

// collectDisks fills both the aggregate (disk_total / disk_used) and
// per-partition detail. We only consider "real" partitions — gopsutil
// returns every mount, including docker overlays and tmpfs; those
// inflate totals and clutter the UI so we filter by fstype.
func collectDisks(ctx context.Context, resp *v2pb.SysInfoResponse) {
	parts, err := diskinfo.PartitionsWithContext(ctx, false)
	if err != nil {
		return
	}
	var totalAll, usedAll uint64
	for _, p := range parts {
		if skipFsType(p.Fstype) {
			continue
		}
		usage, err := diskinfo.UsageWithContext(ctx, p.Mountpoint)
		if err != nil || usage == nil {
			continue
		}
		totalAll += usage.Total
		usedAll += usage.Used
		resp.Disks = append(resp.Disks, &v2pb.DiskPartition{
			Device:     p.Device,
			Mountpoint: p.Mountpoint,
			Fstype:     usage.Fstype,
			TotalBytes: usage.Total,
			UsedBytes:  usage.Used,
		})
	}
	resp.DiskTotal = totalAll
	resp.DiskUsed = usedAll
}

// skipFsType hides pseudo / virtual filesystems that would otherwise
// pollute the disk report. The list tracks the common ones across
// Linux / macOS / Windows.
func skipFsType(fs string) bool {
	switch strings.ToLower(fs) {
	case "tmpfs", "devtmpfs", "overlay", "overlayfs", "proc", "sysfs",
		"cgroup", "cgroup2", "pstore", "bpf", "debugfs", "tracefs",
		"fusectl", "securityfs", "autofs", "binfmt_misc", "configfs",
		"devpts", "mqueue", "hugetlbfs", "rpc_pipefs", "ramfs",
		"squashfs", "nsfs":
		return true
	}
	return false
}

// collectInterfaces enumerates NICs via the stdlib (always available;
// no CGO). The legacy `NetworkInterfaces` map stays populated for
// older server / UI builds that don't yet read the `interfaces`
// repeated field.
func collectInterfaces(resp *v2pb.SysInfoResponse) {
	ifs, err := net.Interfaces()
	if err != nil {
		return
	}
	for _, ifi := range ifs {
		addrs, _ := ifi.Addrs()
		cidrs := make([]string, 0, len(addrs))
		for _, a := range addrs {
			cidrs = append(cidrs, a.String())
		}
		isUp := ifi.Flags&net.FlagUp != 0
		isLoop := ifi.Flags&net.FlagLoopback != 0
		resp.Interfaces = append(resp.Interfaces, &v2pb.NetworkInterface{
			Name:       ifi.Name,
			Mac:        ifi.HardwareAddr.String(),
			Addrs:      cidrs,
			Flags:      ifi.Flags.String(),
			Mtu:        uint64(ifi.MTU),
			IsUp:       isUp,
			IsLoopback: isLoop,
		})
		if len(cidrs) > 0 {
			resp.NetworkInterfaces[ifi.Name] = strings.Join(cidrs, ",")
		}
	}
	if ip, mac := detectPrimary(); ip != "" {
		resp.PrimaryIp = ip
		resp.PrimaryMac = mac
	}
}

// detectPrimary picks the interface the kernel would use for an
// outbound connection. We do a UDP "dial" to a non-routable target
// — no packets are sent, but the socket's local address is resolved
// against the routing table.
func detectPrimary() (ip string, mac string) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "", ""
	}
	defer func() { _ = conn.Close() }()
	local, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || local == nil {
		return "", ""
	}
	ip = local.IP.String()

	// Walk interfaces and match by assigned address to recover the MAC.
	ifs, err := net.Interfaces()
	if err != nil {
		return ip, ""
	}
	for _, ifi := range ifs {
		addrs, _ := ifi.Addrs()
		for _, a := range addrs {
			if in, ok := a.(*net.IPNet); ok && in.IP.Equal(local.IP) {
				return ip, ifi.HardwareAddr.String()
			}
		}
	}
	return ip, ""
}

// detectDefaultGateway is best-effort: on Linux we parse /proc/net/route;
// on other platforms we return "" rather than shelling out (which is
// brittle and would leak a child-process-visible command name).
func detectDefaultGateway() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n")[1:] {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		// Destination 00000000 = default route. Gateway is little-endian hex.
		if fields[1] != "00000000" {
			continue
		}
		gw := fields[2]
		if len(gw) != 8 {
			continue
		}
		var b [4]byte
		for i := 0; i < 4; i++ {
			v := uint8(0)
			_, err := fmt.Sscanf(gw[i*2:i*2+2], "%x", &v)
			if err != nil {
				return ""
			}
			b[3-i] = v
		}
		return net.IPv4(b[0], b[1], b[2], b[3]).String()
	}
	return ""
}

// publicIPProbeEndpoints lists the v4 / v6 HTTP endpoints we hit to
// learn the host's apparent public address. Each pair has a v4-only
// hostname and a v6-only hostname so the underlying dial naturally
// picks the right transport without us having to force a family —
// `api.ipify.org` resolves only to A records, `api6.ipify.org` only
// to AAAA, and the body is a bare IP string. The fallback set
// (ident.me) is unrelated infrastructure so a single provider outage
// or geographic block can't silence us; we always try the first pair
// and fall through to the second on per-family error.
//
// Why HTTP instead of DNS: the o-o.myaddr.l.google.com TXT trick we
// used previously returns the *DNS resolver's* IP, which under a
// campus / corporate network is often a recursive resolver many hops
// removed from the host's actual egress (e.g. CERNET resolvers in
// front of every Tsinghua box). HTTP echoes the server-observed TCP
// source IP, which is the precise NAT-translated egress we want.
var publicIPProbeEndpoints = []struct {
	v4URL string
	v6URL string
}{
	{v4URL: "https://api.ipify.org", v6URL: "https://api6.ipify.org"},
	{v4URL: "https://v4.ident.me", v6URL: "https://v6.ident.me"},
}

// detectPublicIPs probes the v4 / v6 endpoints in parallel and
// returns the apparent public addresses for each family. Either side
// may come back empty: dual-stack hosts get both, single-stack hosts
// get whichever family has working egress, and a host with no
// outbound connectivity gets nothing. The 1500ms timeout caps the
// whole pair so a stalled v6 path can't hold up the snapshot.
//
// Each family runs sequentially through its fallback chain so a
// blocked / slow primary doesn't waste the whole budget — we move on
// to the next provider as soon as the previous one fails. The two
// families never block each other.
func detectPublicIPs(ctx context.Context) (v4, v6 string) {
	c, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()

	type result struct {
		family string
		ip     string
	}
	out := make(chan result, 2)

	probe := func(family string) {
		urls := make([]string, 0, len(publicIPProbeEndpoints))
		for _, ep := range publicIPProbeEndpoints {
			if family == "v4" {
				urls = append(urls, ep.v4URL)
			} else {
				urls = append(urls, ep.v6URL)
			}
		}
		for _, u := range urls {
			if ip := fetchPublicIP(c, u, family); ip != "" {
				out <- result{family: family, ip: ip}
				return
			}
		}
		out <- result{family: family}
	}

	go probe("v4")
	go probe("v6")

	for i := 0; i < 2; i++ {
		r := <-out
		switch r.family {
		case "v4":
			v4 = r.ip
		case "v6":
			v6 = r.ip
		}
	}
	return v4, v6
}

// fetchPublicIP issues a single GET against url and returns the body
// parsed as an IP literal of the requested family. The dialer is
// pinned to the matching network ("tcp4" / "tcp6") so a v4-only
// hostname accidentally resolved over a happy-eyeballs preferred v6
// path can't ever come back as a v6 address (and vice versa). Empty
// return means "this provider didn't give us a usable answer" — the
// caller falls through to the next provider in the chain.
func fetchPublicIP(ctx context.Context, url, family string) string {
	network := "tcp4"
	if family == "v6" {
		network = "tcp6"
	}
	transport := &http.Transport{
		// Match the family on every layer of the dial — the resolver
		// step (LookupIP) honours the requested network, and the
		// final TCP connect uses the same network so a misconfigured
		// dual-stack provider can't sneak a wrong-family answer past
		// us.
		DialContext: func(dctx context.Context, _, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 800 * time.Millisecond}
			return d.DialContext(dctx, network, address)
		},
		TLSHandshakeTimeout:   800 * time.Millisecond,
		ResponseHeaderTimeout: 800 * time.Millisecond,
		DisableKeepAlives:     true,
	}
	client := &http.Client{Transport: transport}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "platypus-agent/public-ip-probe")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	// Bodies are tiny (an IP literal). Cap the read so a misbehaving
	// provider can't trick us into buffering megabytes.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 128))
	if err != nil {
		return ""
	}
	ip := net.ParseIP(strings.TrimSpace(string(body)))
	if ip == nil {
		return ""
	}
	isV4 := ip.To4() != nil
	if (family == "v4" && !isV4) || (family == "v6" && isV4) {
		return ""
	}
	return ip.String()
}
