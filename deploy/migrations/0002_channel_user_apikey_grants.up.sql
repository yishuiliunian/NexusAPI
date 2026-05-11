-- 0002 新增「渠道-用户」「渠道-ApiKey」两张白名单关联表。
--
-- 语义：与 channel_groups 对称（多对多关联）。
--   - channel_users   存渠道对用户级白名单
--   - channel_apikeys 存渠道对 ApiKey 级白名单
--   - 空 = 该层不限制；非空 = 仅列表中实体可用
--
-- 开发环境通常依赖 GORM AutoMigrate，此文件用于生产 migrate CLI 路径。

CREATE TABLE IF NOT EXISTS channel_users (
    channel_id  BIGINT       NOT NULL,
    user_id     BIGINT       NOT NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (channel_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_channel_users_user_id ON channel_users (user_id);

CREATE TABLE IF NOT EXISTS channel_apikeys (
    channel_id  BIGINT       NOT NULL,
    apikey_id   BIGINT       NOT NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (channel_id, apikey_id)
);

CREATE INDEX IF NOT EXISTS idx_channel_apikeys_apikey_id ON channel_apikeys (apikey_id);
