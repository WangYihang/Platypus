// Package ipinfo enriches a bare IP address with cheap-to-derive
// metadata: address class (private / loopback / link-local), IP
// version, and best-effort geo / ISP attribution from an ip2region
// xdb dataset loaded at runtime.
//
// Resolution order for the v4 / v6 xdb files:
//
//  1. Env override — PLATYPUS_IP2REGION_V4_XDB / PLATYPUS_IP2REGION_V6_XDB
//  2. <exec dir>/data/ip2region_v{4,6}.xdb — what release tarballs ship
//  3. $XDG_DATA_HOME/platypus/ip2region_v{4,6}.xdb (or ~/.local/share/...)
//  4. ./data/ip2region_v{4,6}.xdb — dev-tree convenience
//
// First file that exists and parses wins. A missing / malformed file
// degrades to "classification only" — the IP still gets a version /
// private / loopback verdict, but country / ISP / city stay empty.
// IPv6 enrichment is opt-in: drop a v6 xdb in any of the search
// paths to turn it on.
package ipinfo

import (
	"container/list"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/lionsoul2014/ip2region/binding/golang/xdb"

	"github.com/WangYihang/Platypus/internal/log"
)

const (
	envV4Override = "PLATYPUS_IP2REGION_V4_XDB"
	envV6Override = "PLATYPUS_IP2REGION_V6_XDB"
	v4Filename    = "ip2region_v4.xdb"
	v6Filename    = "ip2region_v6.xdb"
)

// Info is the enriched description we hand back for an IP. Empty
// strings on the geo fields mean "unknown / not looked up" — the
// caller decides whether to omit them in the rendered UI.
type Info struct {
	IP         string `json:"ip"`
	Version    int    `json:"version,omitempty"` // 4 or 6; 0 when unparseable
	IsPrivate  bool   `json:"is_private,omitempty"`
	IsLoopback bool   `json:"is_loopback,omitempty"`
	Country    string `json:"country,omitempty"`
	Province   string `json:"province,omitempty"`
	City       string `json:"city,omitempty"`
	ISP        string `json:"isp,omitempty"`
}

var (
	// Per-version: a sync.Once gates initialisation, a mutex guards
	// concurrent .Search() calls (xdb.Searcher isn't safe for parallel
	// access), and the searcher is nil when no xdb file was found.
	searcherV4Once sync.Once
	searcherV4Mu   sync.Mutex
	searcherV4     *xdb.Searcher

	searcherV6Once sync.Once
	searcherV6Mu   sync.Mutex
	searcherV6     *xdb.Searcher

	cache = newLRU(1024)
)

// loadSearcher resolves the xdb path for the given IP version and
// opens it via xdb.NewWithFileOnly so we don't slurp the whole file
// (especially the 36 MB v6 dataset) into memory. Missing / malformed
// files log once and leave the returned searcher nil; callers must
// nil-check.
func loadSearcher(version *xdb.Version, envVar, filename string) *xdb.Searcher {
	path := resolveXDBPath(envVar, filename)
	if path == "" {
		log.L.Info("ipinfo.xdb_not_found",
			"version", version.Name,
			"hint", "set "+envVar+" or place "+filename+" next to the binary",
		)
		return nil
	}
	s, err := xdb.NewWithFileOnly(version, path)
	if err != nil {
		log.L.Warn("ipinfo.xdb_load_failed",
			"version", version.Name,
			"path", path,
			"error", err.Error(),
		)
		return nil
	}
	log.L.Info("ipinfo.xdb_loaded", "version", version.Name, "path", path)
	return s
}

// resolveXDBPath walks the standard search paths and returns the
// first one that exists.
func resolveXDBPath(envVar, filename string) string {
	if v := strings.TrimSpace(os.Getenv(envVar)); v != "" {
		if fileExists(v) {
			return v
		}
	}
	candidates := []string{}
	if execPath, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(execPath), "data", filename))
	}
	if dataHome := os.Getenv("XDG_DATA_HOME"); dataHome != "" {
		candidates = append(candidates, filepath.Join(dataHome, "platypus", filename))
	} else if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".local", "share", "platypus", filename))
	}
	candidates = append(candidates, filepath.Join("data", filename))

	for _, p := range candidates {
		if fileExists(p) {
			return p
		}
	}
	return ""
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func ensureV4Searcher() *xdb.Searcher {
	searcherV4Once.Do(func() {
		searcherV4 = loadSearcher(xdb.IPv4, envV4Override, v4Filename)
	})
	return searcherV4
}

func ensureV6Searcher() *xdb.Searcher {
	searcherV6Once.Do(func() {
		searcherV6 = loadSearcher(xdb.IPv6, envV6Override, v6Filename)
	})
	return searcherV6
}

// Lookup returns enrichment for an IP literal. host:port forms and
// surrounding whitespace are tolerated. Results are LRU-cached so
// repeated calls (e.g. listing the same sessions every poll) only
// pay the xdb cost once per distinct address.
func Lookup(raw string) Info {
	key := strings.TrimSpace(raw)
	if h, _, err := net.SplitHostPort(key); err == nil {
		key = h
	}
	// IPv6 literals can arrive bracketed (`[::1]`) — strip once we've
	// handled the host:port form above so a plain `[::1]` still works.
	key = strings.TrimPrefix(strings.TrimSuffix(key, "]"), "[")

	if v, ok := cache.get(key); ok {
		return v
	}

	info := Info{IP: key}
	ip := net.ParseIP(key)
	if ip == nil {
		cache.put(key, info)
		return info
	}
	if ip.To4() != nil {
		info.Version = 4
	} else {
		info.Version = 6
	}
	info.IsLoopback = ip.IsLoopback()
	info.IsPrivate = isPrivateOrReserved(ip)

	// Skip geo lookup for non-routable addresses regardless of version
	// — both the v4 and v6 datasets only carry public-internet ranges,
	// and a private IP would just return blanks anyway.
	if !info.IsPrivate && !info.IsLoopback {
		var region string
		var ok bool
		switch info.Version {
		case 4:
			region, ok = lookupV4(key)
		case 6:
			region, ok = lookupV6(key)
		}
		if ok {
			info.Country, info.Province, info.City, info.ISP = parseRegion(region)
		}
	}

	cache.put(key, info)
	return info
}

func lookupV4(ip string) (string, bool) {
	s := ensureV4Searcher()
	if s == nil {
		return "", false
	}
	searcherV4Mu.Lock()
	defer searcherV4Mu.Unlock()
	r, err := s.Search(ip)
	if err != nil || r == "" {
		return "", false
	}
	return r, true
}

func lookupV6(ip string) (string, bool) {
	s := ensureV6Searcher()
	if s == nil {
		return "", false
	}
	searcherV6Mu.Lock()
	defer searcherV6Mu.Unlock()
	r, err := s.Search(ip)
	if err != nil || r == "" {
		return "", false
	}
	return r, true
}

// parseRegion splits ip2region's "国家|区域|省份|城市|ISP" pipe-delimited
// region string into named fields. Missing values appear as "0" in
// the dataset and are normalised to empty.
func parseRegion(region string) (country, province, city, isp string) {
	parts := strings.Split(region, "|")
	get := func(i int) string {
		if i >= len(parts) {
			return ""
		}
		v := strings.TrimSpace(parts[i])
		if v == "0" || v == "" {
			return ""
		}
		return v
	}
	// Layout: 国家 | 区域 | 省份 | 城市 | ISP. The "区域" slot is almost
	// always "0" for inland CN data, so we drop it from the surface.
	return get(0), get(2), get(3), get(4)
}

// isPrivateOrReserved covers the ranges net.IP.IsPrivate misses that
// still aren't useful to look up: link-local, CGNAT (100.64/10),
// multicast, unspecified, plus IPv6 ULA (fc00::/7). IPv4-mapped v6
// addresses recurse into the v4 check so a tunneled RFC1918 still
// classifies as private.
func isPrivateOrReserved(ip net.IP) bool {
	if ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		// CGNAT shared address space, RFC 6598.
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return true
		}
		return false
	}
	// IPv6 from here on. ULA fc00::/7 — IsPrivate already covers fd00::/8
	// (the locally-assigned half) but not the fc00::/8 reserved half;
	// treat both as non-routable.
	if len(ip) == net.IPv6len && (ip[0]&0xfe) == 0xfc {
		return true
	}
	return false
}

// --- bounded LRU ----------------------------------------------------

type lruCache struct {
	mu    sync.Mutex
	cap   int
	ll    *list.List
	index map[string]*list.Element
}

type lruEntry struct {
	key string
	val Info
}

func newLRU(capacity int) *lruCache {
	return &lruCache{
		cap:   capacity,
		ll:    list.New(),
		index: make(map[string]*list.Element, capacity),
	}
}

func (c *lruCache) get(key string) (Info, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.index[key]; ok {
		c.ll.MoveToFront(e)
		return e.Value.(*lruEntry).val, true
	}
	return Info{}, false
}

func (c *lruCache) put(key string, val Info) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.index[key]; ok {
		e.Value.(*lruEntry).val = val
		c.ll.MoveToFront(e)
		return
	}
	e := c.ll.PushFront(&lruEntry{key: key, val: val})
	c.index[key] = e
	if c.ll.Len() > c.cap {
		old := c.ll.Back()
		if old != nil {
			c.ll.Remove(old)
			delete(c.index, old.Value.(*lruEntry).key)
		}
	}
}
