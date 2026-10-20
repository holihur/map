// Package sld 提供简化但实用的 SLD（Styled Layer Descriptor）解析：
// 支持 NamedLayer → UserStyle → FeatureTypeStyle → Rule（含比例尺范围）与
// Polygon/Line/Point 符号的 Fill/Stroke/Size（CssParameter 与 SvgParameter 均可）。
package sld

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

// RGBA 颜色。
type RGBA struct{ R, G, B, A uint8 }

// Rule 一条 SLD 规则。
type Rule struct {
	Name        string
	MinScale    float64
	MaxScale    float64
	Fill        *RGBA
	Stroke      *RGBA
	StrokeWidth *float64
	PointRadius *float64
	// HasFilter 为真时 Filter 包含可转译为属性条件的过滤表达式。
	HasFilter bool
	Filter    string
}

// Document 解析结果：按图层名分组的规则列表（"" 为未指定图层的默认规则）。
type Document struct {
	ByLayer map[string][]Rule
}

type param struct {
	XMLName xml.Name
	Name    string `xml:"name,attr"`
	Value   string `xml:",chardata"`
}

type fillT struct {
	Params []param `xml:",any"`
}

// FillColor 从参数中解析填充色。
func (f fillT) FillColor(defaultAlpha uint8) *RGBA {
	alpha := defaultAlpha
	var hex string
	for _, p := range f.Params {
		switch strings.ToLower(p.Name) {
		case "fill", "color":
			hex = strings.TrimSpace(p.Value)
		case "fill-opacity", "opacity":
			if v, err := strconv.ParseFloat(strings.TrimSpace(p.Value), 64); err == nil {
				alpha = clamp01(v) * 255
			}
		}
	}
	if hex == "" {
		return nil
	}
	c, ok := parseHex(hex, alpha)
	if !ok {
		return nil
	}
	return &c
}

// Width 解析描边宽度。
func (f fillT) Width() *float64 {
	for _, p := range f.Params {
		switch strings.ToLower(p.Name) {
		case "stroke-width", "width":
			if v, err := strconv.ParseFloat(strings.TrimSpace(p.Value), 64); err == nil {
				return &v
			}
		}
	}
	return nil
}

// StrokeColor 解析描边颜色。
func (f fillT) StrokeColor() *RGBA {
	var hex string
	alpha := uint8(255)
	for _, p := range f.Params {
		switch strings.ToLower(p.Name) {
		case "stroke", "color":
			hex = strings.TrimSpace(p.Value)
		case "stroke-opacity", "opacity":
			if v, err := strconv.ParseFloat(strings.TrimSpace(p.Value), 64); err == nil {
				alpha = clamp01(v) * 255
			}
		}
	}
	if hex == "" {
		return nil
	}
	c, ok := parseHex(hex, alpha)
	if !ok {
		return nil
	}
	return &c
}

type graphicT struct {
	Size float64 `xml:"Size"`
	Mark struct {
		Fill   fillT `xml:"Fill"`
		Stroke fillT `xml:"Stroke"`
	} `xml:"Mark"`
}

type ruleT struct {
	Name     string  `xml:"Name"`
	MinScale float64 `xml:"MinScaleDenominator"`
	MaxScale float64 `xml:"MaxScaleDenominator"`
	Filter   *struct {
		Inner string `xml:",innerxml"`
	} `xml:"Filter"`
	Polygon struct {
		Fill   fillT `xml:"Fill"`
		Stroke fillT `xml:"Stroke"`
	} `xml:"PolygonSymbolizer"`
	Line struct {
		Stroke fillT `xml:"Stroke"`
	} `xml:"LineSymbolizer"`
	Point struct {
		Fill    fillT    `xml:"Fill"`
		Stroke  fillT    `xml:"Stroke"`
		Size    float64  `xml:"Size"`
		Graphic graphicT `xml:"Graphic"`
	} `xml:"PointSymbolizer"`
}

type sldDoc struct {
	XMLName     xml.Name `xml:"StyledLayerDescriptor"`
	NamedLayers []struct {
		Name       string `xml:"Name"`
		UserStyles []struct {
			FeatureTypeStyles []struct {
				Rules []ruleT `xml:"Rule"`
			} `xml:"FeatureTypeStyle"`
		} `xml:"UserStyle"`
	} `xml:"NamedLayer"`
}

// Parse 解析 SLD 文档。
func Parse(data []byte) (*Document, error) {
	var doc sldDoc
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("SLD 解析失败: %w", err)
	}

	out := &Document{ByLayer: map[string][]Rule{}}
	push := func(layer string, r Rule) {
		out.ByLayer[layer] = append(out.ByLayer[layer], r)
	}
	for _, nl := range doc.NamedLayers {
		for _, us := range nl.UserStyles {
			for _, fts := range us.FeatureTypeStyles {
				for _, r := range fts.Rules {
					push(nl.Name, convert(r))
				}
			}
		}
	}
	if len(out.ByLayer) == 0 {
		return nil, fmt.Errorf("SLD 中未找到任何 NamedLayer/UserStyle 规则")
	}
	return out, nil
}

func convert(r ruleT) Rule {
	out := Rule{Name: r.Name, MinScale: r.MinScale, MaxScale: r.MaxScale}
	if r.Filter != nil && strings.TrimSpace(r.Filter.Inner) != "" {
		out.HasFilter = true
		out.Filter = "<Filter>" + r.Filter.Inner + "</Filter>"
	}

	switch {
	case r.Polygon.Fill.Params != nil || r.Polygon.Stroke.Params != nil:
		out.Fill = r.Polygon.Fill.FillColor(255)
		out.Stroke = r.Polygon.Stroke.StrokeColor()
		if w := r.Polygon.Stroke.Width(); w != nil {
			out.StrokeWidth = w
		}
	case r.Line.Stroke.Params != nil:
		out.Stroke = r.Line.Stroke.StrokeColor()
		if w := r.Line.Stroke.Width(); w != nil {
			out.StrokeWidth = w
		}
	default:
		// Point：优先 Graphic/Mark，其次直接 Fill/Stroke
		fill := r.Point.Fill
		stroke := r.Point.Stroke
		if fill.Params == nil {
			fill = r.Point.Graphic.Mark.Fill
		}
		if stroke.Params == nil {
			stroke = r.Point.Graphic.Mark.Stroke
		}
		out.Fill = fill.FillColor(230)
		out.Stroke = stroke.StrokeColor()
		size := r.Point.Size
		if size == 0 {
			size = r.Point.Graphic.Size
		}
		if size > 0 {
			radius := size / 2
			out.PointRadius = &radius
		}
	}
	return out
}

// RulesFor 返回指定图层的规则；若无匹配图层，则返回所有规则（合并）。
func (d *Document) RulesFor(layer string) []Rule {
	if d == nil {
		return nil
	}
	if rs, ok := d.ByLayer[layer]; ok {
		return rs
	}
	if rs, ok := d.ByLayer[""]; ok {
		return rs
	}
	var all []Rule
	for _, rs := range d.ByLayer {
		all = append(all, rs...)
	}
	return all
}

// Select 按比例尺选择第一条匹配的规则（SLD 语义：Min ≤ scale < Max，0 表示不限）。
func Select(rules []Rule, scaleDenominator float64) *Rule {
	for i := range rules {
		r := &rules[i]
		if r.MinScale > 0 && scaleDenominator < r.MinScale {
			continue
		}
		if r.MaxScale > 0 && scaleDenominator >= r.MaxScale {
			continue
		}
		return r
	}
	return nil
}

func clamp01(v float64) uint8 {
	switch {
	case v <= 0:
		return 0
	case v >= 1:
		return 255
	default:
		return uint8(v*255 + 0.5)
	}
}

func parseHex(hex string, alpha uint8) (RGBA, bool) {
	hex = strings.TrimPrefix(strings.TrimSpace(hex), "#")
	switch len(hex) {
	case 3:
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	case 6:
	default:
		return RGBA{}, false
	}
	n, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return RGBA{}, false
	}
	return RGBA{R: uint8(n >> 16), G: uint8(n >> 8 & 0xff), B: uint8(n & 0xff), A: alpha}, true
}
