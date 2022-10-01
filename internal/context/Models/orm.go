package Models

import (
	"fmt"
	"github.com/WangYihang/Platypus/internal/context/Conf"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

var Db *gorm.DB

func init() {
	var err error
	Db, err = gorm.Open("sqlite3", Conf.ConfData.DBFile)
	if err != nil {
		fmt.Println("openDBerr:", err)
		return
	}
}

// CreateAccess 所有反弹shell创建时都会归到超级管理员手里
func CreateAccess(c *Access) {
	Db.Create(c)
	Db.Save(c)
}

// DeleteAccess 所有反弹shell删除时都会在数据库中删除
func DeleteAccess(hash string) {
	var ac Access
	Db.First(&ac, "hash = ?", hash)
	Db.Delete(&ac)
}

func CreateRole(db *gorm.DB, grade string) *Role {
	r := Role{Grade: grade}
	db.Create(&r)
	db.Save(&r)
	return &r
}

func CreateUser(db *gorm.DB, user *User) {

	var role Role
	if user.ID == 1 {
		// 超级管理员初始化
		err := db.First(&role, "grade = ?", SuperRole).Error
		if err == nil {
			user.Roles = append(user.Roles, role)
		} else {
			r := CreateRole(db, SuperRole)

			user.Roles = append(user.Roles, *r)
		}
	} else {
		//搜索有没有普通用户这个角色,没有的话先创建
		err := db.First(&role, "grade = ?", CommonRole).Error
		if err == nil {
			user.Roles = append(user.Roles, role)
		} else {
			r := CreateRole(db, CommonRole)

			user.Roles = append(user.Roles, *r)
		}
	}
	db.Save(&user)
}

func RegisterOne(username, password, tel string) bool {
	// 先检查一下用户名是否已经存在
	var person User
	Db.First(&person, "user_name=?", username)
	if person.UserName != "" {
		return false
	}
	p := User{
		UserName: username,
		Password: password,
		Tel:      tel,
	}
	Db.Save(&p)
	CreateUser(Db, &p)
	return true
}

func CheckPassword(plainText, cipherText string) bool {
	return cipherText == EncryptAlg(plainText)
}

func VerifyUser(userName, password string) bool {
	var person User
	Db.First(&person, "user_name=?", userName)
	if person.UserName != "" && CheckPassword(password, person.Password) {
		return true
	}
	return false
}

// ResetPassword 输入的参数都是经过检查的, 包含内容的
func ResetPassword(userName, password, tel string) bool {

	var person User
	Db.First(&person, "user_name=?", userName)

	if person.UserName != "" && person.Tel == tel {
		person.Password = password
		Db.Save(&person)
		return true
	}
	return false
}
