package models

import (
	"time"

	"gorm.io/gorm"
)

// User 用户模型
type User struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	Username string `gorm:"size:50;uniqueIndex;not null" json:"username" validate:"required,min=3,max=50"`
	Email    string `gorm:"size:100;uniqueIndex;not null" json:"email" validate:"required,email"`
	Password string `gorm:"size:255;not null" json:"-"`
	Nickname string `gorm:"size:50" json:"nickname"`
	Avatar   string `gorm:"size:255" json:"avatar"`
	Status   int    `gorm:"default:1" json:"status"` // 1: 正常，0: 禁用
}

// TableName 指定表名
func (User) TableName() string {
	return "users"
}
