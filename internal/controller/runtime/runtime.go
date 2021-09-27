package runtime_controller

import (
	"github.com/gin-gonic/gin"

	runtime_model "github.com/WangYihang/Platypus/internal/model/runtime"
	"github.com/WangYihang/Platypus/internal/util/update"
)

func GetCpuState(c *gin.Context) {
	cpuState := runtime_model.CpuState{}
	runtime_model.CollectCPUStats(&cpuState)
	c.JSON(200, gin.H{
		"status": true,
		"msg":    cpuState,
	})
}

func GetMemoryState(c *gin.Context) {
	memoryState := runtime_model.MemoryState{}
	runtime_model.CollectMemoryStats(&memoryState)
	c.JSON(200, gin.H{
		"status": true,
		"msg":    memoryState,
	})
}

func GetGcState(c *gin.Context) {
	gcState := runtime_model.GcState{}
	runtime_model.CollectGcStats(&gcState)
	c.JSON(200, gin.H{
		"status": true,
		"msg":    gcState,
	})
}

func GetVersion(c *gin.Context) {
	c.JSON(200, gin.H{
		"status": true,
		"msg":    update.Version,
	})
}
