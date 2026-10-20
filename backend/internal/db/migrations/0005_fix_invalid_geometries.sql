-- 0005_fix_invalid_geometries.sql — 修复演示数据中的非法（自相交）几何
-- 栅格化 / 空间相交运算要求几何有效；ST_MakeValid 可能把自相交面变成 MultiPolygon，
-- 因此同步调整列类型与图层元数据。

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM layers WHERE table_name = 'layer_countries') THEN
        ALTER TABLE layer_countries
            ALTER COLUMN geom TYPE geometry(MultiPolygon, 4326)
            USING ST_Multi(ST_MakeValid(geom));
        UPDATE layers SET geometry_type = 'MULTIPOLYGON' WHERE table_name = 'layer_countries';
    END IF;

    IF EXISTS (SELECT 1 FROM layers WHERE table_name = 'layer_cities') THEN
        ALTER TABLE layer_cities
            ALTER COLUMN geom TYPE geometry(Point, 4326)
            USING ST_MakeValid(geom);
    END IF;
END
$$;
