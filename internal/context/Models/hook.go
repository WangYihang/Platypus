package Models

import (
	"github.com/jinzhu/gorm"
)

// AfterCreate 创建一个新的server之后会将该server归入超级管理员
func (s Access) AfterCreate(tx *gorm.DB) {
	var role Role
	tx.Model(&Role{}).Find(&role, Role{Grade: SuperRole})

	role.Accesses = append(role.Accesses, s)
	tx.Save(&role)
	return
}

// BeforeCreate 创建用户之前的hook, 将密码aes加密一下
func (u *User) BeforeCreate(tx *gorm.DB) (err error) {
	u.Password = EncryptAlg(u.Password)
	return
}
