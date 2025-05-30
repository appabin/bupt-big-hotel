package models

// User 用户表
type User struct {
	ID       int    `gorm:"primary_key;auto_increment"`
	Username string `gorm:"type:varchar(255);unique;not null"`
	Password string `gorm:"type:varchar(255);not null"`
	Identity string `gorm:"type:varchar(255);not null"` // customer, administrator
}
