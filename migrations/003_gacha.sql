-- 抽卡记录表（每日一抽 + 卡册快照）
-- 设计文档：docs/design/gacha-system.md §四.1
-- 图片表的 rarity 列与作者表在 002_photo_upgrade.sql。

SET NAMES utf8mb4;

CREATE TABLE IF NOT EXISTS `tb_girls_rating_gacha_draw` (
  `id` BIGINT NOT NULL COMMENT '雪花 ID',
  `device_id` VARCHAR(64) NOT NULL,
  `draw_date` DATE NOT NULL COMMENT '服务器本地日期（时区见 GACHA_TIMEZONE 配置）',
  `cards` JSON NOT NULL COMMENT '当日抽中的卡牌快照（GachaCardResponse 数组）',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_gacha_draw_device_date` (`device_id`, `draw_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
