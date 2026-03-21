package models

import (
	"time"
)

// ImageResource 图片资源模型，对应表 tb_girls_rating_image_resource
type ImageResource struct {
	ID          int64     `gorm:"primarykey;comment:'id'" json:"id"`
	CreateBy    string    `gorm:"column:create_by;type:varchar(64);not null;default:'';comment:'创建者'" json:"create_by"`
	CreateTime  time.Time `gorm:"column:create_time;not null;comment:'创建时间'" json:"create_time"`
	ResourceURL string    `gorm:"column:resource_url;type:varchar(500);default:null;comment:'图片地址'" json:"resource_url"`
	Width       int       `gorm:"column:width;default:null;comment:'宽度'" json:"width"`
	Height      int       `gorm:"column:height;default:null;comment:'高度'" json:"height"`
	AuthorID    int64     `gorm:"column:author_id;default:null;comment:'作者'" json:"author_id"`
	Rating      float64   `gorm:"column:rating;type:decimal(10,1);not null;default:4.0;comment:'评分'" json:"rating"`
	Views       string    `gorm:"column:views;type:varchar(255);not null;default:'1k';comment:'浏览量'" json:"views"`
}

// TableName 指定表名
func (ImageResource) TableName() string {
	return "tb_girls_rating_image_resource"
}

// ImageResourceRow 用于从 tb_girls_rating_image_resource 扫描的字段结果。
// 注意：该结构不包含业务外键/关联，仅服务于 /api/random 的响应拼装。
type ImageResourceRow struct {
	ResourceURL string `gorm:"column:resource_url"`
	Width       int    `gorm:"column:width"`
	Height      int    `gorm:"column:height"`
	Rating      int    `gorm:"column:rating"`
	Views       string `gorm:"column:views"`
}
