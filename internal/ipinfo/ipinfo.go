// Package ipinfo enriches a bare IP address with cheap-to-derive
// metadata: address class (private / loopback / link-local), IP
// version, and best-effort geo / ISP attribution from the embedded
// ip2region xdb dataset.
//
// The xdb file is shipped inside the binary so callers don't need to
// manage an out-of-tree data file. IPv6 lookup is intentionally not
// supported here — the v6 dataset is ~36 MB and rarely useful for
// the operator-facing IP display this package was written for; v6
// addresses get classified (private/global/version) but no geo data.
package ipinfo

import (
	"container/list"
	_ "embed"
	"net"
	"strings"
	"sync"

	"github.com/lionsoul2014/ip2region/binding/golang/xdb"
)

//go:embed data/ip2region_v4.xdb
var v4DB []byte

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
	searcherOnce sync.Once
	searcherMu   sync.Mutex // ip2region's xdb.Searcher is not safe for concurrent Search().
	searcher     *xdb.Searcher
	cache        = newLRU(1024)
)

func ensureSearcher() {
	searcherOnce.Do(func() {
		s, err := xdb.NewWithBuffer(xdb.IPv4, v4DB)
		if err != nil {
			// Embedded data is built into the binary — a load error
			// here is a build/release problem, not something callers
			// can recover from. Falling back to nil leaves geo fields
			// blank rather than crashing the process.
			return
		}
		searcher = s
	})
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

	// Only public IPv4 addresses get a geo lookup. Private ranges
	// would just return blanks anyway, and IPv6 isn't covered by the
	// embedded dataset.
	if info.Version == 4 && !info.IsPrivate && !info.IsLoopback {
		if region, ok := lookupV4(key); ok {
			info.Country, info.Province, info.City, info.ISP = parseRegion(region)
		}
	}

	cache.put(key, info)
	return info
}

func lookupV4(ip string) (string, bool) {
	ensureSearcher()
	if searcher == nil {
		return "", false
	}
	searcherMu.Lock()
	defer searcherMu.Unlock()
	r, err := searcher.Search(ip)
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
// multicast, unspecified.
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
