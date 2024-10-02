package controllers

import (
	"github.com/WangYihang/Platypus/pkg/version"
	"github.com/gin-gonic/gin"
)

// VersionController handles the version endpoint
func VersionController(c *gin.Context) {
	c.JSON(200, gin.H{
		"version": version.Version,
		"commit":  version.Commit,
		"date":    version.Date,
	})
}
