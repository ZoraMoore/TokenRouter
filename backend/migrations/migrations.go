package migrations

import "embed"

// FS 包含本目录下所有嵌入的 SQL 迁移文件。
//
// 迁移命名规范：
//   - 使用零填充的数字前缀确保正确的执行顺序
//   - 格式：NNN_description.sql（如 001_init.sql, 002_add_users.sql）
//   - 描述部分使用下划线分隔的小写单词
//
// 迁移文件要求：
//   - 必须是幂等的（可重复执行而不产生错误）
//   - 推荐使用 IF NOT EXISTS / IF EXISTS 语法
//   - 一旦应用，不应修改已有的迁移文件（通过 checksum 校验）
//
// 示例迁移文件：
//
//	-- 001_init.sql
//	CREATE TABLE IF NOT EXISTS users (
//	    id BIGSERIAL PRIMARY KEY,
//	    email VARCHAR(255) NOT NULL UNIQUE,
//	    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
//	);
//
//go:embed *.sql
var FS embed.FS
