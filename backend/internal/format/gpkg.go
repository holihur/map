package format

import (
	"bytes"
	"database/sql"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	_ "modernc.org/sqlite"

	"mapserver/internal/models"
)

// ── GeoJSON → WKB ───────────────────────────────────────────────

func wkbHeader(b *bytes.Buffer, typ uint32) {
	b.WriteByte(1) // little endian
	var tmp [4]byte
	binary.LittleEndian.PutUint32(tmp[:], typ)
	b.Write(tmp[:])
}

func wkbPoint(b *bytes.Buffer, c []any) {
	var tmp [8]byte
	binary.LittleEndian.PutUint64(tmp[:], math.Float64bits(fnum(c[0])))
	b.Write(tmp[:])
	binary.LittleEndian.PutUint64(tmp[:], math.Float64bits(fnum(c[1])))
	b.Write(tmp[:])
}

func wkbLine(b *bytes.Buffer, cs [][]any) {
	var tmp [4]byte
	binary.LittleEndian.PutUint32(tmp[:], uint32(len(cs)))
	b.Write(tmp[:])
	for _, c := range cs {
		wkbPoint(b, c)
	}
}

// GeometryToWKB 将 GeoJSON 几何编码为 WKB（用于 GeoPackage）。
func GeometryToWKB(g map[string]any) ([]byte, error) {
	if g == nil {
		return nil, nil
	}
	var b bytes.Buffer
	typ, _ := g["type"].(string)
	switch typ {
	case "Point":
		wkbHeader(&b, 1)
		wkbPoint(&b, coord(g["coordinates"]))
	case "LineString":
		wkbHeader(&b, 2)
		wkbLine(&b, coords(g["coordinates"]))
	case "Polygon":
		wkbHeader(&b, 3)
		rings := coords2(g["coordinates"])
		writeUint32(&b, uint32(len(rings)))
		for _, r := range rings {
			wkbLine(&b, r)
		}
	case "MultiPoint":
		wkbHeader(&b, 4)
		pts := coords(g["coordinates"])
		writeUint32(&b, uint32(len(pts)))
		for _, p := range pts {
			wkbHeader(&b, 1)
			wkbPoint(&b, p)
		}
	case "MultiLineString":
		wkbHeader(&b, 5)
		lines := coords2(g["coordinates"])
		writeUint32(&b, uint32(len(lines)))
		for _, l := range lines {
			wkbHeader(&b, 2)
			wkbLine(&b, l)
		}
	case "MultiPolygon":
		wkbHeader(&b, 6)
		polys := coords3(g["coordinates"])
		writeUint32(&b, uint32(len(polys)))
		for _, p := range polys {
			wkbHeader(&b, 3)
			writeUint32(&b, uint32(len(p)))
			for _, r := range p {
				wkbLine(&b, r)
			}
		}
	default:
		return nil, fmt.Errorf("不支持的几何类型 %q", typ)
	}
	return b.Bytes(), nil
}

func writeUint32(b *bytes.Buffer, v uint32) {
	var tmp [4]byte
	binary.LittleEndian.PutUint32(tmp[:], v)
	b.Write(tmp[:])
}

// gpkgBlob 组装 GeoPackage 几何二进制（无 envelope）。
func gpkgBlob(srid int, wkb []byte) []byte {
	hdr := make([]byte, 8)
	hdr[0], hdr[1] = 'G', 'P'
	hdr[2] = 0    // version
	hdr[3] = 0x01 // little-endian, envelope indicator = 0
	binary.LittleEndian.PutUint32(hdr[4:], uint32(srid))
	return append(hdr, wkb...)
}

// parseGPKGBlob 拆出 SRID 与 WKB。
func parseGPKGBlob(blob []byte) (int, []byte, error) {
	if len(blob) < 8 || blob[0] != 'G' || blob[1] != 'P' {
		return 0, blob, nil // 可能是裸 WKB
	}
	flags := blob[3]
	srid := int(int32(binary.LittleEndian.Uint32(blob[4:8])))
	env := (flags >> 1) & 0x07
	off := 8
	switch env {
	case 1:
		off += 32
	case 2, 3:
		off += 48
	case 4:
		off += 64
	}
	if off > len(blob) {
		return 0, nil, fmt.Errorf("GeoPackage 几何头非法")
	}
	return srid, blob[off:], nil
}

// ── 导出 ────────────────────────────────────────────────────────

// GPKGSource 一个待写入 GeoPackage 的图层。
type GPKGSource struct {
	Name         string
	SRID         int
	GeometryType string
	Fields       []models.Field
	Features     []Feature
	SRSWKT       string // 可选：SRS 的真实 WKT 定义
}

// WriteGeoPackage 将要素导出为 GeoPackage（单图层）。
func WriteGeoPackage(w io.Writer, name string, srid int, geomType string, fields []models.Field, features []Feature) error {
	return WriteGeoPackageMulti(w, []GPKGSource{{
		Name: name, SRID: srid, GeometryType: geomType, Fields: fields, Features: features,
	}})
}

// WriteGeoPackageMulti 将多个图层写入同一个 GeoPackage 文件。
func WriteGeoPackageMulti(w io.Writer, sources []GPKGSource) error {
	tmp, err := os.CreateTemp("", "mapserver-*.gpkg")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(tmpPath) }()

	db, err := sql.Open("sqlite", tmpPath)
	if err != nil {
		return err
	}
	defer db.Close()

	stmts := []string{
		`PRAGMA application_id = 1196444487`,
		`PRAGMA user_version = 10300`,
		`CREATE TABLE gpkg_spatial_ref_sys (srs_name TEXT NOT NULL, srs_id INTEGER PRIMARY KEY, organization TEXT NOT NULL, organization_coordsys_id INTEGER NOT NULL, definition TEXT NOT NULL, description TEXT)`,
		`CREATE TABLE gpkg_contents (table_name TEXT NOT NULL PRIMARY KEY, data_type TEXT NOT NULL, identifier TEXT UNIQUE, description TEXT DEFAULT '', last_change DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')), min_x DOUBLE, min_y DOUBLE, max_x DOUBLE, max_y DOUBLE, srs_id INTEGER)`,
		`CREATE TABLE gpkg_geometry_columns (table_name TEXT NOT NULL, column_name TEXT NOT NULL, geometry_type_name TEXT NOT NULL, srs_id INTEGER NOT NULL, z TINYINT NOT NULL, m TINYINT NOT NULL, CONSTRAINT pk_geom_cols PRIMARY KEY (table_name, column_name))`,
		`INSERT INTO gpkg_spatial_ref_sys VALUES ('Undefined cartesian SRS', -1, 'NONE', -1, 'undefined', NULL)`,
		`INSERT INTO gpkg_spatial_ref_sys VALUES ('Undefined geographic SRS', 0, 'NONE', 0, 'undefined', NULL)`,
		`INSERT INTO gpkg_spatial_ref_sys VALUES ('WGS 84 geodetic', 4326, 'EPSG', 4326, 'GEOGCS["WGS 84",DATUM["WGS_1984",SPHEROID["WGS 84",6378137,298.257223563]],PRIMEM["Greenwich",0],UNIT["degree",0.0174532925199433]]', NULL)`,
	}
	for _, s := range sources {
		def := strings.TrimSpace(s.SRSWKT)
		if def == "" {
			def = wktForKey(s.SRID)
		}
		stmts = append(stmts, fmt.Sprintf(
			`INSERT OR IGNORE INTO gpkg_spatial_ref_sys VALUES ('EPSG:%d', %d, 'EPSG', %d, '%s', NULL)`,
			s.SRID, s.SRID, s.SRID, strings.ReplaceAll(def, "'", "''")))
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("初始化 GeoPackage 失败: %w", err)
		}
	}

	for _, src := range sources {
		if err := writeGPKGTable(db, src); err != nil {
			return err
		}
	}

	if err := db.Close(); err != nil {
		return err
	}
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func writeGPKGTable(db *sql.DB, src GPKGSource) error {
	colDefs := []string{`fid INTEGER PRIMARY KEY AUTOINCREMENT`, `geom BLOB`}
	colNames := []string{"fid", "geom"}
	for _, f := range src.Fields {
		colDefs = append(colDefs, fmt.Sprintf(`"%s" %s`, f.Name, gpkgColumnType(f.Type)))
		colNames = append(colNames, f.Name)
	}
	createSQL := fmt.Sprintf(`CREATE TABLE "%s" (%s)`, src.Name, strings.Join(colDefs, ", "))
	if _, err := db.Exec(createSQL); err != nil {
		return err
	}

	gtype := strings.ToUpper(src.GeometryType)
	if gtype == "" {
		gtype = "GEOMETRY"
	}
	if _, err := db.Exec(
		`INSERT INTO gpkg_contents (table_name, data_type, identifier, srs_id) VALUES (?, 'features', ?, ?)`,
		src.Name, src.Name, src.SRID); err != nil {
		return err
	}
	if _, err := db.Exec(
		`INSERT INTO gpkg_geometry_columns (table_name, column_name, geometry_type_name, srs_id, z, m) VALUES (?, 'geom', ?, ?, 0, 0)`,
		src.Name, gtype, src.SRID); err != nil {
		return err
	}

	placeholders := make([]string, len(colNames))
	for i := range placeholders {
		placeholders[i] = "?"
	}
	insertSQL := fmt.Sprintf(`INSERT INTO "%s" (%s) VALUES (%s)`,
		src.Name, `"`+strings.Join(colNames, `","`)+`"`, strings.Join(placeholders, ","))

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(insertSQL)
	if err != nil {
		return err
	}
	for _, feat := range src.Features {
		wkb, err := GeometryToWKB(feat.Geometry)
		if err != nil {
			continue
		}
		args := make([]any, 0, len(colNames))
		args = append(args, nil, gpkgBlob(src.SRID, wkb))
		for _, f := range src.Fields {
			args = append(args, gpkgValue(feat.Properties[f.Name]))
		}
		if _, err := stmt.Exec(args...); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			return err
		}
	}
	_ = stmt.Close()
	return tx.Commit()
}

// wktForKey 提供一个最小可用的 SRS 定义（真实定义可从 spatial_ref_sys 获取）。
func wktForKey(srid int) string {
	return fmt.Sprintf(`PROJCS["EPSG:%d",GEOGCS["WGS 84",DATUM["WGS_1984",SPHEROID["WGS 84",6378137,298.257223563]],PRIMEM["Greenwich",0],UNIT["degree",0.0174532925199433]],UNIT["metre",1]]`, srid)
}

func gpkgColumnType(t models.FieldType) string {
	switch strings.ToLower(string(t)) {
	case "integer", "bigint", "boolean":
		return "INTEGER"
	case "double":
		return "REAL"
	case "date", "timestamptz":
		return "DATETIME"
	default:
		return "TEXT"
	}
}

func gpkgValue(v any) any {
	switch n := v.(type) {
	case float64:
		if n == float64(int64(n)) {
			return int64(n)
		}
		return n
	default:
		return v
	}
}

// ── 导入 ────────────────────────────────────────────────────────

// ReadGeoPackage 读取 GeoPackage 的第一个要素表。
func ReadGeoPackage(r io.Reader) ([]ImportFeature, error) {
	return ReadGeoPackageTable(r, "")
}

// ListGeoPackageTables 列出 GeoPackage 中的要素表名。
func ListGeoPackageTables(r io.Reader) ([]string, error) {
	db, cleanup, err := openGPKG(r)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	rows, err := db.Query(`SELECT table_name FROM gpkg_contents WHERE data_type='features' ORDER BY table_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err == nil {
			out = append(out, t)
		}
	}
	return out, rows.Err()
}

// ReadGeoPackageTable 读取指定要素表（table 为空时取第一个）。
func ReadGeoPackageTable(r io.Reader, table string) ([]ImportFeature, error) {
	db, cleanup, err := openGPKG(r)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	if table == "" {
		if err := db.QueryRow(
			`SELECT table_name FROM gpkg_contents WHERE data_type='features' LIMIT 1`).Scan(&table); err != nil {
			return nil, fmt.Errorf("GeoPackage 中没有要素表: %w", err)
		}
	}

	var geomCol string
	if err := db.QueryRow(
		`SELECT column_name FROM gpkg_geometry_columns WHERE table_name=? LIMIT 1`, table).Scan(&geomCol); err != nil {
		geomCol = "geom"
	}

	rows, err := db.Query(fmt.Sprintf(`SELECT * FROM "%s"`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	out := make([]ImportFeature, 0)
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}

		props := map[string]any{}
		feat := ImportFeature{Properties: props}
		for i, col := range cols {
			lower := strings.ToLower(col)
			if lower == strings.ToLower(geomCol) {
				if b, ok := toBytes(vals[i]); ok {
					_, wkb, err := parseGPKGBlob(b)
					if err == nil {
						feat.WKB = wkb
					}
				}
				continue
			}
			if lower == "fid" || vals[i] == nil {
				continue
			}
			props[col] = vals[i]
		}
		out = append(out, feat)
	}
	return out, rows.Err()
}

func openGPKG(r io.Reader) (*sql.DB, func(), error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, err
	}
	tmp, err := os.CreateTemp("", "mapserver-*.gpkg")
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tmp.Close() }()
	if _, err := tmp.Write(data); err != nil {
		return nil, nil, err
	}
	db, err := sql.Open("sqlite", tmp.Name())
	if err != nil {
		return nil, nil, fmt.Errorf("打开 GeoPackage 失败: %w", err)
	}
	cleanup := func() {
		_ = db.Close()
		_ = os.Remove(tmp.Name())
	}
	return db, cleanup, nil
}

func toBytes(v any) ([]byte, bool) {
	switch b := v.(type) {
	case []byte:
		return b, true
	case string:
		return []byte(b), true
	default:
		return nil, false
	}
}
