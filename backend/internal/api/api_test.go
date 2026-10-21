package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mapserver/internal/config"
	"mapserver/internal/db"
	"mapserver/internal/models"
	"mapserver/internal/store"
)

var (
	testPool *pgxpool.Pool
	testSt   *store.Store
	testCfg  config.Config
	testTmp  string
)

func TestMain(m *testing.M) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://map:map@localhost:5432/mapserver_test?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "跳过集成测试：无法连接测试库 (%v)\n", err)
		os.Exit(0)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		fmt.Fprintf(os.Stderr, "迁移测试库失败: %v\n", err)
		os.Exit(1)
	}
	// 重置测试库，保证每次运行环境一致
	resetTestDB(ctx, pool)

	st := store.New(pool)
	if err := seedFixtures(ctx, st); err != nil {
		fmt.Fprintf(os.Stderr, "注入测试夹具失败: %v\n", err)
		os.Exit(1)
	}

	tmp, _ := os.MkdirTemp("", "mapserver-test-cache")
	testCfg = config.Config{
		JWTSecret:        "test-secret",
		TokenTTL:         time.Hour,
		AdminUsername:    "admin",
		AdminPassword:    "test-admin-123",
		MaxFeatures:      10000,
		TileMaxZoom:      22,
		TileCacheEntries: 256,
		TileCacheTTL:     time.Minute,
		MetaCacheTTL:     time.Second,
		TileCacheDir:     tmp,
		TileDiskTTL:      time.Minute,
		TileCacheMaxMB:   8,
		RateLimitRPS:     0,
		RateLimitBurst:   0,
	}
	if _, err := db.EnsureAdmin(ctx, pool, testCfg.AdminUsername, testCfg.AdminPassword); err != nil {
		fmt.Fprintf(os.Stderr, "创建测试管理员失败: %v\n", err)
		os.Exit(1)
	}
	testPool, testSt, testTmp = pool, store.New(pool), tmp
	code := m.Run()
	pool.Close()
	_ = os.RemoveAll(tmp)
	os.Exit(code)
}

// seedFixtures 注入固定的演示图层，使集成测试不依赖种子迁移。
func seedFixtures(ctx context.Context, st *store.Store) error {
	countries, err := st.CreateLayer(ctx, models.CreateLayerRequest{
		Name: "countries", Title: "国家", GeometryType: "MULTIPOLYGON", SRID: 4326,
		Fields: []models.Field{{Name: "name", Type: models.FieldText}, {Name: "iso", Type: models.FieldText}, {Name: "population", Type: models.FieldBigInt}},
	})
	if err != nil {
		return err
	}
	cities, err := st.CreateLayer(ctx, models.CreateLayerRequest{
		Name: "cities", Title: "城市", GeometryType: "POINT", SRID: 4326,
		Fields: []models.Field{{Name: "name", Type: models.FieldText}, {Name: "country", Type: models.FieldText}, {Name: "population", Type: models.FieldBigInt}, {Name: "capital", Type: models.FieldBoolean}},
	})
	if err != nil {
		return err
	}

	countryData := []struct {
		name, iso string
		pop       int64
		wkt       string
	}{
		{"中国", "CN", 1412000000, "MULTIPOLYGON(((70 15,135 15,135 53,70 53,70 15)))"},
		{"巴西", "BR", 216000000, "MULTIPOLYGON(((-73 5,-35 -5,-40 -33,-60 -30,-73 5)))"},
		{"法国", "FR", 68000000, "MULTIPOLYGON(((-4 43,8 43,8 51,-4 51,-4 43)))"},
	}
	for _, c := range countryData {
		if _, err := st.InsertFeatureGeom(ctx, countries, store.GeomValue{WKT: c.wkt, SRID: 4326}, map[string]any{
			"name": c.name, "iso": c.iso, "population": c.pop,
		}); err != nil {
			return err
		}
	}

	cityData := []struct {
		name, country string
		lon, lat      float64
		pop           int64
		capital       bool
	}{
		{"北京", "中国", 116.4074, 39.9042, 21890000, true},
		{"上海", "中国", 121.4737, 31.2304, 24870000, false},
		{"东京", "日本", 139.6917, 35.6895, 13960000, true},
		{"巴黎", "法国", 2.3522, 48.8566, 2148000, true},
		{"伦敦", "英国", -0.1276, 51.5072, 8982000, true},
		{"纽约", "美国", -74.0060, 40.7128, 8336000, false},
		{"悉尼", "澳大利亚", 151.2093, -33.8688, 5312000, false},
		{"圣保罗", "巴西", -46.6333, -23.5505, 12330000, false},
		{"开罗", "埃及", 31.2357, 30.0444, 9540000, true},
		{"莫斯科", "俄罗斯", 37.6173, 55.7558, 12500000, true},
	}
	for _, c := range cityData {
		wkt := fmt.Sprintf("POINT(%f %f)", c.lon, c.lat)
		if _, err := st.InsertFeatureGeom(ctx, cities, store.GeomValue{WKT: wkt, SRID: 4326}, map[string]any{
			"name": c.name, "country": c.country, "population": c.pop, "capital": c.capital,
		}); err != nil {
			return err
		}
	}
	return nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// resetTestDB 删除所有图层物理表与元数据，使集成测试从干净状态开始。
func resetTestDB(ctx context.Context, pool *pgxpool.Pool) {
	rows, err := pool.Query(ctx, `SELECT table_name FROM layers`)
	if err == nil {
		tables := []string{}
		for rows.Next() {
			var t string
			if rows.Scan(&t) == nil {
				tables = append(tables, t)
			}
		}
		rows.Close()
		for _, t := range tables {
			if strings.HasPrefix(t, "layer_") {
				_, _ = pool.Exec(ctx, `DROP TABLE IF EXISTS "`+t+`" CASCADE`)
			}
		}
	}
	_, _ = pool.Exec(ctx, `DELETE FROM layer_access`)
	_, _ = pool.Exec(ctx, `DELETE FROM layers`)
	_, _ = pool.Exec(ctx, `DELETE FROM styles`)
	_, _ = pool.Exec(ctx, `DELETE FROM api_keys`)
	_, _ = pool.Exec(ctx, `DELETE FROM map_states`)
	_, _ = pool.Exec(ctx, `DELETE FROM audit_logs`)
	_, _ = pool.Exec(ctx, `DELETE FROM users`)
}

func newServer(t *testing.T, cfg config.Config) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(NewRouter(testSt, cfg, testLogger()))
	t.Cleanup(srv.Close)
	return srv
}

func doReq(t *testing.T, method, url, token string, body io.Reader, contentType string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp
}

func doJSON(t *testing.T, method, url, token string, payload any) *http.Response {
	t.Helper()
	var body io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		body = bytes.NewReader(b)
	}
	return doReq(t, method, url, token, body, "application/json")
}

func readJSON(t *testing.T, resp *http.Response, out any) {
	t.Helper()
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		t.Fatalf("HTTP %d: %s", resp.StatusCode, string(data))
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("decode json: %v (%s)", err, string(data))
		}
	}
}

func wantStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want %d (%s)", resp.StatusCode, want, string(body))
	}
}

func login(t *testing.T, srv *httptest.Server, user, pass string) string {
	t.Helper()
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/auth/login", "", map[string]string{
		"username": user, "password": pass,
	})
	var out struct {
		Token string `json:"token"`
	}
	readJSON(t, resp, &out)
	if out.Token == "" {
		t.Fatal("empty token")
	}
	return out.Token
}

var layerSeq int64

func uniqueName(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, atomic.AddInt64(&layerSeq, 1))
}

func createLayer(t *testing.T, srv *httptest.Server, token, name, geomType string, fields []map[string]any) {
	t.Helper()
	payload := map[string]any{
		"name": name, "title": name, "geometryType": geomType, "srid": 4326, "fields": fields,
	}
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/layers", token, payload)
	wantStatus(t, resp, http.StatusCreated)
	t.Cleanup(func() {
		doReq(t, http.MethodDelete, srv.URL+"/api/layers/"+name, token, nil, "")
	})
}

// ── 测试 ────────────────────────────────────────────────────────

func TestAuthAndRBAC(t *testing.T) {
	srv := newServer(t, testCfg)

	// 错误密码
	wantStatus(t, doJSON(t, http.MethodPost, srv.URL+"/api/auth/login", "", map[string]string{
		"username": "admin", "password": "wrong",
	}), http.StatusUnauthorized)

	admin := login(t, srv, "admin", testCfg.AdminPassword)

	// me
	resp := doReq(t, http.MethodGet, srv.URL+"/api/auth/me", admin, nil, "")
	var me struct {
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	readJSON(t, resp, &me)
	if me.Username != "admin" || me.Role != "admin" {
		t.Fatalf("me = %+v", me)
	}

	// 未登录写入 → 401
	wantStatus(t, doJSON(t, http.MethodPost, srv.URL+"/api/layers", "", map[string]any{"name": "x"}), http.StatusUnauthorized)

	// 创建 viewer 与 editor
	viewer := uniqueName("viewer")
	editor := uniqueName("editor")
	for _, u := range []struct{ name, role string }{{viewer, "viewer"}, {editor, "editor"}} {
		resp := doJSON(t, http.MethodPost, srv.URL+"/api/users", admin, map[string]string{
			"username": u.name, "password": "password123", "role": u.role,
		})
		wantStatus(t, resp, http.StatusCreated)
	}
	t.Cleanup(func() {
		var users struct {
			Users []struct {
				ID       int64  `json:"id"`
				Username string `json:"username"`
			} `json:"users"`
		}
		readJSON(t, doReq(t, http.MethodGet, srv.URL+"/api/users", admin, nil, ""), &users)
		for _, u := range users.Users {
			if u.Username == viewer || u.Username == editor {
				doReq(t, http.MethodDelete, fmt.Sprintf("%s/api/users/%d", srv.URL, u.ID), admin, nil, "")
			}
		}
	})

	viewerTok := login(t, srv, viewer, "password123")
	editorTok := login(t, srv, editor, "password123")

	// 密码过短 → 400
	wantStatus(t, doJSON(t, http.MethodPost, srv.URL+"/api/users", admin, map[string]string{
		"username": uniqueName("short"), "password": "1234", "role": "viewer",
	}), http.StatusBadRequest)

	// viewer 读 OK，写 403
	wantStatus(t, doReq(t, http.MethodGet, srv.URL+"/api/layers", viewerTok, nil, ""), http.StatusOK)
	name := uniqueName("rbac")
	wantStatus(t, doJSON(t, http.MethodPost, srv.URL+"/api/layers", viewerTok, map[string]any{
		"name": name, "geometryType": "POINT",
	}), http.StatusForbidden)

	// editor 可建图层
	createLayer(t, srv, editorTok, name, "POINT", nil)

	// viewer 不可访问用户管理
	wantStatus(t, doReq(t, http.MethodGet, srv.URL+"/api/users", viewerTok, nil, ""), http.StatusForbidden)
}

func TestTokenRevocation(t *testing.T) {
	srv := newServer(t, testCfg)
	admin := login(t, srv, "admin", testCfg.AdminPassword)

	// 正常访问
	wantStatus(t, doReq(t, http.MethodGet, srv.URL+"/api/auth/me", admin, nil, ""), http.StatusOK)

	// logout-all 使旧 token 失效
	wantStatus(t, doReq(t, http.MethodPost, srv.URL+"/api/auth/logout-all", admin, nil, ""), http.StatusOK)
	wantStatus(t, doReq(t, http.MethodGet, srv.URL+"/api/auth/me", admin, nil, ""), http.StatusUnauthorized)

	// 重新登录后可 refresh
	admin2 := login(t, srv, "admin", testCfg.AdminPassword)
	var ref struct {
		Token string `json:"token"`
	}
	readJSON(t, doReq(t, http.MethodPost, srv.URL+"/api/auth/refresh", admin2, nil, ""), &ref)
	if ref.Token == "" {
		t.Fatal("refresh returned empty token")
	}
	wantStatus(t, doReq(t, http.MethodGet, srv.URL+"/api/auth/me", ref.Token, nil, ""), http.StatusOK)
}

func TestLayerAndFeatureCRUD(t *testing.T) {
	srv := newServer(t, testCfg)
	tok := login(t, srv, "admin", testCfg.AdminPassword)
	name := uniqueName("poi")
	createLayer(t, srv, tok, name, "POINT", []map[string]any{
		{"name": "name", "type": "text"},
		{"name": "rating", "type": "double"},
		{"name": "open", "type": "boolean"},
		{"name": "population", "type": "bigint"},
	})

	// 批量插入
	insertBody := `{"type":"FeatureCollection","features":[
	  {"type":"Feature","geometry":{"type":"Point","coordinates":[116.4,39.9]},"properties":{"name":"A","rating":4.5,"open":true,"population":100}},
	  {"type":"Feature","geometry":{"type":"Point","coordinates":[121.5,31.2]},"properties":{"name":"B","rating":3.2,"open":false,"population":200}}
	]}`
	wantStatus(t, doReq(t, http.MethodPost, srv.URL+"/api/layers/"+name+"/features", tok,
		strings.NewReader(insertBody), "application/json"), http.StatusCreated)

	// 列表
	var fc struct {
		Total    int64 `json:"total"`
		Features []struct {
			ID         any            `json:"id"`
			Properties map[string]any `json:"properties"`
		} `json:"features"`
	}
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/api/layers/"+name+"/features", tok, nil, ""), &fc)
	if fc.Total != 2 || len(fc.Features) != 2 {
		t.Fatalf("total=%d features=%d", fc.Total, len(fc.Features))
	}

	// 过滤 rating>=4
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/api/layers/"+name+"/features?f.rating.gte=4", tok, nil, ""), &fc)
	if fc.Total != 1 || fc.Features[0].Properties["name"] != "A" {
		t.Fatalf("filter gte failed: %+v", fc)
	}

	// bbox
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/api/layers/"+name+"/features?bbox=120,30,122,32", tok, nil, ""), &fc)
	if fc.Total != 1 || fc.Features[0].Properties["name"] != "B" {
		t.Fatalf("bbox failed: %+v", fc)
	}

	// 更新
	fid := int64(fc.Features[0].ID.(float64))
	upd := `{"type":"Feature","geometry":{"type":"Point","coordinates":[121.6,31.3]},"properties":{"name":"B2","rating":3.9,"open":true,"population":222}}`
	wantStatus(t, doReq(t, http.MethodPut, fmt.Sprintf("%s/api/layers/%s/features/%d", srv.URL, name, fid), tok,
		strings.NewReader(upd), "application/json"), http.StatusOK)

	// 删除
	wantStatus(t, doReq(t, http.MethodDelete, fmt.Sprintf("%s/api/layers/%s/features/%d", srv.URL, name, fid), tok, nil, ""), http.StatusNoContent)

	// bounds
	var bounds struct {
		Bounds []float64 `json:"bounds"`
	}
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/api/layers/"+name+"/bounds", tok, nil, ""), &bounds)
	if len(bounds.Bounds) != 4 {
		t.Fatalf("bounds = %v", bounds.Bounds)
	}

	// 不存在的要素 → 404
	wantStatus(t, doReq(t, http.MethodGet, fmt.Sprintf("%s/api/layers/%s/features/999999", srv.URL, name), tok, nil, ""), http.StatusNotFound)
	// 不存在的图层 → 404
	wantStatus(t, doReq(t, http.MethodGet, srv.URL+"/api/layers/does_not_exist_xyz", tok, nil, ""), http.StatusNotFound)
}

func TestCRS(t *testing.T) {
	srv := newServer(t, testCfg)
	tok := login(t, srv, "admin", testCfg.AdminPassword)
	name := uniqueName("proj")
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/layers", tok, map[string]any{
		"name": name, "geometryType": "POINT", "srid": 3857,
		"fields": []map[string]any{{"name": "n", "type": "text"}},
	})
	wantStatus(t, resp, http.StatusCreated)
	t.Cleanup(func() { doReq(t, http.MethodDelete, srv.URL+"/api/layers/"+name, tok, nil, "") })

	// 用 4326 坐标写入，应被投影到 3857
	body := `{"type":"Feature","geometry":{"type":"Point","coordinates":[116.4074,39.9042]},"properties":{"n":"x"}}`
	wantStatus(t, doReq(t, http.MethodPost, srv.URL+"/api/layers/"+name+"/features?inputSrid=4326", tok,
		strings.NewReader(body), "application/json"), http.StatusCreated)

	// 输出 3857 时应为米制大数值
	var fc struct {
		CRS      map[string]any `json:"crs"`
		Features []struct {
			Geometry struct {
				Coordinates []float64 `json:"coordinates"`
			} `json:"geometry"`
		} `json:"features"`
	}
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/api/layers/"+name+"/features?outSrid=3857", tok, nil, ""), &fc)
	coords := fc.Features[0].Geometry.Coordinates
	if len(coords) != 2 || coords[0] < 1000000 {
		t.Fatalf("expected projected coords, got %v", coords)
	}

	// 输出 4326 应回到经纬度
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/api/layers/"+name+"/features?outSrid=4326", tok, nil, ""), &fc)
	coords = fc.Features[0].Geometry.Coordinates
	if coords[0] < 100 || coords[0] > 130 {
		t.Fatalf("expected lon/lat, got %v", coords)
	}

	// bbox 以 4326 声明
	var count struct {
		Total int64 `json:"total"`
	}
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/api/layers/"+name+"/features?bbox=116,39,117,41&bboxSrid=4326", tok, nil, ""), &count)
	if count.Total != 1 {
		t.Fatalf("bbox 4326 total=%d", count.Total)
	}

	// 转换为 4326
	var updated struct {
		SRID int `json:"srid"`
	}
	readJSON(t, doJSON(t, http.MethodPost, srv.URL+"/api/layers/"+name+"/transform", tok, map[string]int{"srid": 4326}), &updated)
	if updated.SRID != 4326 {
		t.Fatalf("transform srid = %d", updated.SRID)
	}

	// 未知 SRID → 400
	wantStatus(t, doJSON(t, http.MethodPost, srv.URL+"/api/layers/"+name+"/transform", tok, map[string]int{"srid": 999999}), http.StatusBadRequest)

	// CRS 查询
	var crs struct {
		CRS []struct {
			SRID int `json:"srid"`
		} `json:"crs"`
	}
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/api/crs?q=WGS", tok, nil, ""), &crs)
	if len(crs.CRS) == 0 {
		t.Fatal("crs search returned nothing")
	}
}

func TestTiles(t *testing.T) {
	srv := newServer(t, testCfg)
	tok := login(t, srv, "admin", testCfg.AdminPassword)

	// 默认 WebMercatorQuad MVT
	resp := doReq(t, http.MethodGet, srv.URL+"/api/tiles/countries/2/3/1.mvt", tok, nil, "")
	wantStatus(t, resp, http.StatusOK)
	resp = doReq(t, http.MethodGet, srv.URL+"/api/tiles/countries/2/3/1.mvt", tok, nil, "")
	if resp.Header.Get("X-Tile-Cache") != "HIT" {
		t.Errorf("second tile request should hit cache, got %q", resp.Header.Get("X-Tile-Cache"))
	}
	resp.Body.Close()

	// CRS84 矩阵集
	wantStatus(t, doReq(t, http.MethodGet, srv.URL+"/api/tiles/countries/WorldCRS84Quad/0/0/0.mvt", tok, nil, ""), http.StatusOK)

	// 栅格 PNG
	resp = doReq(t, http.MethodGet, srv.URL+"/api/tiles/countries/2/3/1.png", tok, nil, "")
	wantStatus(t, resp, http.StatusOK)
	if ct := resp.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("png content-type = %q", ct)
	}
	resp.Body.Close()

	// 越界 → 400
	wantStatus(t, doReq(t, http.MethodGet, srv.URL+"/api/tiles/countries/WorldCRS84Quad/0/99/0.mvt", tok, nil, ""), http.StatusBadRequest)

	// 瓦片矩阵集列表
	var tms struct {
		Sets []struct {
			ID string `json:"id"`
		} `json:"tileMatrixSets"`
	}
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/api/tilematrixsets", tok, nil, ""), &tms)
	if len(tms.Sets) != 2 {
		t.Fatalf("tilematrixsets = %+v", tms.Sets)
	}
}

func TestWMS(t *testing.T) {
	srv := newServer(t, testCfg)
	tok := login(t, srv, "admin", testCfg.AdminPassword)

	// GetCapabilities
	resp := doReq(t, http.MethodGet, srv.URL+"/api/wms?SERVICE=WMS&REQUEST=GetCapabilities", tok, nil, "")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "<WMS_Capabilities") {
		t.Fatalf("wms capabilities failed: %d", resp.StatusCode)
	}

	// GetMap（EPSG:4326，1.3.0 轴序 lat,lon）
	resp = doReq(t, http.MethodGet, srv.URL+"/api/wms?SERVICE=WMS&VERSION=1.3.0&REQUEST=GetMap&LAYERS=countries&CRS=EPSG:4326&BBOX=-90,-180,90,180&WIDTH=200&HEIGHT=100&FORMAT=image/png", tok, nil, "")
	pngBytes, _ := io.ReadAll(resp.Body)
	status := resp.StatusCode
	resp.Body.Close()
	if status != http.StatusOK {
		t.Fatalf("GetMap status=%d body=%s", status, string(pngBytes))
	}
	if len(pngBytes) < 8 || !bytes.Equal(pngBytes[1:4], []byte("PNG")) {
		t.Fatalf("expected PNG magic, got %x", pngBytes[:min(8, len(pngBytes))])
	}

	// GetFeatureInfo
	resp = doReq(t, http.MethodGet, srv.URL+"/api/wms?SERVICE=WMS&VERSION=1.3.0&REQUEST=GetFeatureInfo&QUERY_LAYERS=countries&CRS=EPSG:4326&BBOX=-90,-180,90,180&WIDTH=360&HEIGHT=180&I=296&J=50&INFO_FORMAT=application/json", tok, nil, "")
	var fi struct {
		Total int `json:"total"`
	}
	readJSON(t, resp, &fi)
	if fi.Total == 0 {
		t.Error("GetFeatureInfo returned no features")
	}

	// 图层不存在 → 异常
	resp = doReq(t, http.MethodGet, srv.URL+"/api/wms?SERVICE=WMS&REQUEST=GetMap&LAYERS=nope&CRS=EPSG:4326&BBOX=0,0,1,1&WIDTH=10&HEIGHT=10&FORMAT=image/png", tok, nil, "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown layer, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestWFS(t *testing.T) {
	srv := newServer(t, testCfg)
	tok := login(t, srv, "admin", testCfg.AdminPassword)

	// GetCapabilities
	resp := doReq(t, http.MethodGet, srv.URL+"/api/wfs?SERVICE=WFS&REQUEST=GetCapabilities", tok, nil, "")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "WFS_Capabilities") {
		t.Fatal("wfs capabilities missing")
	}

	// DescribeFeatureType
	resp = doReq(t, http.MethodGet, srv.URL+"/api/wfs?SERVICE=WFS&REQUEST=DescribeFeatureType&TYPENAMES=cities", tok, nil, "")
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "xsd:schema") {
		t.Fatal("describe feature type missing schema")
	}

	// GetFeature GeoJSON
	var fc struct {
		Total    int64 `json:"total"`
		Features []struct {
			Properties map[string]any `json:"properties"`
		} `json:"features"`
	}
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/api/wfs?SERVICE=WFS&VERSION=2.0.0&REQUEST=GetFeature&TYPENAMES=cities&COUNT=5", tok, nil, ""), &fc)
	if fc.Total != 10 || len(fc.Features) != 5 {
		t.Fatalf("wfs getfeature total=%d returned=%d", fc.Total, len(fc.Features))
	}

	// FILTER Like
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/api/wfs?SERVICE=WFS&REQUEST=GetFeature&TYPENAMES=cities&RESULTTYPE=hits", tok, nil, ""), &fc)
	if fc.Total == 0 {
		t.Fatal("hits returned 0")
	}

	// GML 输出
	resp = doReq(t, http.MethodGet, srv.URL+"/api/wfs?SERVICE=WFS&REQUEST=GetFeature&TYPENAMES=cities&COUNT=1&OUTPUTFORMAT=application/gml+xml", tok, nil, "")
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "gml:Point") {
		t.Fatalf("gml output missing geometry: %s", string(body)[:min(200, len(body))])
	}
}

func TestWFSTransactionAtomic(t *testing.T) {
	srv := newServer(t, testCfg)
	tok := login(t, srv, "admin", testCfg.AdminPassword)
	name := uniqueName("wfst")
	createLayer(t, srv, tok, name, "POINT", []map[string]any{{"name": "name", "type": "text"}})

	insert := fmt.Sprintf(`<?xml version="1.0"?><wfs:Transaction xmlns:wfs="http://www.opengis.net/wfs/2.0" xmlns:ms="http://mapserver.local/ns" xmlns:gml="http://www.opengis.net/gml/3.2" version="2.0.0">
	  <wfs:Insert><ms:%s><ms:geom><gml:Point><gml:pos>1 2</gml:pos></gml:Point></ms:geom><ms:name>one</ms:name></ms:%s></wfs:Insert>
	  <wfs:Insert><ms:%s><ms:geom><gml:Point><gml:pos>3 4</gml:pos></gml:Point></ms:geom><ms:name>two</ms:name></ms:%s></wfs:Insert>
	</wfs:Transaction>`, name, name, name, name)
	resp := doReq(t, http.MethodPost, srv.URL+"/api/wfs", tok, strings.NewReader(insert), "application/xml")
	var txResp string
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	txResp = string(b)
	if resp.StatusCode != http.StatusOK || !strings.Contains(txResp, "<wfs:totalInserted>2</wfs:totalInserted>") {
		t.Fatalf("insert transaction failed: %d %s", resp.StatusCode, txResp)
	}

	// 第二个 insert 指向不存在的图层 → 整体回滚（第一条也不应写入）
	bad := fmt.Sprintf(`<?xml version="1.0"?><wfs:Transaction xmlns:wfs="http://www.opengis.net/wfs/2.0" xmlns:ms="http://mapserver.local/ns" xmlns:gml="http://www.opengis.net/gml/3.2" version="2.0.0">
	  <wfs:Insert><ms:%s><ms:geom><gml:Point><gml:pos>5 6</gml:pos></gml:Point></ms:geom><ms:name>three</ms:name></ms:%s></wfs:Insert>
	  <wfs:Insert><ms:no_such_layer_xyz><ms:geom><gml:Point><gml:pos>7 8</gml:pos></gml:Point></ms:geom></ms:no_such_layer_xyz></wfs:Insert>
	</wfs:Transaction>`, name, name)
	resp = doReq(t, http.MethodPost, srv.URL+"/api/wfs", tok, strings.NewReader(bad), "application/xml")
	if resp.StatusCode == http.StatusOK {
		t.Fatal("expected transaction failure")
	}
	resp.Body.Close()

	var fc struct {
		Total int64 `json:"total"`
	}
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/api/layers/"+name+"/features", tok, nil, ""), &fc)
	if fc.Total != 2 {
		t.Fatalf("transaction was not atomic: total=%d (want 2)", fc.Total)
	}

	// 更新 + 删除
	upd := fmt.Sprintf(`<?xml version="1.0"?><wfs:Transaction xmlns:wfs="http://www.opengis.net/wfs/2.0" xmlns:fes="http://www.opengis.net/fes/2.0" version="2.0.0">
	  <wfs:Update typeName="%s"><wfs:Property><wfs:ValueReference>name</wfs:ValueReference><wfs:Value>one!</wfs:Value></wfs:Property>
	  <fes:Filter><fes:PropertyIsEqualTo><fes:PropertyName>name</fes:PropertyName><fes:Literal>one</fes:Literal></fes:PropertyIsEqualTo></fes:Filter></wfs:Update>
	  <wfs:Delete typeName="%s"><fes:Filter><fes:PropertyIsEqualTo><fes:PropertyName>name</fes:PropertyName><fes:Literal>two</fes:Literal></fes:PropertyIsEqualTo></fes:Filter></wfs:Delete>
	</wfs:Transaction>`, name, name)
	resp = doReq(t, http.MethodPost, srv.URL+"/api/wfs", tok, strings.NewReader(upd), "application/xml")
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), "<wfs:totalUpdated>1</wfs:totalUpdated>") || !strings.Contains(string(b), "<wfs:totalDeleted>1</wfs:totalDeleted>") {
		t.Fatalf("update/delete transaction failed: %s", string(b))
	}

	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/api/layers/"+name+"/features", tok, nil, ""), &fc)
	if fc.Total != 1 {
		t.Fatalf("after update+delete total=%d (want 1)", fc.Total)
	}
}

func TestStylesAndSLD(t *testing.T) {
	srv := newServer(t, testCfg)
	tok := login(t, srv, "admin", testCfg.AdminPassword)

	// 创建 MapLibre 样式
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/styles", tok, map[string]any{
		"name": uniqueName("style"), "isDefault": false, "style": map[string]any{"version": 8},
	})
	wantStatus(t, resp, http.StatusCreated)

	// 上传 SLD
	sldName := uniqueName("sld")
	sld := `<StyledLayerDescriptor><NamedLayer><Name>countries</Name><UserStyle><FeatureTypeStyle>
	  <Rule><Name>green</Name><PolygonSymbolizer><Fill><CssParameter name="fill">#00aa00</CssParameter></Fill></PolygonSymbolizer></Rule>
	</FeatureTypeStyle></UserStyle></NamedLayer></StyledLayerDescriptor>`
	wantStatus(t, doReq(t, http.MethodPost, srv.URL+"/api/styles/sld?name="+sldName, tok, strings.NewReader(sld), "application/xml"), http.StatusCreated)
	t.Cleanup(func() {
		var list struct {
			Styles []struct {
				ID   int64  `json:"id"`
				Name string `json:"name"`
			} `json:"styles"`
		}
		readJSON(t, doReq(t, http.MethodGet, srv.URL+"/api/styles", tok, nil, ""), &list)
		for _, s := range list.Styles {
			if strings.HasPrefix(s.Name, "sld_") || strings.HasPrefix(s.Name, "style_") {
				doReq(t, http.MethodDelete, fmt.Sprintf("%s/api/styles/%d", srv.URL, s.ID), tok, nil, "")
			}
		}
	})

	// 非法 SLD → 400
	wantStatus(t, doReq(t, http.MethodPost, srv.URL+"/api/styles/sld?name=bad_"+sldName, tok, strings.NewReader("<foo/>"), "application/xml"), http.StatusBadRequest)

	// 按名获取
	var got struct {
		SLD string `json:"sld"`
	}
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/api/styles/"+sldName, tok, nil, ""), &got)
	if !strings.Contains(got.SLD, "PolygonSymbolizer") {
		t.Fatal("stored SLD missing")
	}

	// WMS 使用 STYLES=名称
	resp = doReq(t, http.MethodGet, srv.URL+"/api/wms?SERVICE=WMS&VERSION=1.3.0&REQUEST=GetMap&LAYERS=countries&CRS=EPSG:4326&BBOX=-90,-180,90,180&WIDTH=100&HEIGHT=50&FORMAT=image/png&STYLES="+sldName, tok, nil, "")
	wantStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestImportExportRoundTrip(t *testing.T) {
	srv := newServer(t, testCfg)
	tok := login(t, srv, "admin", testCfg.AdminPassword)

	for _, formatName := range []string{"geojson", "csv", "shapefile", "gpkg"} {
		t.Run(formatName, func(t *testing.T) {
			// 导出 cities
			resp := doReq(t, http.MethodGet, srv.URL+"/api/layers/cities/export?format="+formatName, tok, nil, "")
			data, _ := io.ReadAll(resp.Body)
			status := resp.StatusCode
			resp.Body.Close()
			if status != http.StatusOK {
				t.Fatalf("export status=%d body=%s", status, string(data))
			}
			if len(data) == 0 {
				t.Fatal("empty export")
			}

			// 导入到新图层
			name := uniqueName("imp")
			createLayer(t, srv, tok, name, "POINT", []map[string]any{
				{"name": "name", "type": "text"},
				{"name": "country", "type": "text"},
				{"name": "population", "type": "bigint"},
				{"name": "capital", "type": "boolean"},
			})

			var buf bytes.Buffer
			mw := multipart.NewWriter(&buf)
			fw, _ := mw.CreateFormFile("file", "data."+extFor(formatName))
			_, _ = fw.Write(data)
			_ = mw.WriteField("srid", "4326")
			_ = mw.Close()

			resp = doReq(t, http.MethodPost, srv.URL+"/api/layers/"+name+"/import", tok, &buf, mw.FormDataContentType())
			var res struct {
				Imported int `json:"imported"`
			}
			readJSON(t, resp, &res)
			if res.Imported != 10 {
				t.Fatalf("imported %d, want 10", res.Imported)
			}

			var fc struct {
				Total    int64 `json:"total"`
				Features []struct {
					Properties map[string]any `json:"properties"`
				} `json:"features"`
			}
			readJSON(t, doReq(t, http.MethodGet, srv.URL+"/api/layers/"+name+"/features", tok, nil, ""), &fc)
			if fc.Total != 10 {
				t.Fatalf("after import total=%d", fc.Total)
			}
			// 属性完整性
			if fc.Features[0].Properties["name"] == nil || fc.Features[0].Properties["population"] == nil {
				t.Fatalf("properties lost: %+v", fc.Features[0].Properties)
			}
		})
	}
}

func extFor(f string) string {
	switch f {
	case "shapefile":
		return "zip"
	case "gpkg":
		return "gpkg"
	case "csv":
		return "csv"
	default:
		return "geojson"
	}
}

func TestMetricsAndStats(t *testing.T) {
	srv := newServer(t, testCfg)
	tok := login(t, srv, "admin", testCfg.AdminPassword)

	_ = doReq(t, http.MethodGet, srv.URL+"/api/layers", tok, nil, "")

	resp := doReq(t, http.MethodGet, srv.URL+"/metrics", "", nil, "")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "mapserver_http_requests_total") {
		t.Fatal("metrics missing request counter")
	}
	if !strings.Contains(string(body), "mapserver_disk_cache_bytes") {
		t.Fatal("metrics missing disk cache gauge")
	}

	var stats struct {
		Disk struct {
			Enabled    bool  `json:"enabled"`
			LimitBytes int64 `json:"limitBytes"`
		} `json:"diskCache"`
	}
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/api/stats", tok, nil, ""), &stats)
	if !stats.Disk.Enabled || stats.Disk.LimitBytes == 0 {
		t.Fatalf("stats diskCache = %+v", stats.Disk)
	}
}

func TestRateLimit(t *testing.T) {
	cfg := testCfg
	cfg.RateLimitRPS = 1
	cfg.RateLimitBurst = 2
	srv := newServer(t, cfg)

	got429 := false
	for i := 0; i < 10; i++ {
		resp := doReq(t, http.MethodGet, srv.URL+"/api/layers", "", nil, "")
		if resp.StatusCode == http.StatusTooManyRequests {
			got429 = true
			resp.Body.Close()
			break
		}
		resp.Body.Close()
	}
	if !got429 {
		t.Fatal("expected 429 after exceeding rate limit")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
