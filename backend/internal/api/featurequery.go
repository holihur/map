package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"mapserver/internal/store"
)

// parseFeatureQuery 从 HTTP 查询参数构造 FeatureQuery，供要素查询与导出复用。
func (h *Handlers) parseFeatureQuery(r *http.Request) (store.FeatureQuery, error) {
	q := store.FeatureQuery{
		Limit:      h.cfg.MaxFeatures,
		OutSRID:    4326,
		BBoxSRID:   4326,
		OrderBy:    r.URL.Query().Get("orderBy"),
		OrderDesc:  strings.EqualFold(r.URL.Query().Get("order"), "desc"),
		TextSearch: strings.TrimSpace(r.URL.Query().Get("q")),
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q.Limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			q.Offset = n
		}
	}
	if v := r.URL.Query().Get("outSrid"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			q.OutSRID = n
		}
	}
	if v := r.URL.Query().Get("bboxSrid"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			q.BBoxSRID = n
		}
	}
	if v := r.URL.Query().Get("simplify"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			q.Simplify = f
		}
	}
	if v := r.URL.Query().Get("ids"); v != "" {
		for _, part := range strings.Split(v, ",") {
			if part = strings.TrimSpace(part); part == "" {
				continue
			}
			id, err := strconv.ParseInt(part, 10, 64)
			if err != nil {
				return q, fmt.Errorf("ids 参数非法")
			}
			q.IDs = append(q.IDs, id)
		}
	}
	if v := r.URL.Query().Get("bbox"); v != "" {
		parts := strings.Split(v, ",")
		if len(parts) != 4 {
			return q, fmt.Errorf("bbox 需要 4 个逗号分隔的数字")
		}
		var b [4]float64
		for i, p := range parts {
			f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
			if err != nil {
				return q, fmt.Errorf("bbox 解析失败")
			}
			b[i] = f
		}
		q.BBox = &b
	}
	// 属性过滤：f.<field> 或 f.<field>.<op>
	for key, vals := range r.URL.Query() {
		if !strings.HasPrefix(key, "f.") || len(vals) == 0 {
			continue
		}
		rest := strings.TrimPrefix(key, "f.")
		field, op := rest, "eq"
		if idx := strings.LastIndex(rest, "."); idx > 0 {
			field, op = rest[:idx], rest[idx+1:]
		}
		q.Filters = append(q.Filters, store.Filter{Field: field, Op: op, Value: vals[0]})
	}
	return q, nil
}
