package user

import (
	"os/user"

	"github.com/WangYihang/Platypus/internal/databases"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	ID       string `json:"ID" gorm:"primaryKey"`
	Username string `json:"username"`
	Password string `json:"-"`
	Role     string `json:"role"`
}

func (u *User) BeforeCreate(tx *gorm.DB) (err error) {
	u.ID = uuid.New().String()
	return nil
}

func FindUserByUsername(username string) *User {
	var user = User{}
	databases.DB.Model(user).Where("username = ?", username).First(&user)
	return &user
}

func CheckUserExists(username string) bool {
	var user = User{}
	result := databases.DB.Model(user).Where("username = ?", username).First(&user)
	return result.Error == nil
}

func Authenticate(username string, password string) bool {
	user := FindUserByUsername(username)
	return CheckPasswordHash(password, user.Password)
}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func GetAllUsers() *[]user.User {
	var users []user.User
	databases.DB.Find(&users)
	return &users
}
