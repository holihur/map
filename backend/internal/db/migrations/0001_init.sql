-- 0001_init.sql — 启用 PostGIS 并建立图层 / 样式元数据表

CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- 图层元数据。空间要素存放在物理表 layers.table_name 中。
CREATE TABLE IF NOT EXISTS layers (
    id            BIGSERIAL PRIMARY KEY,
    name          TEXT        NOT NULL UNIQUE,
    title         TEXT        NOT NULL DEFAULT '',
    description   TEXT        NOT NULL DEFAULT '',
    geometry_type TEXT        NOT NULL DEFAULT 'GEOMETRY',
    srid          INTEGER     NOT NULL DEFAULT 4326,
    table_name    TEXT        NOT NULL UNIQUE,
    fields        JSONB       NOT NULL DEFAULT '[]'::jsonb,
    min_zoom      INTEGER     NOT NULL DEFAULT 0,
    max_zoom      INTEGER     NOT NULL DEFAULT 22,
    bounds        geometry(Polygon, 4326),
    style         JSONB       NOT NULL DEFAULT '{}'::jsonb,
    metadata      JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS layers_bounds_gix ON layers USING GIST (bounds);

-- MapLibre Style 规范样式。
CREATE TABLE IF NOT EXISTS styles (
    id         BIGSERIAL PRIMARY KEY,
    name       TEXT        NOT NULL UNIQUE,
    is_default BOOLEAN     NOT NULL DEFAULT false,
    style      JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
