package Controller

import (
	oss "github.com/WangYihang/Platypus/internal/util/os"
	"time"
)

type RoleList struct {
	Get  bool   `json:"get"`
	Role string `json:"role"`
}

type AccessResponse struct {
	Get       bool                `json:"get"`
	Address   string              `json:"address"`
	User      string              `json:"user"`
	OS        oss.OperatingSystem `json:"os"`
	TimeStamp time.Time           `json:"timestamp"`
	Hash      string              `json:"hash"`
}
