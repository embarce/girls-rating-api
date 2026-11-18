package service

import (
	"context"
	"strings"

	"girls-rating-api/internal/config"
	"girls-rating-api/internal/models"
)

// AuthorResponse 返回给前端展示的作者信息（{ id, name, avatar }，id 供前端作者专区跳转）。
// @Description 作者信息
type AuthorResponse struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
}

// PhotoResponse 统一照片响应结构：random / cosplay / 抽卡卡面共用（docs/design/photo-system.md §3.3）。
// rating 恢复浮点；rarity 输出档位名；author 形状不变（{name, avatar}），数据源为作者服务。
type PhotoResponse struct {
	ID          string         `json:"id,omitempty"` // random 池无 id 时省略
	ResourceURL string         `json:"resourceUrl"`
	Width       int            `json:"width"`
	Height      int            `json:"height"`
	Rating      float64        `json:"rating"`
	Views       string         `json:"views"`
	Rarity      string         `json:"rarity"` // "R" | "SR" | "SSR"
	Author      AuthorResponse `json:"author"`
}

// RarityName 档位列 → 档位名。
func RarityName(rarity int8) string {
	switch rarity {
	case models.RaritySSR:
		return "SSR"
	case models.RaritySR:
		return "SR"
	default:
		return "R"
	}
}

// ParseRarity 档位名 → 档位列；未识别返回 (0, false)。
func ParseRarity(name string) (int8, bool) {
	switch name {
	case "SSR", "ssr":
		return models.RaritySSR, true
	case "SR", "sr":
		return models.RaritySR, true
	case "R", "r":
		return models.RarityR, true
	default:
		return models.RarityR, false
	}
}

// DeriveRarity 按 rating 阈值派生档位（只在写入时调用，读取永不派生）。
// 阈值与 002 迁移回填 SQL 同源：>= SSRMinRating(9.0) → SSR，>= SRMinRating(7.0) → SR，否则 R。
func DeriveRarity(rating float64, cfg config.PhotoConfig) int8 {
	if rating >= cfg.SSRMinRating {
		return models.RaritySSR
	}
	if rating >= cfg.SRMinRating {
		return models.RaritySR
	}
	return models.RarityR
}

// ResolveRarity 显式定档优先，未指定则按 rating 派生（上传链路两条定档路径的汇聚点）。
func ResolveRarity(rating float64, explicit *int8, cfg config.PhotoConfig) int8 {
	if explicit != nil {
		switch *explicit {
		case models.RarityR, models.RaritySR, models.RaritySSR:
			return *explicit
		}
	}
	return DeriveRarity(rating, cfg)
}

// PhotoRowFromImage 从图片资源行构造池成员（随机池 / 抽卡池共用，v2 结构）。
func PhotoRowFromImage(item *models.ImageResource) models.PhotoRow {
	return models.PhotoRow{
		ResourceURL: item.ResourceURL,
		Width:       item.Width,
		Height:      item.Height,
		Rating:      item.Rating,
		Views:       item.Views,
		Rarity:      item.Rarity,
		AuthorID:    item.AuthorID,
	}
}

// BuildPhotoResponse 由池行组装响应（作者响应期解析，池里不冗余 name/avatar）。
func BuildPhotoResponse(ctx context.Context, row models.PhotoRow, authors *AuthorService, s3Host string) PhotoResponse {
	return PhotoResponse{
		ResourceURL: buildS3URL(row.ResourceURL, s3Host),
		Width:       row.Width,
		Height:      row.Height,
		Rating:      row.Rating,
		Views:       row.Views,
		Rarity:      RarityName(row.Rarity),
		Author:      authors.Resolve(ctx, row.AuthorID),
	}
}

// buildS3URL 如果 s3Host 不为空，则将资源路径拼接为完整 URL；否则直接返回原路径
func buildS3URL(resourceURL, s3Host string) string {
	if s3Host == "" {
		return resourceURL
	}
	// 避免双斜杠问题：s3Host 可能以 / 结尾，resourceURL 可能以 / 开头
	s3Host = strings.TrimSuffix(s3Host, "/")
	resourceURL = strings.TrimPrefix(resourceURL, "/")
	return s3Host + "/" + resourceURL
}
