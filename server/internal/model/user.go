package model

// User represents an authenticated user.
// Auth token is NOT stored — middleware maps token → user UUID via FirstOrCreate.
type User struct {
	BaseModel
	Name string `gorm:"type:varchar(255);not null;default:''"`
}

func (User) TableName() string { return "users" }
