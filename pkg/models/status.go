package models

import (
	"os"
	"os/user"
	"runtime"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/matishsiao/goInfo"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/mem"
)

// NetworkStatus represents the network status of the server
type NetworkStatus struct {
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
}

// NewNetworkStatus returns a new network status of the server
func NewNetworkStatus() NetworkStatus {
	hostname, _ := os.Hostname()
	return NetworkStatus{
		Hostname: hostname,
	}
}

// CPUStatus represents the CPU status of the server
type CPUStatus struct {
	Percent    float64 `json:"percent"`
	NumCores   int     `json:"num_cores"`
	NumThreads int     `json:"num_threads"`
}

// NewCPUStatus returns a new CPU status of the server
func NewCPUStatus() CPUStatus {
	cpuPercent, _ := cpu.Percent(time.Second, false)
	numCores := runtime.NumCPU()
	numThreads := runtime.GOMAXPROCS(0)
	return CPUStatus{
		Percent:    cpuPercent[0],
		NumCores:   numCores,
		NumThreads: numThreads,
	}
}

// DiskStatus represents the disk status of the server
type DiskStatus struct {
	Total uint64 `json:"total"`
	Used  uint64 `json:"used"`
}

// NewDiskStatus returns a new disk status of the server
func NewDiskStatus() DiskStatus {
	cache := expirable.NewLRU[string, DiskStatus](1, nil, 30*time.Minute)
	return func() DiskStatus {
		r, ok := cache.Get("disk_status")
		if ok {
			return r
		}
		diskStat, _ := disk.Usage("/")
		status := DiskStatus{
			Total: diskStat.Total,
			Used:  diskStat.Used,
		}
		cache.Add("disk_status", status)
		return status
	}()
}

// MemoryStatus represents the memory status of the server
type MemoryStatus struct {
	Total uint64 `json:"total"`
	Used  uint64 `json:"used"`
}

// NewMemoryStatus returns a new memory status of the server
func NewMemoryStatus() MemoryStatus {
	memStat, _ := mem.VirtualMemory()
	return MemoryStatus{
		Total: memStat.Total,
		Used:  memStat.Used,
	}
}

// GoStatus represents the Go status of the server
type GoStatus struct {
	Version       string `json:"version"`
	NumGoroutines int    `json:"num_goroutines"`
	NumCgoCalls   int64  `json:"num_cgo_calls"`
	MemoryUsage   int64  `json:"memory_usage"`
}

// NewGoStatus returns a new Go status of the server
func NewGoStatus() GoStatus {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	numGoroutines := runtime.NumGoroutine()
	numCgoCalls := runtime.NumCgoCall()
	return GoStatus{
		NumGoroutines: numGoroutines,
		Version:       runtime.Version(),
		NumCgoCalls:   numCgoCalls,
		MemoryUsage:   int64(m.Alloc),
	}
}

// OSStatus represents the OS status of the server
type OSStatus struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Arch    string `json:"arch"`
}

// NewOSStatus returns a new OS status of the server
func NewOSStatus() OSStatus {
	var version string = "unknown"
	gi, err := goInfo.GetInfo()
	if err == nil {
		version = gi.Core
	}
	return OSStatus{
		Name:    runtime.GOOS,
		Version: version,
		Arch:    runtime.GOARCH,
	}
}

// UserStatus represents the user status of the server
type UserStatus struct {
	Username string `json:"username"`
	IsRoot   bool   `json:"is_root"`
}

// NewUserStatus returns a new user status of the server
func NewUserStatus() UserStatus {
	var username string
	u, err := user.Current()
	if err != nil {
		username = "unknown"
	} else {
		username = u.Username
	}
	return UserStatus{
		Username: username,
		IsRoot:   os.Getuid() == 0,
	}
}

// Status represents the status of the server
type Status struct {
	OSStatus      `json:"os,omitempty,omitzero"`
	NetworkStatus `json:"network,omitempty,omitzero"`
	CPUStatus     `json:"cpu,omitempty,omitzero"`
	DiskStatus    `json:"disk,omitempty,omitzero"`
	MemoryStatus  `json:"memory,omitempty,omitzero"`
	GoStatus      `json:"go,omitempty,omitzero"`
	UserStatus    `json:"user,omitempty,omitzero"`
	Timestamp     time.Time `json:"timestamp,omitzero"`
}

// StatusGrabber is a helper to grab the status of the server
type StatusGrabber struct {
	withOS      bool
	withNetwork bool
	withCPU     bool
	withDisk    bool
	withMemory  bool
	withUser    bool
	withGo      bool
}

// NewStatusGrabber returns a new status grabber
func NewStatusGrabber() StatusGrabber {
	return StatusGrabber{}
}

// WithAll adds all status to the status
func (s StatusGrabber) WithAll() StatusGrabber {
	s.withOS = true
	s.withNetwork = true
	s.withCPU = true
	s.withDisk = true
	s.withMemory = true
	s.withGo = true
	return s
}

// WithOS adds OS status to the status
func (s StatusGrabber) WithOS() StatusGrabber {
	s.withOS = true
	return s
}

// WithNetwork adds network status to the status
func (s StatusGrabber) WithNetwork() StatusGrabber {
	s.withNetwork = true
	return s
}

// WithCPU adds CPU status to the status
func (s StatusGrabber) WithCPU() StatusGrabber {
	s.withCPU = true
	return s
}

// WithDisk adds disk status to the status
func (s StatusGrabber) WithDisk() StatusGrabber {
	s.withDisk = true
	return s
}

// WithMemory adds memory status to the status
func (s StatusGrabber) WithMemory() StatusGrabber {
	s.withMemory = true
	return s
}

// WithUser adds user status to the status
func (s StatusGrabber) WithUser() StatusGrabber {
	s.withUser = true
	return s
}

// WithGo adds Go status to the status
func (s StatusGrabber) WithGo() StatusGrabber {
	s.withGo = true
	return s
}

// Grab returns the status of the server
func (s StatusGrabber) Grab() Status {
	status := Status{Timestamp: time.Now()}
	if s.withOS {
		status.OSStatus = NewOSStatus()
	}
	if s.withNetwork {
		status.NetworkStatus = NewNetworkStatus()
	}
	if s.withCPU {
		status.CPUStatus = NewCPUStatus()
	}
	if s.withDisk {
		status.DiskStatus = NewDiskStatus()
	}
	if s.withMemory {
		status.MemoryStatus = NewMemoryStatus()
	}
	if s.withGo {
		status.GoStatus = NewGoStatus()
	}
	if s.withUser {
		status.UserStatus = NewUserStatus()
	}
	return status
}
