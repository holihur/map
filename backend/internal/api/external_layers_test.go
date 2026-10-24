package api

import (
	"net/http"
	"strings"
	"testing"
)

func TestExternalLayerCRUDAndCapabilities(t *testing.T) {
	srv := newServer(t, testCfg)
	admin := login(t, srv, "admin", testCfg.AdminPassword)
	name := uniqueName("ext")

	var created struct {
		ID         int64  `json:"id"`
		Name       string `json:"name"`
		SourceType string `json:"sourceType"`
		Visibility string `json:"visibility"`
	}
	readJSON(t, doJSON(t, http.MethodPost, srv.URL+"/api/external-layers", admin, map[string]any{
		"name":       name,
		"title":      "高德地图",
		"sourceType": "xyz",
		"sourceUrl":  "https://webrd01.is.autonavi.com/appmaptile?lang=zh_cn&size=1&scale=1&style=8&x={x}&y={y}&z={z}",
		"style":      map[string]any{"type": "raster", "paint": map[string]any{"raster-opacity": 1}},
		"metadata":   map[string]any{"provider": "amap"},
	}), &created)
	t.Cleanup(func() {
		doReq(t, http.MethodDelete, srv.URL+"/api/external-layers/"+name, admin, nil, "")
	})
	if created.Name != name || created.SourceType != "xyz" || created.Visibility != "public" {
		t.Fatalf("created = %+v", created)
	}

	var list struct {
		Layers []struct {
			Name string `json:"name"`
		} `json:"layers"`
	}
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/api/external-layers", "", nil, ""), &list)
	found := false
	for _, l := range list.Layers {
		if l.Name == name {
			found = true
		}
	}
	if !found {
		t.Fatalf("external layer %s not in list", name)
	}

	// 能力文档应包含第三方图层
	capResp := doReq(t, http.MethodGet, srv.URL+"/api/capabilities", "", nil, "")
	capBody, _ := readAll(capResp)
	if !strings.Contains(capBody, name) {
		t.Fatalf("capabilities missing external layer: %s", capBody)
	}

	// 更新为私有
	readJSON(t, doJSON(t, http.MethodPatch, srv.URL+"/api/external-layers/"+name, admin, map[string]any{
		"visibility": "private",
	}), nil)
	wantStatus(t, doReq(t, http.MethodGet, srv.URL+"/api/external-layers/"+name, "", nil, ""), http.StatusForbidden)
	wantStatus(t, doReq(t, http.MethodGet, srv.URL+"/api/external-layers/"+name, admin, nil, ""), http.StatusOK)
}

func TestAIChatWithFakeLLM(t *testing.T) {
	srv := newServer(t, testCfg)
	admin := login(t, srv, "admin", testCfg.AdminPassword)

	calls := 0
	fake := newFakeLLM(t, func(call int, body string) string {
		calls = call
		if call == 1 {
			return `{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"list_layers","arguments":"{}"}}]}}]}`
		}
		if !strings.Contains(body, `"role":"tool"`) {
			t.Error("second LLM call missing tool result message")
		}
		return `{"choices":[{"message":{"role":"assistant","content":"当前有可用图层，已为你读取。"}}]}`
	})
	defer fake.Close()

	payload := []byte(`{"messages":[{"role":"user","content":"有哪些图层？"}],"allowWrite":false}`)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/ai/chat", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+admin)
	req.Header.Set("X-AI-API-Key", "test-key")
	req.Header.Set("X-AI-Base-URL", fake.URL)
	req.Header.Set("X-AI-Model", "fake-model")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Message string `json:"message"`
		Steps   []struct {
			Tool string `json:"tool"`
		} `json:"steps"`
	}
	readJSON(t, resp, &out)
	if calls < 2 {
		t.Fatalf("LLM calls = %d, want >= 2", calls)
	}
	if !strings.Contains(out.Message, "图层") {
		t.Fatalf("message = %q", out.Message)
	}
	found := false
	for _, s := range out.Steps {
		if s.Tool == "list_layers" {
			found = true
		}
	}
	if !found {
		t.Fatalf("steps = %+v", out.Steps)
	}

	// 缺少 BYOK Key → 400
	wantStatus(t, doJSON(t, http.MethodPost, srv.URL+"/api/ai/chat", admin, map[string]any{
		"messages": []map[string]string{{"role": "user", "content": "hi"}},
	}), http.StatusBadRequest)
}

func TestAIChatDefaultsToDeepSeek(t *testing.T) {
	if testCfg.AIBaseURL != "" && testCfg.AIBaseURL != "https://api.deepseek.com/v1" {
		t.Fatalf("test config AIBaseURL = %q", testCfg.AIBaseURL)
	}
	if testCfg.AIModel != "" && testCfg.AIModel != "deepseek-chat" {
		t.Fatalf("test config AIModel = %q", testCfg.AIModel)
	}
}

func TestExternalTileProxy(t *testing.T) {
	srv := newServer(t, testCfg)
	admin := login(t, srv, "admin", testCfg.AdminPassword)

	upstreamCalls := 0
	upstream2 := newTileUpstream(t, &upstreamCalls)
	defer upstream2.Close()

	name := uniqueName("extproxy")
	readJSON(t, doJSON(t, http.MethodPost, srv.URL+"/api/external-layers", admin, map[string]any{
		"name":       name,
		"title":      "proxy test",
		"sourceType": "xyz",
		"sourceUrl":  upstream2.URL + "/{z}/{x}/{y}.png?token=abc",
		"sourceOptions": map[string]any{
			"proxy": true,
		},
	}), nil)
	t.Cleanup(func() {
		doReq(t, http.MethodDelete, srv.URL+"/api/external-layers/"+name, admin, nil, "")
	})

	resp := doReq(t, http.MethodGet, srv.URL+"/api/external-layers/"+name+"/tile/1/0/0", "", nil, "")
	body, _ := readAll(resp)
	if resp.StatusCode != http.StatusOK || body != "TILE" {
		t.Fatalf("proxy status=%d body=%q", resp.StatusCode, body)
	}
	if get := resp.Header.Get("X-Tile-Cache"); get != "MISS" {
		t.Fatalf("cache header = %q, want MISS", get)
	}
	if upstreamCalls != 1 {
		t.Fatalf("upstream calls = %d", upstreamCalls)
	}
	resp = doReq(t, http.MethodGet, srv.URL+"/api/external-layers/"+name+"/tile/1/0/0", "", nil, "")
	resp.Body.Close()
	if got := resp.Header.Get("X-Tile-Cache"); got != "HIT" {
		t.Fatalf("second cache header = %q, want HIT", got)
	}
}
