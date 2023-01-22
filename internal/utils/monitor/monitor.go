package monitor

import (
	"fmt"
	"time"

	runtime_model "github.com/WangYihang/Platypus/internal/models/runtime"
	go_units "github.com/docker/go-units"
)

func Monitor(interval int) {
	for {
		time.Sleep(time.Duration(interval) * time.Duration(time.Second))
		Snapshot()
	}
}

func Snapshot() {
	fmt.Println(time.Now().String())
	memoryState := runtime_model.MemoryState{}
	runtime_model.CollectMemoryStats(&memoryState)
	fmt.Printf("Memory usage: %s (%d)\n", go_units.BytesSize(float64(memoryState.Alloc)), memoryState.Alloc)

	cpuState := runtime_model.CpuState{}
	runtime_model.CollectCPUStats(&cpuState)
	fmt.Printf("Number of Go routines: %d\n", cpuState.NumGoroutine)
}
