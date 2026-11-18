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
	Rarity      int8      `gorm:"column:rarity;not null;default:0;comment:'卡片等级 0=R 1=SR 2=SSR'" json:"rarity"`
}

// TableName 指定表名
func (ImageResource) TableName() string {
	return "tb_girls_rating_image_resource"
}

// Rarity 卡片等级常量：上传时定档（或按 rating 派生），读取永不派生。
// 权威定义见 docs/design/photo-system.md §四。
const (
	RarityR   int8 = 0
	RaritySR  int8 = 1
	RaritySSR int8 = 2
)

// PhotoRow 池成员结构（Redis Set 成员的 JSON 序列化对象）。
// 注意：结构升级时必须 bump 池 key 版本（v1 缺 rarity/authorId 字段，混池会产生
// 「全部 R 档 + 默认作者」的脏数据），见 docs/design/photo-system.md §4.2。
type PhotoRow struct {
	ResourceURL string  `json:"resourceUrl"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	Rating      float64 `json:"rating"`
	Views       string  `json:"views"`
	Rarity      int8    `json:"rarity"`
	AuthorID    int64   `json:"authorId"` // 响应期解析作者，池里不冗余 name/avatar
}
