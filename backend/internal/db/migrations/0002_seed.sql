-- 0002_seed.sql — 演示图层（国界多边形与城市点位）及默认样式
-- 注意：示例几何为高度简化的概略形状，仅用于功能演示，不具备制图精度。

-- ── 物理表：国家面 ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS layer_countries (
    fid         BIGSERIAL PRIMARY KEY,
    name        TEXT,
    iso         TEXT,
    population  BIGINT,
    geom        geometry(POLYGON, 4326),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS layer_countries_geom_gix ON layer_countries USING GIST (geom);

INSERT INTO layer_countries (name, iso, population, geom) VALUES
('中国', 'CN', 1412000000, ST_GeomFromText('POLYGON((73 39,80 45,90 45,96 43,110 45,119 50,127 50,134 48,130 42,122 39,121 31,122 22,110 21,98 24,97 28,88 28,79 33,73 39))', 4326)),
('巴西', 'BR', 216000000, ST_GeomFromText('POLYGON((-73 5,-60 5,-50 0,-35 -5,-38 -15,-48 -25,-55 -30,-58 -38,-63 -30,-70 -20,-73 5))', 4326)),
('澳大利亚', 'AU', 26000000, ST_GeomFromText('POLYGON((114 -22,122 -18,130 -12,142 -11,146 -19,153 -25,150 -37,140 -38,130 -32,115 -34,114 -22))', 4326)),
('法国', 'FR', 68000000, ST_GeomFromText('POLYGON((-4 48,-1 49,3 51,7 49,7 44,3 43,-2 43,-4 48))', 4326)),
('日本', 'JP', 124000000, ST_GeomFromText('POLYGON((130 31,132 34,136 36,141 41,145 43,143 44,140 38,137 36,133 33,130 31))', 4326))
ON CONFLICT DO NOTHING;

-- ── 物理表：城市点 ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS layer_cities (
    fid         BIGSERIAL PRIMARY KEY,
    name        TEXT,
    country     TEXT,
    population  BIGINT,
    capital     BOOLEAN,
    geom        geometry(POINT, 4326),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS layer_cities_geom_gix ON layer_cities USING GIST (geom);

INSERT INTO layer_cities (name, country, population, capital, geom) VALUES
('北京', '中国', 21890000, true,  ST_SetSRID(ST_MakePoint(116.4074, 39.9042), 4326)),
('上海', '中国', 24870000, false, ST_SetSRID(ST_MakePoint(121.4737, 31.2304), 4326)),
('东京', '日本', 13960000, true,  ST_SetSRID(ST_MakePoint(139.6917, 35.6895), 4326)),
('巴黎', '法国', 2148000,  true,  ST_SetSRID(ST_MakePoint(2.3522, 48.8566), 4326)),
('伦敦', '英国', 8982000,  true,  ST_SetSRID(ST_MakePoint(-0.1276, 51.5072), 4326)),
('纽约', '美国', 8336000,  false, ST_SetSRID(ST_MakePoint(-74.0060, 40.7128), 4326)),
('悉尼', '澳大利亚', 5312000, false, ST_SetSRID(ST_MakePoint(151.2093, -33.8688), 4326)),
('圣保罗', '巴西', 12330000, false, ST_SetSRID(ST_MakePoint(-46.6333, -23.5505), 4326)),
('开罗', '埃及', 9540000,  true,  ST_SetSRID(ST_MakePoint(31.2357, 30.0444), 4326)),
('莫斯科', '俄罗斯', 12500000, true, ST_SetSRID(ST_MakePoint(37.6173, 55.7558), 4326))
ON CONFLICT DO NOTHING;

-- ── 图层元数据 ─────────────────────────────────────────────────
INSERT INTO layers (name, title, description, geometry_type, srid, table_name, fields, min_zoom, max_zoom, bounds, style)
VALUES
(
  'countries', '国家（面）', '示例：高度简化的国家边界多边形', 'POLYGON', 4326, 'layer_countries',
  '[{"name":"name","type":"text"},{"name":"iso","type":"text"},{"name":"population","type":"bigint"}]'::jsonb,
  0, 10,
  ST_MakeEnvelope(-180, -90, 180, 90, 4326),
  '{"type":"fill","paint":{"fill-color":"#6366f1","fill-opacity":0.45,"fill-outline-color":"#4338ca"}}'::jsonb
),
(
  'cities', '城市（点）', '示例：世界主要城市点位', 'POINT', 4326, 'layer_cities',
  '[{"name":"name","type":"text"},{"name":"country","type":"text"},{"name":"population","type":"bigint"},{"name":"capital","type":"boolean"}]'::jsonb,
  2, 22,
  ST_MakeEnvelope(-180, -90, 180, 90, 4326),
  '{"type":"circle","paint":{"circle-radius":5,"circle-color":"#f97316","circle-stroke-color":"#ffffff","circle-stroke-width":1.5}}'::jsonb
)
ON CONFLICT (name) DO NOTHING;

-- ── 默认样式 ───────────────────────────────────────────────────
INSERT INTO styles (name, is_default, style)
VALUES
('MapServer Light', true, '{"version":8,"name":"MapServer Light","sources":{},"layers":[{"id":"background","type":"background","paint":{"background-color":"#f8fafc"}}],"metadata":{"palette":{"text":"#0f172a"}}}'::jsonb),
('MapServer Dark', false, '{"version":8,"name":"MapServer Dark","sources":{},"layers":[{"id":"background","type":"background","paint":{"background-color":"#0f172a"}}],"metadata":{"palette":{"text":"#e2e8f0"}}}'::jsonb)
ON CONFLICT (name) DO NOTHING;
