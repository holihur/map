package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

func TestFeatureSearchBatchStatsAndFilteredExport(t *testing.T) {
	srv := newServer(t, testCfg)
	tok := login(t, srv, "admin", testCfg.AdminPassword)
	name := uniqueName("v2")
	createLayer(t, srv, tok, name, "POINT", []map[string]any{
		{"name": "name", "type": "text"},
		{"name": "score", "type": "integer"},
	})
	t.Cleanup(func() {
		doReq(t, http.MethodDelete, srv.URL+"/api/layers/"+name, tok, nil, "")
	})
	payload := map[string]any{
		"type": "FeatureCollection",
		"features": []map[string]any{
			{"type": "Feature", "geometry": map[string]any{"type": "Point", "coordinates": []float64{116.4, 39.9}}, "properties": map[string]any{"name": "Alpha 城", "score": 10}},
			{"type": "Feature", "geometry": map[string]any{"type": "Point", "coordinates": []float64{121.4, 31.2}}, "properties": map[string]any{"name": "Beta 城", "score": 20}},
		},
	}
	readJSON(t, doJSON(t, http.MethodPost, srv.URL+"/api/layers/"+name+"/features", tok, payload), nil)

	var fc struct {
		Total    int64 `json:"total"`
		Features []struct {
			ID int64 `json:"id"`
		} `json:"features"`
	}
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/api/layers/"+name+"/features?q=Alpha", tok, nil, ""), &fc)
	if fc.Total != 1 || len(fc.Features) != 1 {
		t.Fatalf("search total = %d, want 1", fc.Total)
	}
	id := fc.Features[0].ID

	var search struct {
		Features []json.RawMessage `json:"features"`
	}
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/api/search?q=Alpha", tok, nil, ""), &search)
	if len(search.Features) == 0 {
		t.Fatal("search endpoint returned no feature hits")
	}

	wantStatus(t, doJSON(t, http.MethodPost, srv.URL+"/api/layers/reorder", tok, map[string]any{
		"names": []string{name},
	}), http.StatusOK)

	var stats struct {
		Stats []struct {
			Field string `json:"field"`
		} `json:"stats"`
	}
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/api/layers/"+name+"/stats", tok, nil, ""), &stats)
	if len(stats.Stats) != 2 {
		t.Fatalf("stats fields = %d, want 2", len(stats.Stats))
	}

	var batch struct {
		Affected int64 `json:"affected"`
	}
	readJSON(t, doJSON(t, http.MethodPost, srv.URL+"/api/layers/"+name+"/features/batch", tok, map[string]any{
		"action": "update", "ids": []int64{id}, "properties": map[string]any{"score": 99},
	}), &batch)
	if batch.Affected != 1 {
		t.Fatalf("batch affected = %d", batch.Affected)
	}

	resp := doReq(t, http.MethodGet, srv.URL+"/api/layers/"+name+"/export?format=csv&f.score.gte=99", tok, nil, "")
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "Alpha") {
		t.Fatalf("filtered export status=%d body=%s", resp.StatusCode, body)
	}

	analyzeResp := doJSON(t, http.MethodPost, srv.URL+"/api/layers/"+name+"/analyze", tok, map[string]any{
		"op": "buffer", "distance": 0.1, "ids": []int64{id},
	})
	analyzeBody, _ := io.ReadAll(analyzeResp.Body)
	analyzeResp.Body.Close()
	if analyzeResp.StatusCode != http.StatusOK || !strings.Contains(string(analyzeBody), "FeatureCollection") {
		t.Fatalf("analyze status=%d body=%s", analyzeResp.StatusCode, analyzeBody)
	}

	readJSON(t, doJSON(t, http.MethodPost, srv.URL+"/api/layers/"+name+"/features/batch", tok, map[string]any{
		"action": "delete", "ids": []int64{id},
	}), &batch)
	if batch.Affected != 1 {
		t.Fatalf("batch delete affected = %d", batch.Affected)
	}
}

func TestAutoImportPreviewAndCreate(t *testing.T) {
	srv := newServer(t, testCfg)
	tok := login(t, srv, "admin", testCfg.AdminPassword)
	name := uniqueName("auto")

	post := func(preview bool) *http.Response {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		_ = mw.WriteField("name", name)
		_ = mw.WriteField("srid", "4326")
		if preview {
			_ = mw.WriteField("preview", "true")
		}
		fw, _ := mw.CreateFormFile("file", "points.csv")
		_, _ = fw.Write([]byte("name,population,lon,lat\nA,123,116,39\nB,456,121,31\n"))
		_ = mw.Close()
		return doReq(t, http.MethodPost, srv.URL+"/api/layers/import", tok, &buf, mw.FormDataContentType())
	}

	var preview struct {
		Preview      bool   `json:"preview"`
		GeometryType string `json:"geometryType"`
		Count        int    `json:"count"`
		Fields       []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"fields"`
	}
	readJSON(t, post(true), &preview)
	if !preview.Preview || preview.GeometryType != "POINT" || preview.Count != 2 {
		t.Fatalf("preview = %+v", preview)
	}
	foundInt := false
	for _, f := range preview.Fields {
		if f.Name == "population" && f.Type == "integer" {
			foundInt = true
		}
	}
	if !foundInt {
		t.Fatalf("inferred fields = %+v", preview.Fields)
	}

	var created struct {
		Layer struct {
			Name string `json:"name"`
		} `json:"layer"`
		Imported int `json:"imported"`
	}
	readJSON(t, post(false), &created)
	t.Cleanup(func() {
		doReq(t, http.MethodDelete, srv.URL+"/api/layers/"+created.Layer.Name, tok, nil, "")
	})
	if created.Layer.Name != name || created.Imported != 2 {
		t.Fatalf("created = %+v", created)
	}
	var fc struct {
		Total int64 `json:"total"`
	}
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/api/layers/"+name+"/features", tok, nil, ""), &fc)
	if fc.Total != 2 {
		t.Fatalf("imported features = %d", fc.Total)
	}
}

func TestPrivateLayerACLAndAPIKeyAndMaps(t *testing.T) {
	srv := newServer(t, testCfg)
	admin := login(t, srv, "admin", testCfg.AdminPassword)
	name := uniqueName("private")
	createLayer(t, srv, admin, name, "POINT", []map[string]any{{"name": "name", "type": "text"}})
	t.Cleanup(func() {
		doReq(t, http.MethodDelete, srv.URL+"/api/layers/"+name, admin, nil, "")
	})

	// 创建 viewer 用户
	username := uniqueName("viewer")
	resp := doJSON(t, http.MethodPost, srv.URL+"/api/users", admin, map[string]string{
		"username": username, "password": "password123", "role": "viewer",
	})
	var user struct {
		ID int64 `json:"id"`
	}
	readJSON(t, resp, &user)
	t.Cleanup(func() {
		doReq(t, http.MethodDelete, fmt.Sprintf("%s/api/users/%d", srv.URL, user.ID), admin, nil, "")
	})
	viewer := login(t, srv, username, "password123")

	// 设为私有并授权 viewer
	readJSON(t, doJSON(t, http.MethodPut, srv.URL+"/api/layers/"+name+"/access", admin, map[string]any{
		"visibility": "private",
		"access":     []map[string]any{{"userId": user.ID, "username": username, "level": "read"}},
	}), nil)

	wantStatus(t, doReq(t, http.MethodGet, srv.URL+"/api/layers/"+name, "", nil, ""), http.StatusForbidden)
	wantStatus(t, doReq(t, http.MethodGet, srv.URL+"/api/layers/"+name, viewer, nil, ""), http.StatusOK)

	// 能力文档不应向匿名用户泄露私有图层
	capResp := doReq(t, http.MethodGet, srv.URL+"/api/capabilities", "", nil, "")
	capBody, _ := io.ReadAll(capResp.Body)
	capResp.Body.Close()
	if strings.Contains(string(capBody), `"`+name+`"`) {
		t.Fatalf("capabilities leaked private layer %s", name)
	}

	// API Key：admin 创建后可用于匿名请求
	var keyResp struct {
		Plaintext string `json:"plaintext"`
	}
	readJSON(t, doJSON(t, http.MethodPost, srv.URL+"/api/apikeys", admin, map[string]string{
		"name": "test-key", "role": "viewer",
	}), &keyResp)
	if keyResp.Plaintext == "" {
		t.Fatal("empty api key")
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/layers", nil)
	req.Header.Set("X-API-Key", keyResp.Plaintext)
	keyHTTPResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(t, keyHTTPResp, http.StatusOK)

	// 分享地图：创建 + 匿名读取
	state := map[string]any{"version": 1, "basemap": "osm", "center": []float64{116, 39}, "zoom": 5}
	var m struct {
		Token string `json:"token"`
	}
	readJSON(t, doJSON(t, http.MethodPost, srv.URL+"/api/maps", admin, map[string]any{
		"name": "test-share", "state": state, "isPublic": true,
	}), &m)
	if m.Token == "" {
		t.Fatal("empty map token")
	}
	var got struct {
		Name  string          `json:"name"`
		State json.RawMessage `json:"state"`
	}
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/api/maps/"+m.Token, "", nil, ""), &got)
	if got.Name != "test-share" || len(got.State) == 0 {
		t.Fatalf("map = %+v", got)
	}

	// 审计日志应包含刚才的写操作
	var logs struct {
		Logs []struct {
			Method string `json:"method"`
			Path   string `json:"path"`
		} `json:"logs"`
	}
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/api/audit?limit=50", admin, nil, ""), &logs)
	found := false
	for _, l := range logs.Logs {
		if l.Method == http.MethodPost && strings.Contains(l.Path, "/api/maps") {
			found = true
		}
	}
	if !found {
		t.Fatalf("audit logs missing map creation: %+v", logs.Logs)
	}
}
