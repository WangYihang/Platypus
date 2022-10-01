package Models

import (
	oss "github.com/WangYihang/Platypus/internal/util/os"
	"github.com/jinzhu/gorm"
	"time"
)

const SuperRole = "super"
const CommonRole = "common"

type User struct {
	gorm.Model
	UserName string `json:"username"` // 和sql表有关, UserName -> user_name; UserMM -> user_m_m
	Password string `json:"password"`
	Tel      string `json:"tel"`
	Roles    []Role `gorm:"many2many:user_roles;" json:"roles"`
}

type Role struct {
	gorm.Model
	Grade string `json:"grade"`
	//Users   []User   `gorm:"many2many:role_users;"`
	Accesses []Access `gorm:"many2many:role_accesses;" json:"accesses"`
}
type Access struct {
	gorm.Model
	Host      string              `json:"host"`
	Port      uint16              `json:"port"`
	Hash      string              `json:"hash"`
	User      string              `json:"user"`
	OS        oss.OperatingSystem `json:"os"`
	TimeStamp time.Time           `json:"timestamp"`
	//Role []Role `gorm:"many2many:server_roles;"`
}
