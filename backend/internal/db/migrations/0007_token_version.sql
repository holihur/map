-- 0007_token_version.sql — Token 吊销支持（修改密码 / 主动登出会使其失效）
ALTER TABLE users ADD COLUMN IF NOT EXISTS token_version INTEGER NOT NULL DEFAULT 0;
