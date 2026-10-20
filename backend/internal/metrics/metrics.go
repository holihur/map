// Package metrics 提供轻量的 Prometheus 文本格式指标。
package metrics

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Registry 收集计数器与仪表。
type Registry struct {
	mu        sync.Mutex
	counters  map[string]float64
	help      map[string]string
	gauges    map[string]func() float64
	gaugeHelp map[string]string
}

// New 创建 Registry。
func New() *Registry {
	return &Registry{
		counters:  map[string]float64{},
		help:      map[string]string{},
		gauges:    map[string]func() float64{},
		gaugeHelp: map[string]string{},
	}
}

// Inc 计数器 +1。
func (r *Registry) Inc(name string, labels map[string]string) {
	r.Add(name, labels, 1)
}

// Add 计数器增加 v。
func (r *Registry) Add(name string, labels map[string]string, v float64) {
	key := name + labelString(labels)
	r.mu.Lock()
	r.counters[key] += v
	if _, ok := r.help[name]; !ok {
		r.help[name] = ""
	}
	r.mu.Unlock()
}

// Describe 设置指标的 HELP 文本。
func (r *Registry) Describe(name, help string) {
	r.mu.Lock()
	r.help[name] = help
	r.mu.Unlock()
}

// RegisterGauge 注册一个采集时求值的仪表。
func (r *Registry) RegisterGauge(name, help string, fn func() float64) {
	r.mu.Lock()
	r.gauges[name] = fn
	r.gaugeHelp[name] = help
	r.mu.Unlock()
}

// SetGauge 直接设置仪表值（内部存储为常量函数）。
func (r *Registry) SetGauge(name, help string, value float64) {
	v := value
	r.RegisterGauge(name, help, func() float64 { return v })
}

// Render 输出 Prometheus 文本格式。
func (r *Registry) Render() string {
	r.mu.Lock()
	counters := make(map[string]float64, len(r.counters))
	for k, v := range r.counters {
		counters[k] = v
	}
	help := make(map[string]string, len(r.help))
	for k, v := range r.help {
		help[k] = v
	}
	gauges := make(map[string]func() float64, len(r.gauges))
	for k, v := range r.gauges {
		gauges[k] = v
	}
	gaugeHelp := make(map[string]string, len(r.gaugeHelp))
	for k, v := range r.gaugeHelp {
		gaugeHelp[k] = v
	}
	r.mu.Unlock()

	var b strings.Builder

	byName := map[string][]string{}
	for key := range counters {
		name := metricName(key)
		byName[name] = append(byName[name], key)
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if h := help[name]; h != "" {
			fmt.Fprintf(&b, "# HELP %s %s\n", name, h)
		}
		fmt.Fprintf(&b, "# TYPE %s counter\n", name)
		keys := byName[name]
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(&b, "%s %g\n", key, counters[key])
		}
	}

	gaugeNames := make([]string, 0, len(gauges))
	for name := range gauges {
		gaugeNames = append(gaugeNames, name)
	}
	sort.Strings(gaugeNames)
	for _, name := range gaugeNames {
		if h := gaugeHelp[name]; h != "" {
			fmt.Fprintf(&b, "# HELP %s %s\n", name, h)
		}
		fmt.Fprintf(&b, "# TYPE %s gauge\n", name)
		fmt.Fprintf(&b, "%s %g\n", name, gauges[name]())
	}

	return b.String()
}

// metricName 从 "name{labels}" 中取出指标名。
func metricName(key string) string {
	if i := strings.IndexByte(key, '{'); i >= 0 {
		return key[:i]
	}
	return key
}

func labelString(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", k, labels[k]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}
