-- 0009_external_layers.sql — 第三方图层（XYZ / WMS / WMTS / MVT / GeoJSON / ArcGIS）

CREATE TABLE IF NOT EXISTS external_layers (
    id             BIGSERIAL PRIMARY KEY,
    name           TEXT        NOT NULL UNIQUE,
    title          TEXT        NOT NULL DEFAULT '',
    description    TEXT        NOT NULL DEFAULT '',
    source_type    TEXT        NOT NULL,
    source_url     TEXT        NOT NULL,
    source_options JSONB       NOT NULL DEFAULT '{}'::jsonb,
    style          JSONB       NOT NULL DEFAULT '{}'::jsonb,
    metadata       JSONB       NOT NULL DEFAULT '{}'::jsonb,
    min_zoom       INTEGER     NOT NULL DEFAULT 0,
    max_zoom       INTEGER     NOT NULL DEFAULT 22,
    sort_order     INTEGER     NOT NULL DEFAULT 0,
    visibility     TEXT        NOT NULL DEFAULT 'public',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS external_layers_sort_idx ON external_layers (sort_order, name);
