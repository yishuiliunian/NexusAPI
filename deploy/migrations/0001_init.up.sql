-- NexusAPI 初始 schema（M0 占位）。
-- 真实表结构在 M1 里程碑按 domain 模型补全。
-- 为保证 docker compose up 首次启动时迁移能成功，这里先创建一张元信息表。

CREATE TABLE IF NOT EXISTS schema_meta (
    key   VARCHAR(64) PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO schema_meta (key, value) VALUES ('schema_version', '0001_init')
    ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW();
