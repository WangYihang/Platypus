package Models

import (
	"fmt"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"os"
)

var Db *gorm.DB

func OpenDb(fileName string) {
	var err error
	Db, err = gorm.Open("sqlite3", fileName)
	if err != nil {
		fmt.Println("openDBerr:", err)
		return
	}
	// 目前会将被关闭的反弹shell直接完全删除, 所以清空对应的表即可, 后续计划将反弹shell软删除
	Db.Exec("DELETE FROM accesses;")
	Db.Exec("UPDATE sqlite_sequence SET seq = 0 WHERE name = \"accesses\";")
	Db.Exec("DELETE FROM role_accesses;")
	Db.Exec("UPDATE sqlite_sequence SET seq = 0 WHERE name = \"role_accesses\";")
}

func CreateDb(fileName string) {
	fp, err := os.Create(fileName)
	if err != nil {
		fmt.Printf("fileName: %s, fp: %v, err: %v\n", fileName, fp, err)
		panic(err)
	} else {
		Db, err = gorm.Open("sqlite3", fileName)
		if err != nil {
			fmt.Println("openDBerr:", err)
			return
		}
		var user User
		var role Role
		var access Access
		Db.AutoMigrate(&user, &role, &access)
		CreateRole(Db, SuperRole)
		CreateRole(Db, CommonRole)
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
		person.Password = EncryptAlg(password)
		Db.Save(&person)
		return true
	}
	return false
}
