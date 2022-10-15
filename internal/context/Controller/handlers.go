package Controller

import (
	"fmt"
	"github.com/WangYihang/Platypus/internal/context/Conf"
	"github.com/WangYihang/Platypus/internal/context/Middlewares"
	"github.com/WangYihang/Platypus/internal/context/Models"
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"
)

func CreateCaptcha(c *gin.Context) {
	Models.Captcha(c, 4)
}

func GetRbac(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": true,
		"msg":    "获取rbac页面成功",
	})
}

// GetIndex Index is the index handler.
func GetIndex(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": true,
		"msg":    "获取主页成功",
	})

}

// LoginGet - 获取登录页面
func LoginGet(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": true,
		"msg":    "获取登录页面成功",
	})

}

func LogOut(c *gin.Context) {
	c.SetCookie("refresh", "", Conf.RestfulConf.RefreshExpireTime, "/", Conf.RestfulConf.Domain, false, true)
	c.SetCookie("access", "", Conf.RestfulConf.RefreshExpireTime, "/", Conf.RestfulConf.Domain, false, true)
	c.JSON(http.StatusOK, gin.H{
		"status": true,
		"msg":    "登出成功",
	})
}

type LoginInfo struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Captcha  string `json:"captcha"`
}

// LoginPost - 发送登录信息
func LoginPost(c *gin.Context) {
	// 验证账号密码
	var loginInfo LoginInfo
	c.BindJSON(&loginInfo)
	if loginInfo.Username == "" {
		c.JSON(http.StatusOK, gin.H{
			"status": false,
			"msg":    "用户名不能为空",
		})
		return
	}
	if loginInfo.Password == "" {
		c.JSON(http.StatusOK, gin.H{
			"status": false,
			"msg":    "密码不能为空",
		})
		return
	}
	if !Models.CaptchaVerify(c, loginInfo.Captcha) {
		c.JSON(http.StatusOK, gin.H{
			"status": false,
			"msg":    "验证码错误",
		})
		return
	}
	if Models.VerifyUser(loginInfo.Username, loginInfo.Password) {
		refreshToken, err := Middlewares.CreateRefreshToken(loginInfo.Username)
		if err != nil {
			panic(fmt.Sprintf("CreateRefreshToken err: %s", err))
		} else {
			c.SetCookie("refresh", refreshToken, Conf.RestfulConf.RefreshExpireTime, "/", Conf.RestfulConf.Domain, false, true)
			c.JSON(http.StatusOK, gin.H{
				"status": true,
				"msg":    "登录成功",
			})
		}
	} else {
		c.JSON(http.StatusOK, gin.H{
			"status": false,
			"msg":    "用户信息错误",
		})
	}
}

// ListUserAccess /api/user/:username 应该显示出该用户拥有的权限
func ListUserAccess(c *gin.Context) {
	userName := c.Param("username")
	var user Models.User
	Models.Db.Preload("Roles").First(&user, "user_name = ?", userName)
	var accesses []Models.Access
	for _, role := range user.Roles {
		for _, access := range role.Accesses {
			accesses = append(accesses, access)
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"status": true,
		"msg":    accesses,
	})
}

// RegisterGet - 获取注册页面
func RegisterGet(c *gin.Context) {
	//生成验证码 生成验证码图片 保存到session里
	c.JSON(http.StatusOK, gin.H{
		"status": true,
		"msg":    "获取注册页面成功",
	})

}

type RegisterInfo struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Tel      string `json:"tel"`
	Captcha  string `json:"captcha"`
}

func VerifyMobileFormat(mobileNum string) bool {
	regular := "^((13[0-9])|(14[5,7])|(15[0-3,5-9])|(17[0,3,5-8])|(18[0-9])|166|198|199|(147))\\d{8}$"

	reg := regexp.MustCompile(regular)
	return reg.MatchString(mobileNum)
}

// RegisterPost - 注册
func RegisterPost(c *gin.Context) {
	var registerInfo RegisterInfo
	c.BindJSON(&registerInfo)

	if registerInfo.Username == "" {
		c.JSON(http.StatusOK, gin.H{
			"status": false,
			"msg":    "用户名不能为空",
		})
		return
	}
	if registerInfo.Password == "" || len(registerInfo.Password) < 8 {
		c.JSON(http.StatusOK, gin.H{
			"status": false,
			"msg":    "密码不能为空",
		})
		return
	}
	if registerInfo.Tel == "" || !VerifyMobileFormat(registerInfo.Tel) {
		c.JSON(http.StatusOK, gin.H{
			"status": false,
			"msg":    "手机号不正确",
		})
		return
	}
	if !Models.CaptchaVerify(c, registerInfo.Captcha) {
		c.JSON(http.StatusOK, gin.H{
			"status": false,
			"msg":    "验证码错误",
		})
		return
	}
	if Models.RegisterOne(registerInfo.Username, registerInfo.Password, registerInfo.Tel) {
		c.JSON(http.StatusOK, gin.H{
			"status": true,
			"msg":    "注册成功",
		})
	} else {
		c.JSON(http.StatusOK, gin.H{
			"status": false,
			"msg":    "注册失败",
		})
		return
	}

}

func ResetPasswordGet(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": true,
		"msg":    "获取重置密码页面成功",
	})
}

type ResetInfo struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Tel      string `json:"tel"`
}

func ResetPasswordPost(c *gin.Context) {
	var resetInfo ResetInfo
	c.BindJSON(&resetInfo)

	if resetInfo.Username == "" {
		c.JSON(http.StatusOK, gin.H{
			"status": false,
			"msg":    "用户名不能为空",
		})
		return
	}
	if resetInfo.Password == "" {
		c.JSON(http.StatusOK, gin.H{
			"status": false,
			"msg":    "密码不能为空",
		})
		return
	}
	if resetInfo.Tel == "" {
		c.JSON(http.StatusOK, gin.H{
			"status": false,
			"msg":    "手机号不能为空",
		})
		return
	}
	if Models.ResetPassword(resetInfo.Username, resetInfo.Password, resetInfo.Tel) {
		c.JSON(http.StatusOK, gin.H{
			"status": true,
			"msg":    "重置密码成功",
		})
		// 重定向到login
		c.Redirect(http.StatusTemporaryRedirect, "/")
	} else {
		c.JSON(http.StatusOK, gin.H{
			"status": false,
			"msg":    "重置密码失败",
		})
	}
}

func ListUsers(c *gin.Context) {

	var users []Models.User
	Models.Db.Find(&users)
	var usernames []string
	for _, u := range users {
		usernames = append(usernames, u.UserName)
	}

	c.JSON(http.StatusOK, gin.H{
		"status": true,
		"msg": map[string][]string{
			"usernames": usernames,
		},
	})
}

func ListRoles(c *gin.Context) {

	var roles []Models.Role
	Models.Db.Find(&roles)
	roleNames := make([]string, 0, len(roles))
	for _, role := range roles {
		roleNames = append(roleNames, role.Grade)
	}

	c.JSON(http.StatusOK, gin.H{
		"status": true,
		"msg": map[string][]string{
			"rolenames": roleNames,
		},
	})
}

//c.JSON(http.StatusOK, gin.H{"hhh": 12314566})

func listAllRoles() []Models.Role {

	var roles []Models.Role
	Models.Db.Find(&roles)
	//if len(roles) == 0 {
	//	return nil
	//}

	return roles
}

func listAllAccesses() []Models.Access {

	var accesses []Models.Access
	Models.Db.Find(&accesses)
	return accesses
}

func ListUserRoles(c *gin.Context) {
	username := c.Param("user")

	roles := listAllRoles()
	var user Models.User
	Models.Db.Preload("Roles").First(&user, "user_name = ?", username)
	userRolesMap := make(map[string]bool, len(user.Roles))
	for _, r := range user.Roles {
		userRolesMap[r.Grade] = true
	}
	rs := make([]RoleList, 0, len(roles))
	for _, r := range roles {
		if _, ok := userRolesMap[r.Grade]; ok {
			rs = append(rs, RoleList{
				Get:  true,
				Role: r.Grade,
			})
		} else {
			rs = append(rs, RoleList{
				Get:  false,
				Role: r.Grade,
			})
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"status": true,
		"msg":    rs,
	})
}

type SaveNewUserRoles struct {
	UserName string     `json:"username"`
	Roles    []RoleList `json:"roles"`
}

func SaveUserRoles(c *gin.Context) {
	var saveNewUserRoles SaveNewUserRoles
	_ = c.BindJSON(&saveNewUserRoles)

	SaveUserRolesHelper(saveNewUserRoles.UserName, saveNewUserRoles.Roles)

}

func SaveUserRolesHelper(username string, roles []RoleList) {
	var oroles []Models.Role
	Models.Db.Find(&oroles)
	orolesMap := make(map[string]Models.Role)
	for _, o := range oroles {
		orolesMap[o.Grade] = o
	}
	var user Models.User
	//Models.Db.Preload("Roles").First(&user, "user_name = ?", username)
	Models.Db.Preload("Roles").First(&user, "user_name = ?", username)

	// 无论怎么样超级管理员都不会丢失超级管理员权限
	isSuper := false
	getSuper := false
	for _, r := range user.Roles {
		if r.Grade == Models.SuperRole {
			isSuper = true
		}
	}
	var tempArr = make([]Models.Role, 0)
	for _, r := range roles {
		if r.Get {
			tempArr = append(tempArr, orolesMap[r.Role])
			if r.Role == Models.SuperRole {
				getSuper = true
			}
		}
	}
	if isSuper && !getSuper {
		tempArr = append(tempArr, orolesMap[Models.SuperRole])
	}
	user.Roles = tempArr
	Models.Db.Model(&user).Association("Roles").Replace(tempArr)
	Models.Db.Save(&user)
}

func ListRoleAccesses(c *gin.Context) {
	rolegrade := c.Param("role")

	var role Models.Role
	Models.Db.Preload("Accesses").First(&role, "grade = ?", rolegrade)
	accesses := listAllAccesses()
	roleAccessesMap := make(map[string]bool)
	for _, s := range role.Accesses {
		roleAccessesMap[s.Hash] = true
	}

	rs := make([]AccessResponse, 0, len(accesses))
	for _, ac := range accesses {
		if _, ok := roleAccessesMap[ac.Hash]; ok {
			rs = append(rs, AccessResponse{
				Get:       true,
				Address:   fmt.Sprintf("%s:%d", ac.Host, ac.Port),
				User:      ac.User,
				OS:        ac.OS,
				TimeStamp: ac.TimeStamp,
				Hash:      ac.Hash,
			})
		} else {
			rs = append(rs, AccessResponse{
				Get:       false,
				User:      ac.User,
				OS:        ac.OS,
				TimeStamp: ac.TimeStamp,
				Address:   fmt.Sprintf("%s:%d", ac.Host, ac.Port),
				Hash:      ac.Hash,
			})
		}
	}
	//{status:...,msg:[{get:XX,server:XX,hash:XX},...]}
	c.JSON(http.StatusOK, gin.H{
		"status": true,
		"msg":    rs,
	})
}

func CreateRole(c *gin.Context) {
	var newrole Models.Role
	c.ShouldBindJSON(&newrole)
	Models.Db.First(&newrole, "grade = ?", newrole.Grade)
	if newrole.Grade == "" {
		Models.Db.Create(&newrole)
		Models.Db.Save(&newrole)
	} else {
		c.JSON(http.StatusOK, gin.H{
			"status": false,
			"msg":    "该角色已存在",
		})
	}
}

type SaveNewRoleAccesses struct {
	Rolename string           `json:"rolename"`
	Accesses []AccessResponse `json:"accesses"`
}

func SaveRoleAccesses(c *gin.Context) {
	var saveNewRoleAccesses SaveNewRoleAccesses
	c.BindJSON(&saveNewRoleAccesses)
	SaveRoleAccessesHelper(saveNewRoleAccesses.Rolename, saveNewRoleAccesses.Accesses)
	return
}

func SaveRoleAccessesHelper(rolegrade string, accesses []AccessResponse) {
	//数据库找出所有access
	var oaccesses []Models.Access
	Models.Db.Find(&oaccesses)
	oaccessesMap := make(map[string]Models.Access)
	for _, o := range oaccesses {
		oaccessesMap[o.Hash] = o
	}
	//
	var role Models.Role
	Models.Db.First(&role, "grade = ?", rolegrade)
	var tempArr = make([]Models.Access, 0)
	for _, a := range accesses {
		if a.Get {
			tempArr = append(tempArr, oaccessesMap[a.Hash])
		}
	}
	Models.Db.Model(&role).Association("Accesses").Replace(tempArr)
	Models.Db.Save(&role)
}
