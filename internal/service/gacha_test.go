package service

import (
	"testing"

	"girls-rating-api/internal/config"
	"girls-rating-api/internal/models"
)

// 验收（gacha-system.md §八）：mock 后 1000+ 次抽样分布 ≈ 6/24/70（±3%）
func TestRollRarityDistribution(t *testing.T) {
	s := &GachaService{cfg: config.GachaConfig{
		SSRWeight: 6,
		SRWeight:  24,
		RWeight:   70,
	}}

	const total = 100000
	counts := map[int8]int{}
	for i := 0; i < total; i++ {
		counts[s.rollRarity()]++
	}

	ratio := func(r int8) float64 { return float64(counts[r]) / total * 100 }

	if got := ratio(models.RaritySSR); got < 3 || got > 9 {
		t.Errorf("SSR ratio = %.2f%%, want 6%% ± 3%%", got)
	}
	if got := ratio(models.RaritySR); got < 21 || got > 27 {
		t.Errorf("SR ratio = %.2f%%, want 24%% ± 3%%", got)
	}
	if got := ratio(models.RarityR); got < 67 || got > 73 {
		t.Errorf("R ratio = %.2f%%, want 70%% ± 3%%", got)
	}
}

// 验收（photo-system.md §四）：阈值派生 9/7 与 002 迁移回填 SQL 同源
func TestDeriveRarity(t *testing.T) {
	cfg := config.PhotoConfig{SSRMinRating: 9.0, SRMinRating: 7.0}

	cases := []struct {
		rating float64
		want   int8
	}{
		{10.0, models.RaritySSR},
		{9.0, models.RaritySSR}, // 边界含等号
		{8.9, models.RaritySR},
		{7.0, models.RaritySR}, // 边界含等号
		{6.9, models.RarityR},
		{4.0, models.RarityR},
	}

	for _, c := range cases {
		if got := DeriveRarity(c.rating, cfg); got != c.want {
			t.Errorf("DeriveRarity(%v) = %d, want %d", c.rating, got, c.want)
		}
	}
}

func TestResolveRarity(t *testing.T) {
	cfg := config.PhotoConfig{SSRMinRating: 9.0, SRMinRating: 7.0}

	// 显式指定覆盖派生
	explicit := int8(models.RarityR)
	if got := ResolveRarity(9.5, &explicit, cfg); got != models.RarityR {
		t.Errorf("ResolveRarity explicit override = %d, want R(%d)", got, models.RarityR)
	}

	// 未指定走派生
	if got := ResolveRarity(9.5, nil, cfg); got != models.RaritySSR {
		t.Errorf("ResolveRarity derive = %d, want SSR(%d)", got, models.RaritySSR)
	}

	// 非法显式值回退派生
	invalid := int8(9)
	if got := ResolveRarity(4.0, &invalid, cfg); got != models.RarityR {
		t.Errorf("ResolveRarity invalid explicit = %d, want R(%d)", got, models.RarityR)
	}
}

func TestRarityNameAndParse(t *testing.T) {
	for rarity, name := range map[int8]string{
		models.RarityR:   "R",
		models.RaritySR:  "SR",
		models.RaritySSR: "SSR",
	} {
		if got := RarityName(rarity); got != name {
			t.Errorf("RarityName(%d) = %q, want %q", rarity, got, name)
		}
		parsed, ok := ParseRarity(name)
		if !ok || parsed != rarity {
			t.Errorf("ParseRarity(%q) = %d,%v, want %d,true", name, parsed, ok, rarity)
		}
	}

	if _, ok := ParseRarity("XX"); ok {
		t.Errorf("ParseRarity(XX) should not parse")
	}
}

// 幂等补发：同日快照统计与卡面 rarity 一致（纯内存部分）
func TestSummarizeClientCards(t *testing.T) {
	cards := []GachaCardResponse{
		{PhotoResponse: PhotoResponse{Rarity: "SSR"}},
		{PhotoResponse: PhotoResponse{Rarity: "SR"}},
		{PhotoResponse: PhotoResponse{Rarity: "SR"}},
		{PhotoResponse: PhotoResponse{Rarity: "R"}},
	}
	stats := summarize(cards)
	if stats.TotalCards != 4 || stats.SSR != 1 || stats.SR != 2 || stats.R != 1 {
		t.Errorf("summarize = %+v", stats)
	}
}
