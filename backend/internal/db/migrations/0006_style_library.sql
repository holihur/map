-- 0006_style_library.sql — SLD 样式库与图层默认样式

ALTER TABLE styles ADD COLUMN IF NOT EXISTS sld_xml TEXT;
ALTER TABLE layers ADD COLUMN IF NOT EXISTS style_name TEXT;
