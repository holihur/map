-- 0004_raster.sql — 启用 PostGIS 栅格扩展（WMS 栅格出图依赖 ST_AsRaster / ST_AsPNG）
-- PostGIS 3.x 起栅格功能被拆分到独立的 postgis_raster 扩展。
CREATE EXTENSION IF NOT EXISTS postgis_raster;
