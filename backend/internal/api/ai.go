package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mapserver/internal/auth"
)

// ── BYOK 自然语言地图助手 ─────────────────────────────────────
// 客户端在请求头中携带自己的 OpenAI 兼容 API Key，服务端只做代理与工具调用，
// 不持久化、不记录密钥。

type aiChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type aiMapContext struct {
	VisibleLayers []string    `json:"visibleLayers,omitempty"`
	SelectedLayer string      `json:"selectedLayer,omitempty"`
	Bounds        *[4]float64 `json:"bounds,omitempty"`
}

type aiChatRequest struct {
	Messages   []aiChatMessage `json:"messages"`
	Context    *aiMapContext   `json:"context,omitempty"`
	AllowWrite bool            `json:"allowWrite"`
}

type aiStep struct {
	Tool    string          `json:"tool"`
	Summary string          `json:"summary"`
	Args    json.RawMessage `json:"args,omitempty"`
}

type aiAction struct {
	Type       string          `json:"type"`
	Layer      string          `json:"layer,omitempty"`
	Bounds     *[4]float64     `json:"bounds,omitempty"`
	FeatureIDs []int64         `json:"featureIds,omitempty"`
	Bbox       *[4]float64     `json:"bbox,omitempty"`
	Query      string          `json:"query,omitempty"`
	Visible    *bool           `json:"visible,omitempty"`
	Style      json.RawMessage `json:"style,omitempty"`
	Message    string          `json:"message,omitempty"`
}

type aiResult struct {
	Layer      string          `json:"layer"`
	LayerTitle string          `json:"layerTitle"`
	Feature    json.RawMessage `json:"feature"`
	Total      int64           `json:"total"`
}

type aiChatResponse struct {
	Message string          `json:"message"`
	Steps   []aiStep        `json:"steps,omitempty"`
	Actions []aiAction      `json:"actions,omitempty"`
	Results []aiResult      `json:"results,omitempty"`
	Usage   json.RawMessage `json:"usage,omitempty"`
}

// ── OpenAI 兼容协议 ───────────────────────────────────────────

type llmFunctionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type llmTool struct {
	Type     string         `json:"type"`
	Function llmFunctionDef `json:"function"`
}

type llmToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type llmMessage struct {
	Role       string        `json:"role"`
	Content    *string       `json:"content"`
	ToolCalls  []llmToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	Name       string        `json:"name,omitempty"`
}

type llmCompletion struct {
	Choices []struct {
		Message      llmMessage `json:"message"`
		FinishReason string     `json:"finish_reason"`
	} `json:"choices"`
	Usage json.RawMessage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

type aiToolOutcome struct {
	Result  string
	Step    *aiStep
	Actions []aiAction
	Results []aiResult
}

func stringPtr(s string) *string { return &s }

// aiChat 处理自然语言请求：调用用户提供的 LLM，执行工具，返回回答与地图动作。
func (h *Handlers) aiChat(w http.ResponseWriter, r *http.Request) {
	user, ok := auth.UserFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, errString("需要登录"))
		return
	}
	apiKey := strings.TrimSpace(r.Header.Get("X-AI-API-Key"))
	if apiKey == "" {
		writeError(w, http.StatusBadRequest, errString("缺少 X-AI-API-Key（BYOK：请在前端设置中填写你自己的模型 API Key）"))
		return
	}
	baseURL := strings.TrimSpace(r.Header.Get("X-AI-Base-URL"))
	if baseURL == "" {
		baseURL = h.cfg.AIBaseURL
	}
	model := strings.TrimSpace(r.Header.Get("X-AI-Model"))
	if model == "" {
		model = h.cfg.AIModel
	}
	if !validAIBaseURL(baseURL) {
		writeError(w, http.StatusBadRequest, errString("X-AI-Base-URL 必须是 http/https 地址"))
		return
	}

	var req aiChatRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(req.Messages) == 0 {
		writeError(w, http.StatusBadRequest, errString("messages 不能为空"))
		return
	}

	contextJSON := "{}"
	if req.Context != nil {
		if b, err := json.Marshal(req.Context); err == nil {
			contextJSON = string(b)
		}
	}
	messages := make([]llmMessage, 0, len(req.Messages)+2)
	system := fmt.Sprintf(`你是 MapServer 的地图助手，运行在 Go 后端，可以通过工具读取图层、查询要素、统计、做空间分析，并通过 map_action 控制前端地图。

规则：
1. 先调用 list_layers 了解可用图层、字段与要素数量，禁止编造不存在的图层或字段。
2. 用户要求“定位/缩放到/查看某图层或要素”时，调用 map_action（fly_to_layer / fly_to_bounds / highlight_features / open_data）。
3. 用户要求修改或删除数据时，调用 update_features / delete_features；如果工具返回无权限，如实告知。不要声称已经修改。
4. 坐标默认 EPSG:4326，bbox 顺序为 minx,miny,maxx,maxy。
5. 用中文简洁回答，给出关键数量与少量示例；不要把大段 GeoJSON 直接输出给用户。
6. 如果用户的问题与地图无关，简短说明你能做什么。

当前地图上下文（JSON）：%s`, contextJSON)
	messages = append(messages, llmMessage{Role: "system", Content: stringPtr(system)})
	start := 0
	if len(req.Messages) > 20 {
		start = len(req.Messages) - 20
	}
	for _, m := range req.Messages[start:] {
		role := strings.ToLower(strings.TrimSpace(m.Role))
		if role != "user" && role != "assistant" {
			role = "user"
		}
		messages = append(messages, llmMessage{Role: role, Content: stringPtr(m.Content)})
	}

	client := &http.Client{Timeout: h.cfg.AITimeout}
	if client.Timeout <= 0 {
		client.Timeout = 60 * time.Second
	}

	out := aiChatResponse{}
	var usage json.RawMessage
	for iteration := 0; iteration < 6; iteration++ {
		completion, err := callLLM(r.Context(), client, baseURL, apiKey, model, messages)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		if completion.Error != nil {
			writeError(w, http.StatusBadGateway, fmt.Errorf("模型返回错误: %s", completion.Error.Message))
			return
		}
		if len(completion.Choices) == 0 {
			writeError(w, http.StatusBadGateway, errString("模型没有返回任何结果"))
			return
		}
		usage = completion.Usage
		msg := completion.Choices[0].Message
		if len(msg.ToolCalls) == 0 {
			if msg.Content != nil {
				out.Message = *msg.Content
			}
			if strings.TrimSpace(out.Message) == "" {
				out.Message = "已完成。"
			}
			out.Usage = usage
			w.Header().Set("Cache-Control", "no-store")
			writeJSON(w, http.StatusOK, out)
			return
		}

		// 保留 assistant 的 tool_calls，再逐条追加 tool 结果。
		msg.Role = "assistant"
		messages = append(messages, msg)
		for i, tc := range msg.ToolCalls {
			if tc.ID == "" {
				tc.ID = fmt.Sprintf("call_%d_%d", iteration, i)
			}
			outcome := h.executeAITool(r.Context(), user, tc.Function.Name, tc.Function.Arguments, req.AllowWrite)
			if outcome.Step != nil {
				out.Steps = append(out.Steps, *outcome.Step)
			}
			out.Actions = append(out.Actions, outcome.Actions...)
			out.Results = append(out.Results, outcome.Results...)
			messages = append(messages, llmMessage{
				Role:       "tool",
				ToolCallID: tc.ID,
				Name:       tc.Function.Name,
				Content:    stringPtr(outcome.Result),
			})
		}
	}

	out.Message = "工具调用轮次已达上限，请缩小问题范围后重试。"
	out.Usage = usage
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}

func callLLM(ctx context.Context, client *http.Client, baseURL, apiKey, model string, messages []llmMessage) (*llmCompletion, error) {
	endpoint := strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}
	payload := map[string]any{
		"model":       model,
		"messages":    messages,
		"tools":       aiToolDefs(),
		"tool_choice": "auto",
		"temperature": 0.1,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求模型失败: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 400 {
		var detail struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(data, &detail) == nil && detail.Error.Message != "" {
			return nil, fmt.Errorf("模型请求失败 (%d): %s", resp.StatusCode, detail.Error.Message)
		}
		return nil, fmt.Errorf("模型请求失败 (%d): %s", resp.StatusCode, truncate(string(data), 300))
	}
	var completion llmCompletion
	if err := json.Unmarshal(data, &completion); err != nil {
		return nil, fmt.Errorf("解析模型响应失败: %w", err)
	}
	return &completion, nil
}

func validAIBaseURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
