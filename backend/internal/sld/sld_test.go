package sld

import "testing"

const sampleSLD = `<?xml version="1.0"?>
<StyledLayerDescriptor version="1.0.0" xmlns="http://www.opengis.net/sld">
  <NamedLayer>
    <Name>countries</Name>
    <UserStyle>
      <FeatureTypeStyle>
        <Rule>
          <Name>small</Name>
          <MaxScaleDenominator>1000000</MaxScaleDenominator>
          <PolygonSymbolizer>
            <Fill><CssParameter name="fill">#ff0000</CssParameter>
                  <CssParameter name="fill-opacity">0.5</CssParameter></Fill>
            <Stroke><CssParameter name="stroke">#00ff00</CssParameter>
                    <CssParameter name="stroke-width">3</CssParameter></Stroke>
          </PolygonSymbolizer>
        </Rule>
        <Rule>
          <Name>large</Name>
          <MinScaleDenominator>1000000</MinScaleDenominator>
          <PolygonSymbolizer>
            <Fill><SvgParameter name="fill">#0000ff</SvgParameter></Fill>
          </PolygonSymbolizer>
        </Rule>
      </FeatureTypeStyle>
    </UserStyle>
  </NamedLayer>
  <NamedLayer>
    <Name>cities</Name>
    <UserStyle><FeatureTypeStyle><Rule>
      <PointSymbolizer><Graphic><Mark>
        <Fill><CssParameter name="fill">#ffee00</CssParameter></Fill>
      </Mark><Size>10</Size></Graphic></PointSymbolizer>
    </Rule></FeatureTypeStyle></UserStyle>
  </NamedLayer>
</StyledLayerDescriptor>`

func TestParseAndSelect(t *testing.T) {
	doc, err := Parse([]byte(sampleSLD))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(doc.RulesFor("countries")) != 2 {
		t.Fatalf("countries rules = %d", len(doc.RulesFor("countries")))
	}

	// 小比例尺（更放大）→ 第一条 #ff0000, opacity 0.5 → alpha 127
	r := Select(doc.RulesFor("countries"), 500000)
	if r == nil || r.Name != "small" {
		t.Fatalf("expected small rule, got %+v", r)
	}
	if r.Fill == nil || r.Fill.R != 255 || r.Fill.G != 0 || r.Fill.B != 0 {
		t.Fatalf("fill = %+v", r.Fill)
	}
	if r.Fill.A < 120 || r.Fill.A > 135 {
		t.Fatalf("alpha = %d", r.Fill.A)
	}
	if r.Stroke == nil || r.Stroke.G != 255 {
		t.Fatalf("stroke = %+v", r.Stroke)
	}
	if r.StrokeWidth == nil || *r.StrokeWidth != 3 {
		t.Fatalf("stroke width = %v", r.StrokeWidth)
	}

	// 大比例尺 → 第二条 #0000ff
	r = Select(doc.RulesFor("countries"), 5000000)
	if r == nil || r.Name != "large" || r.Fill.B != 255 {
		t.Fatalf("expected large blue rule, got %+v", r)
	}

	// 点符号：Size 10 → 半径 5
	pr := Select(doc.RulesFor("cities"), 1000)
	if pr == nil || pr.PointRadius == nil || *pr.PointRadius != 5 {
		t.Fatalf("point radius = %+v", pr)
	}
	if pr.Fill == nil || pr.Fill.R != 255 || pr.Fill.G != 238 {
		t.Fatalf("point fill = %+v", pr.Fill)
	}
}

func TestParseInvalid(t *testing.T) {
	if _, err := Parse([]byte("<foo/>")); err == nil {
		t.Fatal("expected error for non-SLD document")
	}
}
