package databases

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var DB *gorm.DB

func ConnectDatabase() *gorm.DB {
	if DB == nil {
		database, err := gorm.Open(sqlite.Open("db.sqlite3"), &gorm.Config{})
		if err != nil {
			panic("Failed to connect to database!")
		}
		DB = database
	}
	return DB
}
