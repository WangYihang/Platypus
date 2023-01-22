package runtime

import (
	"github.com/gin-gonic/gin"

	runtime_model "github.com/WangYihang/Platypus/internal/models/runtime"
	"github.com/WangYihang/Platypus/internal/utils/update"
)

// GetCpuState godoc
// @Summary     Get CPU State
// @Description get current CPU state, including number CPU cores, number of goroutines, etc.
// @Tags        Runtime
// @Accept      json
// @Produce     json
// @Success     200 {object} runtime_model.CpuState
// @Router      /runtime/cpu [get]
// @Security    ApiKeyAuth
func GetCpuState(c *gin.Context) {
	cpuState := runtime_model.CpuState{}
	runtime_model.CollectCPUStats(&cpuState)
	c.JSON(200, gin.H{
		"status": true,
		"msg":    cpuState,
	})
}

// GetMemoryState godoc
// @Summary     Get Memory State
// @Description get current memory state.
// @Tags        Runtime
// @Accept      json
// @Produce     json
// @Success     200 {object} runtime_model.MemoryState
// @Router      /runtime/memory [get]
// @Security    ApiKeyAuth
func GetMemoryState(c *gin.Context) {
	memoryState := runtime_model.MemoryState{}
	runtime_model.CollectMemoryStats(&memoryState)
	c.JSON(200, gin.H{
		"status": true,
		"msg":    memoryState,
	})
}

// GetGcState godoc
// @Summary     Get Garbage Collection State
// @Description get current garbage collection state.
// @Tags        Runtime
// @Accept      json
// @Produce     json
// @Success     200 {object} runtime_model.GcState
// @Router      /runtime/gc [get]
// @Security    ApiKeyAuth
func GetGcState(c *gin.Context) {
	gcState := runtime_model.GcState{}
	runtime_model.CollectGcStats(&gcState)
	c.JSON(200, gin.H{
		"status": true,
		"msg":    gcState,
	})
}

// GetVersion godoc
// @Summary     Get Version
// @Description get current version of Platypus.
// @Tags        Runtime
// @Accept      json
// @Produce     json
// @Success     200 {object} string
// @Router      /runtime/version [get]
// @Security    ApiKeyAuth
func GetVersion(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": true,
		"msg":    update.Version,
	})
}
