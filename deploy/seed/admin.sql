-- 初始管理员种子。M1 接入用户表后，此文件会被真实 SQL 替换。
-- 当前 M0 阶段：仅记录 seed 元信息。
INSERT INTO schema_meta (key, value) VALUES ('seed_applied', 'admin@v0.1.0')
    ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW();
