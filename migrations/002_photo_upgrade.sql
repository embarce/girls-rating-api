-- 照片体系改造（作者实体化 + 评分定档）
-- 设计文档：docs/design/photo-system.md §三.1
-- Docker Compose 在初始化 MySQL 数据目录时会自动执行此目录下的 .sql 文件；
-- 已有数据目录需手动执行本文件。

SET NAMES utf8mb4;

-- 作者实体表
CREATE TABLE IF NOT EXISTS `authors` (
  `id` INT UNSIGNED NOT NULL AUTO_INCREMENT,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  `name` VARCHAR(50) NOT NULL,
  `avatar` VARCHAR(500) NULL DEFAULT NULL COMMENT '头像 URL（R2 或外链）',
  `deleted_at` DATETIME(3) NULL DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uq_authors_name` (`name`),
  KEY `idx_authors_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- seed 默认作者（与旧硬编码一致，保证升级零感知）
INSERT INTO `authors` (`id`, `name`, `avatar`)
  SELECT 1, 'Embrace', 'https://www.girls-rating.com/images/avatars/avatar1.webp'
  WHERE NOT EXISTS (SELECT 1 FROM `authors` WHERE `id` = 1);

-- 图片表：作者兜底指向默认作者
UPDATE `tb_girls_rating_image_resource` SET `author_id` = 1 WHERE `author_id` IS NULL OR `author_id` = 0;

-- 卡片等级列（0=R 1=SR 2=SSR）
ALTER TABLE `tb_girls_rating_image_resource`
  ADD COLUMN `rarity` TINYINT NOT NULL DEFAULT 0 COMMENT '卡片等级 0=R 1=SR 2=SSR' AFTER `rating`;

-- 存量定档回填（规则见 photo-system.md §四：rating>=9 → SSR，rating>=7 → SR）
UPDATE `tb_girls_rating_image_resource` SET `rarity` = 2 WHERE `rating` >= 9;
UPDATE `tb_girls_rating_image_resource` SET `rarity` = 1 WHERE `rating` >= 7 AND `rarity` = 0;
