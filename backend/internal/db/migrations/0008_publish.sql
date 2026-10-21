-- 0008_publish.sql — 图层排序/发布、访问控制、API Key、审计日志、地图分享

-- 图层排序与可见性
ALTER TABLE layers ADD COLUMN IF NOT EXISTS sort_order INTEGER NOT NULL DEFAULT 0;
ALTER TABLE layers ADD COLUMN IF NOT EXISTS visibility TEXT NOT NULL DEFAULT 'public';

CREATE INDEX IF NOT EXISTS layers_sort_idx ON layers (sort_order, name);

-- 按用户的图层授权（私有图层）
CREATE TABLE IF NOT EXISTS layer_access (
    layer_id  BIGINT      NOT NULL REFERENCES layers(id) ON DELETE CASCADE,
    user_id   BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    level     TEXT        NOT NULL DEFAULT 'read', -- read / write
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (layer_id, user_id)
);

-- 第三方只读 API Key（仅存 SHA-256 哈希）
CREATE TABLE IF NOT EXISTS api_keys (
    id           BIGSERIAL PRIMARY KEY,
    name         TEXT        NOT NULL,
    key_prefix   TEXT        NOT NULL,
    key_hash     TEXT        NOT NULL UNIQUE,
    role         TEXT        NOT NULL DEFAULT 'viewer',
    layer_name   TEXT,       -- 非空时仅允许访问指定图层
    created_by   BIGINT,
    revoked      BOOLEAN     NOT NULL DEFAULT false,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS api_keys_hash_idx ON api_keys (key_hash);

-- 写入审计
CREATE TABLE IF NOT EXISTS audit_logs (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT,
    username   TEXT,
    role       TEXT,
    method     TEXT        NOT NULL,
    path       TEXT        NOT NULL,
    status     INTEGER     NOT NULL DEFAULT 0,
    layer      TEXT,
    ip         TEXT,
    detail     JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS audit_logs_created_idx ON audit_logs (created_at DESC);
CREATE INDEX IF NOT EXISTS audit_logs_layer_idx ON audit_logs (layer, created_at DESC);

-- 分享 / 保存的地图状态
CREATE TABLE IF NOT EXISTS map_states (
    id         BIGSERIAL PRIMARY KEY,
    token      TEXT        NOT NULL UNIQUE,
    name       TEXT        NOT NULL DEFAULT '',
    state      JSONB       NOT NULL,
    created_by BIGINT,
    is_public  BOOLEAN     NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS map_states_token_idx ON map_states (token);
