-- 回退 0002：移除两张白名单关联表。
DROP INDEX IF EXISTS idx_channel_apikeys_apikey_id;
DROP TABLE IF EXISTS channel_apikeys;

DROP INDEX IF EXISTS idx_channel_users_user_id;
DROP TABLE IF EXISTS channel_users;
