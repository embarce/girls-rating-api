package models

import (
	"time"
)

// GachaDraw 抽卡记录模型，对应表 tb_girls_rating_gacha_draw。
// 一台设备一天一行：唯一索引 uq(device_id, draw_date) 既做每日幂等，
// cards JSON 快照同时充当卡册存储（图片后续被删也不影响卡册）。
type GachaDraw struct {
	ID        int64     `gorm:"primarykey;column:id" json:"id"`
	DeviceID  string    `gorm:"column:device_id;type:varchar(64);not null;uniqueIndex:uq_gacha_draw_device_date,priority:1" json:"device_id"`
	DrawDate  string    `gorm:"column:draw_date;type:date;not null;uniqueIndex:uq_gacha_draw_device_date,priority:2" json:"draw_date"`
	Cards     string    `gorm:"column:cards;type:json;not null" json:"cards"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

// TableName 指定表名
func (GachaDraw) TableName() string {
	return "tb_girls_rating_gacha_draw"
}
