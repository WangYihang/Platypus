package Models

import (
	"github.com/jinzhu/gorm"
)

func ListUsers(db *gorm.DB) []User {
	var users []User
	// 指定user表, 查找所有user, 并且预加载Languages项外键, 不预加载的话看不到这项内容
	db.Find(&users)
	return users
}

// ListRoles 如果要管理某台电脑, 就切换到某个角色, 角色就在当前页, 不同的角色标签会附带当前持有多少个Server的标记
func (u *User) ListRoles() []Role {
	if u == nil {
		return nil
	}
	return u.Roles
}

func (r *Role) ListAccesses() []Access {
	if r == nil {
		return nil
	}
	return r.Accesses
}

func ListAllAccesses(userName string) map[string]bool {
	var user User
	Db.Preload("Roles").First(&user, "user_name = ?", userName)
	accessesMap := make(map[string]bool)
	for _, r := range user.Roles {
		var role Role
		Db.Preload("Accesses").First(&role, "grade = ?", r.Grade)
		for _, ac := range r.Accesses {
			accessesMap[ac.Hash] = true
		}
	}
	return accessesMap
}

// UserAddRole 创建一个超级管理员页面, oauth验证只有超级管理员才可以打开, 可以实现以下操作
func UserAddRole(userName string, grade ...string) bool {

	var user User

	Db.Preload("Role").Find(&user, "user_name = ?", userName)
	if &user == nil {
		return false
	}
	var role Role
	for _, g := range grade {
		Db.Find(&role, "grade = ?", g)
		if &role != nil {
			user.Roles = append(user.Roles, role)
		}
	}
	Db.Save(&user)
	return true

}

func RoleAddAccess(grade string, accesses ...string) bool {
	//接收前端传来的rolename和permissions

	//连接数据库

	//查数据库内有没有该角色名 有就更新 没有就创建
	var role Role
	Db.Preload("Accesses").Find(&role, "grade = ?", grade)
	if &role == nil {
		return false
	}
	var access Access
	for _, a := range accesses {
		Db.Find(&access, "hash = ?", a)
		if &access != nil {
			role.Accesses = append(role.Accesses, access)
		}
	}
	Db.Save(&role)
	return true

}

//TODO: 当得到一个新的反弹shell要把他记录在数据库中 所有server创建时都会归到超级管理员手里
func ListRolesExpectSuperAdmin() []string {

	var roles []Role
	var ret []string
	Db.Select("id").Find(&roles)
	for _, role := range roles {
		if role.Grade != SuperRole {
			ret = append(ret, role.Grade)
		}
	}
	return ret
}
