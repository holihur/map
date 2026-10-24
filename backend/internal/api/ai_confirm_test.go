package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestAIWriteConfirmation(t *testing.T) {
	srv := newServer(t, testCfg)
	admin := login(t, srv, "admin", testCfg.AdminPassword)
	name := uniqueName("aiwrite")
	createLayer(t, srv, admin, name, "POINT", []map[string]any{{"name": "name", "type": "text"}})
	t.Cleanup(func() {
		doReq(t, http.MethodDelete, srv.URL+"/api/layers/"+name, admin, nil, "")
	})

	var created struct {
		IDs []int64 `json:"ids"`
	}
	readJSON(t, doJSON(t, http.MethodPost, srv.URL+"/api/layers/"+name+"/features", admin, map[string]any{
		"type": "Feature", "geometry": map[string]any{"type": "Point", "coordinates": []float64{116, 39}},
		"properties": map[string]any{"name": "待删除"},
	}), &created)
	if len(created.IDs) != 1 {
		t.Fatalf("created ids = %+v", created.IDs)
	}
	fid := created.IDs[0]

	fake := newFakeLLM(t, func(call int, body string) string {
		if call == 1 {
			args := fmt.Sprintf(`{"layer":%q,"ids":[%d]}`, name, fid)
			return fmt.Sprintf(`{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_del","type":"function","function":{"name":"delete_features","arguments":%q}}]}}]}`, args)
		}
		if !strings.Contains(body, `"role":"tool"`) || !strings.Contains(body, "pending_confirmation") {
			t.Error("second LLM call missing pending tool result")
		}
		return `{"choices":[{"message":{"role":"assistant","content":"已生成删除操作，请确认。"}}]}`
	})
	defer fake.Close()

	payload := []byte(`{"messages":[{"role":"user","content":"删除这个要素"}],"allowWrite":true,"confirmWrites":true}`)
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
		Message       string `json:"message"`
		PendingWrites []struct {
			Token string `json:"token"`
			Tool  string `json:"tool"`
		} `json:"pendingWrites"`
	}
	readJSON(t, resp, &out)
	if len(out.PendingWrites) != 1 || out.PendingWrites[0].Token == "" {
		t.Fatalf("pending = %+v", out.PendingWrites)
	}

	// 尚未确认：要素仍然存在
	var fc struct {
		Total int64 `json:"total"`
	}
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/api/layers/"+name+"/features", admin, nil, ""), &fc)
	if fc.Total != 1 {
		t.Fatalf("feature deleted before confirmation, total=%d", fc.Total)
	}

	// 确认执行
	confirmResp := doJSON(t, http.MethodPost, srv.URL+"/api/ai/confirm", admin, map[string]any{
		"token": out.PendingWrites[0].Token,
	})
	var confirmed struct {
		Message string `json:"message"`
	}
	readJSON(t, confirmResp, &confirmed)
	if !strings.Contains(confirmed.Message, "affected") {
		t.Fatalf("confirm message = %q", confirmed.Message)
	}
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/api/layers/"+name+"/features", admin, nil, ""), &fc)
	if fc.Total != 0 {
		t.Fatalf("feature not deleted after confirmation, total=%d", fc.Total)
	}

	// 令牌不可跨用户使用
	var user struct {
		ID int64 `json:"id"`
	}
	uname := uniqueName("aiviewer")
	readJSON(t, doJSON(t, http.MethodPost, srv.URL+"/api/users", admin, map[string]string{
		"username": uname, "password": "password123", "role": "viewer",
	}), &user)
	t.Cleanup(func() {
		doReq(t, http.MethodDelete, fmt.Sprintf("%s/api/users/%d", srv.URL, user.ID), admin, nil, "")
	})
	viewer := login(t, srv, uname, "password123")
	resp = doJSON(t, http.MethodPost, srv.URL+"/api/ai/confirm", viewer, map[string]any{
		"token": out.PendingWrites[0].Token,
	})
	// 令牌已使用/过期或不属于该用户，均应失败
	if resp.StatusCode < 400 {
		body, _ := readAll(resp)
		t.Fatalf("token should not be usable by another user: %d %s", resp.StatusCode, body)
	}
	resp.Body.Close()

	_ = json.Valid
}
