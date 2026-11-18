package models

import (
	"time"

	"gorm.io/gorm"
)

// Author 作者实体模型，对应表 authors。
// 作者由管理端维护（面向爬虫自动化：幂等创建、软删除释放唯一槽位），
// 与 users 登录体系无关。
type Author struct {
	ID        int            `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
	Name      string         `gorm:"type:varchar(50);not null;uniqueIndex" json:"name"`
	Avatar    string         `gorm:"type:varchar(500);default:null" json:"avatar"`
}

// TableName 指定表名
func (Author) TableName() string {
	return "authors"
}
