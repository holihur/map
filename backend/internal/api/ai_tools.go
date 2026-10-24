package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"mapserver/internal/auth"
	"mapserver/internal/models"
	"mapserver/internal/store"
)

type aiFilterArg struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value string `json:"value"`
}

type aiQueryArgs struct {
	Layer      string         `json:"layer"`
	Q          string         `json:"q"`
	Filters    []aiFilterArg  `json:"filters"`
	Bbox       []float64      `json:"bbox"`
	Bounds     []float64      `json:"bounds"`
	IDs        []int64        `json:"ids"`
	Limit      int            `json:"limit"`
	OrderBy    string         `json:"orderBy"`
	Order      string         `json:"order"`
	Op         string         `json:"op"`
	Action     string         `json:"action"`
	Distance   float64        `json:"distance"`
	Tolerance  float64        `json:"tolerance"`
	Properties map[string]any `json:"properties"`
}

func schemaObject(props map[string]any, required ...string) map[string]any {
	out := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

func aiToolDefs() []llmTool {
	layerProp := map[string]any{"type": "string", "description": "图层名称（来自 list_layers）"}
	filtersProp := map[string]any{
		"type":        "array",
		"description": "属性过滤条件，可组合",
		"items": schemaObject(map[string]any{
			"field": map[string]any{"type": "string"},
			"op": map[string]any{
				"type": "string",
				"enum": []string{"eq", "neq", "gt", "gte", "lt", "lte", "like", "isnull", "notnull", "in"},
			},
			"value": map[string]any{"type": "string"},
		}, "field", "op", "value"),
	}
	bboxProp := map[string]any{
		"type":        "array",
		"items":       map[string]any{"type": "number"},
		"minItems":    4,
		"maxItems":    4,
		"description": "bbox [minx,miny,maxx,maxy]，EPSG:4326",
	}
	queryProps := map[string]any{
		"layer":   layerProp,
		"q":       map[string]any{"type": "string", "description": "在所有 text 字段中模糊搜索"},
		"filters": filtersProp,
		"bbox":    bboxProp,
		"limit":   map[string]any{"type": "integer", "minimum": 1, "maximum": 50},
		"orderBy": map[string]any{"type": "string"},
		"order":   map[string]any{"type": "string", "enum": []string{"asc", "desc"}},
	}
	writeQueryProps := map[string]any{
		"layer":   layerProp,
		"ids":     map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
		"filters": filtersProp,
		"bbox":    bboxProp,
	}

	return []llmTool{
		{
			Type: "function",
			Function: llmFunctionDef{
				Name:        "list_layers",
				Description: "列出当前用户可读取的图层、几何类型、字段、要素数量和范围。开始任何地图任务前应先调用。",
				Parameters:  schemaObject(map[string]any{}),
			},
		},
		{
			Type: "function",
			Function: llmFunctionDef{
				Name:        "query_features",
				Description: "查询图层要素，支持全文搜索、属性过滤、bbox 和排序。返回总数与少量要素属性。",
				Parameters:  schemaObject(queryProps, "layer"),
			},
		},
		{
			Type: "function",
			Function: llmFunctionDef{
				Name:        "layer_stats",
				Description: "统计图层字段：数量、空值、distinct、min/max、top values。",
				Parameters:  schemaObject(map[string]any{"layer": layerProp}, "layer"),
			},
		},
		{
			Type: "function",
			Function: llmFunctionDef{
				Name:        "search",
				Description: "在图层名称/标题和文本属性中全局搜索。",
				Parameters: schemaObject(map[string]any{
					"q":     map[string]any{"type": "string"},
					"layer": map[string]any{"type": "string", "description": "可选，限定图层"},
					"limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 20},
				}, "q"),
			},
		},
		{
			Type: "function",
			Function: llmFunctionDef{
				Name:        "analyze_features",
				Description: "对查询结果做空间分析（buffer/centroid/convexhull/envelope/simplify），返回结果数量与示例。",
				Parameters: schemaObject(map[string]any{
					"layer":     layerProp,
					"op":        map[string]any{"type": "string", "enum": []string{"buffer", "centroid", "convexhull", "envelope", "simplify"}},
					"distance":  map[string]any{"type": "number", "description": "buffer 距离，图层坐标系单位"},
					"tolerance": map[string]any{"type": "number", "description": "simplify 容差，图层坐标系单位"},
					"filters":   filtersProp,
					"bbox":      bboxProp,
				}, "layer", "op"),
			},
		},
		{
			Type: "function",
			Function: llmFunctionDef{
				Name:        "map_action",
				Description: "控制前端地图：定位到图层、飞到 bbox、高亮要素、打开数据表、设置图层可见性。",
				Parameters: schemaObject(map[string]any{
					"action": map[string]any{
						"type": "string",
						"enum": []string{"fly_to_layer", "fly_to_bounds", "highlight_features", "open_data", "set_layer_visibility"},
					},
					"layer":      layerProp,
					"bounds":     bboxProp,
					"bbox":       bboxProp,
					"featureIds": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
					"query":      map[string]any{"type": "string"},
					"visible":    map[string]any{"type": "boolean"},
				}, "action"),
			},
		},
		{
			Type: "function",
			Function: llmFunctionDef{
				Name:        "update_features",
				Description: "批量更新要素属性。仅在 allowWrite=true 且用户有写权限时生效。必须提供 ids 或 filters/bbox。",
				Parameters: schemaObject(map[string]any{
					"layer":      layerProp,
					"ids":        map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
					"filters":    filtersProp,
					"bbox":       bboxProp,
					"properties": map[string]any{"type": "object", "additionalProperties": true},
				}, "layer", "properties"),
			},
		},
		{
			Type: "function",
			Function: llmFunctionDef{
				Name:        "delete_features",
				Description: "批量删除要素。仅在 allowWrite=true 且用户有写权限时生效。必须提供 ids 或 filters/bbox。",
				Parameters:  schemaObject(writeQueryProps, "layer"),
			},
		},
	}
}

func (h *Handlers) executeAITool(ctx context.Context, user *models.User, name, rawArgs string, allowWrite bool) aiToolOutcome {
	var args aiQueryArgs
	if strings.TrimSpace(rawArgs) != "" {
		if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
			return toolError("工具参数不是合法 JSON: " + err.Error())
		}
	}
	stepArgs := json.RawMessage(rawArgs)
	if len(stepArgs) == 0 || !json.Valid(stepArgs) {
		stepArgs = json.RawMessage(`{}`)
	}

	switch name {
	case "list_layers":
		return h.aiListLayers(ctx)
	case "query_features":
		return h.aiQueryFeatures(ctx, args, stepArgs)
	case "layer_stats":
		return h.aiLayerStats(ctx, args, stepArgs)
	case "search":
		return h.aiSearch(ctx, args, stepArgs)
	case "analyze_features":
		return h.aiAnalyze(ctx, args, stepArgs)
	case "map_action":
		return h.aiMapAction(ctx, args, stepArgs)
	case "update_features":
		return h.aiUpdateFeatures(ctx, args, stepArgs, allowWrite)
	case "delete_features":
		return h.aiDeleteFeatures(ctx, args, stepArgs, allowWrite)
	default:
		return toolError("未知工具: " + name)
	}
}

func toolError(msg string) aiToolOutcome {
	b, _ := json.Marshal(map[string]any{"error": msg})
	return aiToolOutcome{Result: string(b), Step: &aiStep{Tool: "error", Summary: msg}}
}

func toolJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func (h *Handlers) aiListLayers(ctx context.Context) aiToolOutcome {
	layers, err := h.readableLayers(ctx)
	if err != nil {
		return toolError(err.Error())
	}
	external, _ := h.readableExternalLayers(ctx)
	type fieldInfo struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	type layerInfo struct {
		Name         string      `json:"name"`
		Title        string      `json:"title"`
		Kind         string      `json:"kind"` // postgis / external
		GeometryType string      `json:"geometryType,omitempty"`
		SourceType   string      `json:"sourceType,omitempty"`
		Fields       []fieldInfo `json:"fields,omitempty"`
		FeatureCount int64       `json:"featureCount,omitempty"`
		Bounds       *[4]float64 `json:"bounds,omitempty"`
		Visibility   string      `json:"visibility,omitempty"`
	}
	out := make([]layerInfo, 0, len(layers)+len(external))
	for _, l := range layers {
		li := layerInfo{Name: l.Name, Title: l.Title, Kind: "postgis", GeometryType: l.GeometryType, FeatureCount: l.FeatureCount, Bounds: l.Bounds, Visibility: l.Visibility}
		for _, f := range l.Fields {
			li.Fields = append(li.Fields, fieldInfo{Name: f.Name, Type: string(f.Type)})
		}
		out = append(out, li)
	}
	for _, l := range external {
		out = append(out, layerInfo{Name: l.Name, Title: l.Title, Kind: "external", SourceType: l.SourceType, Visibility: l.Visibility})
	}
	step := &aiStep{Tool: "list_layers", Summary: fmt.Sprintf("读取 %d 个图层", len(out))}
	return aiToolOutcome{Result: toolJSON(map[string]any{"layers": out}), Step: step}
}

func (a aiQueryArgs) toFeatureQuery(defaultLimit int) (store.FeatureQuery, error) {
	q := store.FeatureQuery{
		OutSRID:    4326,
		BBoxSRID:   4326,
		Limit:      defaultLimit,
		TextSearch: strings.TrimSpace(a.Q),
		OrderBy:    a.OrderBy,
		OrderDesc:  strings.EqualFold(a.Order, "desc"),
		IDs:        a.IDs,
	}
	if a.Limit > 0 {
		q.Limit = a.Limit
	}
	if q.Limit > 50 {
		q.Limit = 50
	}
	if q.Limit <= 0 {
		q.Limit = 10
	}
	for _, f := range a.Filters {
		if strings.TrimSpace(f.Field) == "" {
			continue
		}
		op := f.Op
		if op == "" {
			op = "eq"
		}
		q.Filters = append(q.Filters, store.Filter{Field: f.Field, Op: op, Value: f.Value})
	}
	if len(a.Bbox) > 0 {
		if len(a.Bbox) != 4 {
			return q, fmt.Errorf("bbox 需要 4 个数字")
		}
		b := [4]float64{a.Bbox[0], a.Bbox[1], a.Bbox[2], a.Bbox[3]}
		q.BBox = &b
	}
	return q, nil
}

func (h *Handlers) aiGetLayer(ctx context.Context, name string) (*models.Layer, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("缺少 layer")
	}
	l, err := h.store.GetLayerByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("图层不存在: %s", name)
	}
	if !h.layerReadAllowed(ctx, l) {
		return nil, fmt.Errorf("没有图层 %s 的读取权限", name)
	}
	return l, nil
}

func compactFeatures(raw json.RawMessage) ([]json.RawMessage, []map[string]any) {
	var fc struct {
		Features []json.RawMessage `json:"features"`
	}
	_ = json.Unmarshal(raw, &fc)
	compact := make([]map[string]any, 0, len(fc.Features))
	for _, f := range fc.Features {
		var x struct {
			ID         any            `json:"id"`
			Properties map[string]any `json:"properties"`
		}
		if json.Unmarshal(f, &x) == nil {
			compact = append(compact, map[string]any{"id": x.ID, "properties": x.Properties})
		}
	}
	return fc.Features, compact
}

func (h *Handlers) aiQueryFeatures(ctx context.Context, args aiQueryArgs, stepArgs json.RawMessage) aiToolOutcome {
	l, err := h.aiGetLayer(ctx, args.Layer)
	if err != nil {
		return toolError(err.Error())
	}
	q, err := args.toFeatureQuery(10)
	if err != nil {
		return toolError(err.Error())
	}
	raw, total, err := h.store.FeaturesGeoJSON(ctx, l, q)
	if err != nil {
		return toolError(err.Error())
	}
	rawFeatures, compact := compactFeatures(raw)
	results := make([]aiResult, 0, len(rawFeatures))
	for _, f := range rawFeatures {
		results = append(results, aiResult{Layer: l.Name, LayerTitle: l.Title, Feature: f, Total: total})
	}
	step := &aiStep{
		Tool:    "query_features",
		Args:    stepArgs,
		Summary: fmt.Sprintf("查询 %s：共 %d 个，返回 %d 个", l.Name, total, len(rawFeatures)),
	}
	return aiToolOutcome{
		Result:  toolJSON(map[string]any{"layer": l.Name, "total": total, "features": compact}),
		Step:    step,
		Results: results,
	}
}

func (h *Handlers) aiLayerStats(ctx context.Context, args aiQueryArgs, stepArgs json.RawMessage) aiToolOutcome {
	l, err := h.aiGetLayer(ctx, args.Layer)
	if err != nil {
		return toolError(err.Error())
	}
	stats, err := h.store.FieldStats(ctx, l)
	if err != nil {
		return toolError(err.Error())
	}
	type compactStat struct {
		Field    string             `json:"field"`
		Type     string             `json:"type"`
		Count    int64              `json:"count"`
		Nulls    int64              `json:"nulls"`
		Distinct int64              `json:"distinct"`
		Min      any                `json:"min,omitempty"`
		Max      any                `json:"max,omitempty"`
		Top      []store.ValueCount `json:"top,omitempty"`
	}
	out := make([]compactStat, 0, len(stats))
	for _, s := range stats {
		top := s.Top
		if len(top) > 5 {
			top = top[:5]
		}
		out = append(out, compactStat{Field: s.Field, Type: s.Type, Count: s.Count, Nulls: s.Nulls, Distinct: s.Distinct, Min: s.Min, Max: s.Max, Top: top})
	}
	step := &aiStep{Tool: "layer_stats", Args: stepArgs, Summary: fmt.Sprintf("统计 %s 的 %d 个字段", l.Name, len(out))}
	return aiToolOutcome{Result: toolJSON(map[string]any{"layer": l.Name, "stats": out}), Step: step}
}

func (h *Handlers) aiSearch(ctx context.Context, args aiQueryArgs, stepArgs json.RawMessage) aiToolOutcome {
	q := strings.TrimSpace(args.Q)
	if q == "" {
		return toolError("缺少 q")
	}
	limit := args.Limit
	if limit <= 0 || limit > 10 {
		limit = 5
	}
	layers, err := h.readableLayers(ctx)
	if err != nil {
		return toolError(err.Error())
	}
	lower := strings.ToLower(q)
	layerHits := make([]map[string]any, 0)
	featureHits := make([]map[string]any, 0)
	results := make([]aiResult, 0)
	for i := range layers {
		l := &layers[i]
		if args.Layer != "" && l.Name != args.Layer {
			continue
		}
		if strings.Contains(strings.ToLower(l.Name), lower) ||
			strings.Contains(strings.ToLower(l.Title), lower) ||
			strings.Contains(strings.ToLower(l.Description), lower) {
			layerHits = append(layerHits, map[string]any{"name": l.Name, "title": l.Title, "geometryType": l.GeometryType})
		}
		hasText := false
		for _, f := range l.Fields {
			if f.Type == models.FieldText {
				hasText = true
				break
			}
		}
		if !hasText {
			continue
		}
		raw, total, err := h.store.FeaturesGeoJSON(ctx, l, store.FeatureQuery{
			Limit: limit, OutSRID: 4326, TextSearch: q,
		})
		if err != nil {
			continue
		}
		rawFeatures, compact := compactFeatures(raw)
		for idx, f := range rawFeatures {
			featureHits = append(featureHits, map[string]any{"layer": l.Name, "feature": compact[idx]})
			results = append(results, aiResult{Layer: l.Name, LayerTitle: l.Title, Feature: f, Total: total})
		}
	}
	step := &aiStep{Tool: "search", Args: stepArgs, Summary: fmt.Sprintf("搜索「%s」：图层 %d 个，要素 %d 个", q, len(layerHits), len(featureHits))}
	return aiToolOutcome{
		Result:  toolJSON(map[string]any{"layers": layerHits, "features": featureHits}),
		Step:    step,
		Results: results,
	}
}

func (h *Handlers) aiAnalyze(ctx context.Context, args aiQueryArgs, stepArgs json.RawMessage) aiToolOutcome {
	l, err := h.aiGetLayer(ctx, args.Layer)
	if err != nil {
		return toolError(err.Error())
	}
	if args.Op == "" {
		return toolError("缺少 op")
	}
	q, err := args.toFeatureQuery(20)
	if err != nil {
		return toolError(err.Error())
	}
	raw, total, err := h.store.AnalyzeFeatures(ctx, l, q, store.AnalyzeRequest{
		Op: args.Op, Distance: args.Distance, Tolerance: args.Tolerance, OutSRID: 4326, Limit: 20,
	})
	if err != nil {
		return toolError(err.Error())
	}
	rawFeatures, compact := compactFeatures(raw)
	if len(compact) > 5 {
		compact = compact[:5]
	}
	step := &aiStep{Tool: "analyze_features", Args: stepArgs, Summary: fmt.Sprintf("对 %s 执行 %s：%d 个结果", l.Name, args.Op, total)}
	results := make([]aiResult, 0, len(rawFeatures))
	for _, f := range rawFeatures {
		results = append(results, aiResult{Layer: l.Name, LayerTitle: l.Title, Feature: f, Total: total})
	}
	return aiToolOutcome{
		Result:  toolJSON(map[string]any{"layer": l.Name, "op": args.Op, "total": total, "features": compact}),
		Step:    step,
		Results: results,
	}
}

func (h *Handlers) aiMapAction(ctx context.Context, args aiQueryArgs, stepArgs json.RawMessage) aiToolOutcome {
	action := strings.TrimSpace(args.Action)
	allowed := map[string]bool{
		"fly_to_layer": true, "fly_to_bounds": true, "highlight_features": true,
		"open_data": true, "set_layer_visibility": true,
	}
	if !allowed[action] {
		return toolError("不支持的 map_action: " + action)
	}
	if args.Layer != "" && !h.aiLayerVisible(ctx, args.Layer) {
		return toolError("图层不存在或无权访问: " + args.Layer)
	}
	act := aiAction{
		Type:       action,
		Layer:      args.Layer,
		Bbox:       args.Bbox4(),
		FeatureIDs: args.IDs,
		Query:      args.Q,
	}
	if len(args.Bounds) == 4 {
		b := [4]float64{args.Bounds[0], args.Bounds[1], args.Bounds[2], args.Bounds[3]}
		act.Bounds = &b
	}
	var visible *bool
	// 从原始参数中读取 visible。
	var raw struct {
		Visible *bool `json:"visible"`
	}
	if json.Unmarshal(stepArgs, &raw) == nil {
		visible = raw.Visible
	}
	act.Visible = visible
	step := &aiStep{Tool: "map_action", Args: stepArgs, Summary: "地图动作: " + action}
	if args.Layer != "" {
		step.Summary += " · " + args.Layer
	}
	return aiToolOutcome{Result: toolJSON(map[string]any{"ok": true, "action": act}), Step: step, Actions: []aiAction{act}}
}

func (a aiQueryArgs) Bbox4() *[4]float64 {
	if len(a.Bbox) != 4 {
		return nil
	}
	return &[4]float64{a.Bbox[0], a.Bbox[1], a.Bbox[2], a.Bbox[3]}
}

func (h *Handlers) aiWriteLayer(ctx context.Context, name string) (*models.Layer, error) {
	l, err := h.aiGetLayer(ctx, name)
	if err != nil {
		return nil, err
	}
	if !h.layerWriteAllowed(ctx, l) {
		return nil, fmt.Errorf("没有图层 %s 的写入权限", name)
	}
	return l, nil
}

func (h *Handlers) aiUpdateFeatures(ctx context.Context, args aiQueryArgs, stepArgs json.RawMessage, allowWrite bool) aiToolOutcome {
	if !allowWrite {
		return toolError("写操作未启用：请用户在前端打开「允许写操作」后再试")
	}
	l, err := h.aiWriteLayer(ctx, args.Layer)
	if err != nil {
		return toolError(err.Error())
	}
	if len(args.Properties) == 0 {
		return toolError("缺少 properties")
	}
	q, err := args.toFeatureQuery(50)
	if err != nil {
		return toolError(err.Error())
	}
	if len(q.IDs) == 0 && len(q.Filters) == 0 && q.BBox == nil {
		return toolError("必须提供 ids 或 filters/bbox，拒绝批量更新全部要素")
	}
	affected, err := h.store.UpdateFeaturesWhere(ctx, l, q, args.Properties)
	if err != nil {
		return toolError(err.Error())
	}
	h.invalidateLayer(l.Name)
	h.aiAudit(ctx, "ai.update_features", l.Name, map[string]any{
		"ids": args.IDs, "filters": args.Filters, "bbox": args.Bbox,
		"properties": args.Properties, "affected": affected,
	})
	step := &aiStep{Tool: "update_features", Args: stepArgs, Summary: fmt.Sprintf("更新 %s：%d 个要素", l.Name, affected)}
	return aiToolOutcome{Result: toolJSON(map[string]any{"affected": affected}), Step: step}
}

func (h *Handlers) aiDeleteFeatures(ctx context.Context, args aiQueryArgs, stepArgs json.RawMessage, allowWrite bool) aiToolOutcome {
	if !allowWrite {
		return toolError("写操作未启用：请用户在前端打开「允许写操作」后再试")
	}
	l, err := h.aiWriteLayer(ctx, args.Layer)
	if err != nil {
		return toolError(err.Error())
	}
	q, err := args.toFeatureQuery(50)
	if err != nil {
		return toolError(err.Error())
	}
	if len(q.IDs) == 0 && len(q.Filters) == 0 && q.BBox == nil {
		return toolError("必须提供 ids 或 filters/bbox，拒绝删除全部要素")
	}
	affected, err := h.store.DeleteFeaturesWhere(ctx, l, q)
	if err != nil {
		return toolError(err.Error())
	}
	h.invalidateLayer(l.Name)
	h.aiAudit(ctx, "ai.delete_features", l.Name, map[string]any{
		"ids": args.IDs, "filters": args.Filters, "bbox": args.Bbox, "affected": affected,
	})
	step := &aiStep{Tool: "delete_features", Args: stepArgs, Summary: fmt.Sprintf("删除 %s：%d 个要素", l.Name, affected)}
	return aiToolOutcome{Result: toolJSON(map[string]any{"affected": affected}), Step: step}
}

func (h *Handlers) aiLayerVisible(ctx context.Context, name string) bool {
	if l, err := h.store.GetLayerByName(ctx, name); err == nil {
		return h.layerReadAllowed(ctx, l)
	}
	if l, err := h.store.GetExternalLayerByName(ctx, name); err == nil {
		return h.externalReadAllowed(ctx, l)
	}
	return false
}

func (h *Handlers) aiAudit(ctx context.Context, action, layer string, detail map[string]any) {
	user, _ := auth.UserFrom(ctx)
	entry := models.AuditLog{
		Method: "AI", Path: action, Status: 200, Layer: layer,
		Detail: json.RawMessage("{}"),
	}
	if user != nil {
		entry.UserID = user.ID
		entry.Username = user.Username
		entry.Role = user.Role
	}
	if b, err := json.Marshal(detail); err == nil {
		entry.Detail = b
	}
	_ = h.store.InsertAuditLog(ctx, entry)
}
