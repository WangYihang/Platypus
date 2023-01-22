package runtime_model

import (
	"runtime"
	"time"
)

type CpuState struct {
	// CPU
	NumCPU       int64 `json:"cpu.count"`
	NumGoroutine int64 `json:"cpu.goroutines_number"`
	CgoCallDelta int64 `json:"cpu.cgo_calls_number_delta"`
	NumCgoCall   int64 `json:"cpu.cgo_calls_number_total"`
}

type MemoryState struct {
	// General
	Alloc        int64 `json:"mem.general.alloc_bytes"`
	TotalAlloc   int64 `json:"mem.general.total_bytes"`
	Sys          int64 `json:"mem.general.sys_bytes"`
	LookupsDelta int64 `json:"mem.general.lookups_number_delta"`
	MallocsDelta int64 `json:"mem.general.mallocs_number_delta"`
	FreesDelta   int64 `json:"mem.general.frees_number_delta"`
	Lookups      int64 `json:"mem.general.lookups_number_total"`
	Mallocs      int64 `json:"mem.general.mallocs_number_total"`
	Frees        int64 `json:"mem.general.frees_number_total"`

	// Heap
	HeapAlloc    int64 `json:"mem.heap.alloc_bytes"`
	HeapSys      int64 `json:"mem.heap.sys_bytes"`
	HeapIdle     int64 `json:"mem.heap.idle_bytes"`
	HeapInuse    int64 `json:"mem.heap.inuse_bytes"`
	HeapReleased int64 `json:"mem.heap.released_bytes"`
	HeapObjects  int64 `json:"mem.heap.objects_number"`

	// Stack
	StackInuse  int64 `json:"mem.stack.inuse_bytes"`
	StackSys    int64 `json:"mem.stack.sys_bytes"`
	MSpanInuse  int64 `json:"mem.stack.mspan_inuse_bytes"`
	MSpanSys    int64 `json:"mem.stack.mspan_sys_bytes"`
	MCacheInuse int64 `json:"mem.stack.mcache_inuse_bytes"`
	MCacheSys   int64 `json:"mem.stack.mcache_sys_bytes"`

	OtherSys int64 `json:"mem.othersys_bytes"`
}

type GcState struct {
	// GC
	GCSys             int64   `json:"gc.sys_bytes"`
	NextGC            int64   `json:"gc.next_bytes"`
	BetweenGCPerdiod  int64   `json:"gc.between_period_s"`
	LastGC            int64   `json:"-"`
	TimeFromLastGC    int64   `json:"gc.time_from_last_gc_s"`
	PauseTotalNsDelta int64   `json:"gc.pause_ns_total_delta"`
	PauseTotalNs      int64   `json:"gc.pause_ns_total"`
	PauseNs           int64   `json:"gc.pause_ns"`
	LastPauseNs       int64   `json:"gc.pause_last_ns"`
	NumGCDelta        int64   `json:"gc.number_delta"`
	NumGC             int64   `json:"gc.number_total"`
	GCCPUFraction     float64 `json:"gc.cpu_fraction_total"`
}

func CollectCPUStats(cpuState *CpuState) {
	cpuState.NumCPU = int64(runtime.NumCPU())
	cpuState.NumGoroutine = int64(runtime.NumGoroutine())
	cpuState.NumCgoCall = int64(runtime.NumCgoCall())
	cpuState.CgoCallDelta = int64(runtime.NumCgoCall()) - cpuState.NumCgoCall
	cpuState.NumCgoCall = int64(runtime.NumCgoCall())
}

func CollectMemoryStats(memoryState *MemoryState) {
	m := runtime.MemStats{}
	runtime.ReadMemStats(&m)
	// General
	memoryState.Alloc = int64(m.Alloc)
	memoryState.TotalAlloc = int64(m.TotalAlloc)
	memoryState.Sys = int64(m.Sys)
	memoryState.LookupsDelta = int64(m.Lookups) - memoryState.Lookups
	memoryState.MallocsDelta = int64(m.Mallocs) - memoryState.Mallocs
	memoryState.FreesDelta = int64(m.Frees) - memoryState.Frees
	memoryState.Lookups = int64(m.Lookups)
	memoryState.Mallocs = int64(m.Mallocs)
	memoryState.Frees = int64(m.Frees)

	// Heap
	memoryState.HeapAlloc = int64(m.HeapAlloc)
	memoryState.HeapSys = int64(m.HeapSys)
	memoryState.HeapIdle = int64(m.HeapIdle)
	memoryState.HeapInuse = int64(m.HeapInuse)
	memoryState.HeapReleased = int64(m.HeapReleased)
	memoryState.HeapObjects = int64(m.HeapObjects)

	// Stack
	memoryState.StackInuse = int64(m.StackInuse)
	memoryState.StackSys = int64(m.StackSys)
	memoryState.MSpanInuse = int64(m.MSpanInuse)
	memoryState.MSpanSys = int64(m.MSpanSys)
	memoryState.MCacheInuse = int64(m.MCacheInuse)
	memoryState.MCacheSys = int64(m.MCacheSys)
	memoryState.OtherSys = int64(m.OtherSys)
}

func CollectGcStats(gcState *GcState) {
	m := runtime.MemStats{}
	runtime.ReadMemStats(&m)
	gcState.GCSys = int64(m.GCSys)
	gcState.NextGC = int64(m.NextGC)
	if int64(m.LastGC) > gcState.LastGC {
		gcState.PauseNs = int64(m.PauseNs[(m.NumGC+255)%256])
		gcState.LastPauseNs = int64(m.PauseNs[(m.NumGC+255)%256])
	}
	if int64(m.LastGC) != 0 {
		// time in second from last gc cycle
		gcState.TimeFromLastGC = time.Now().Unix() - (int64(m.LastGC) / 1000000000)
	}
	if gcState.LastGC != 0 && int64(m.LastGC) > gcState.LastGC {
		// gcState.BetweenGCPerdiod - calculated time period between last and previous GC
		gcState.BetweenGCPerdiod = int64(m.LastGC) - gcState.LastGC
		// convert to nano seconds to seconds
		gcState.BetweenGCPerdiod = gcState.BetweenGCPerdiod / 1000000000
	}
	// if there was no GC cycle during last goruntimestats interval (default 60s) set BetweenGCPerdiod and gcState.PauseNs to 0
	if int64(m.LastGC) == gcState.LastGC {
		gcState.BetweenGCPerdiod = 0
		gcState.PauseNs = 0
	}
	gcState.PauseTotalNsDelta = int64(m.PauseTotalNs) - gcState.PauseTotalNs
	gcState.PauseTotalNs = int64(m.PauseTotalNs)
	gcState.LastGC = int64(m.LastGC)
	gcState.NumGCDelta = int64(m.NumGC) - gcState.NumGC
	gcState.NumGC = int64(m.NumGC)
	gcState.GCCPUFraction = float64(m.GCCPUFraction)
}
