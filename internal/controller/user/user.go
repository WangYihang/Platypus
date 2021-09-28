package user

import (
	"errors"

	"github.com/WangYihang/Platypus/internal/util/str"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	Token string
}

func connect() *gorm.DB {
	db, err := gorm.Open(sqlite.Open("platypus.db"), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}
	return db
}

func Create() {
	db := connect()
	db.AutoMigrate(&User{})
	db.Create(&User{Token: str.RandomString(0x32)})
}

func Auth(token string) bool {
	var user User
	db := connect()
	result := db.First(&user, "token = ?", token)
	return errors.Is(result.Error, gorm.ErrRecordNotFound)
}
