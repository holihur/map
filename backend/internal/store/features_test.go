package store

import (
	"strings"
	"testing"
)

func TestGeomFromGeoJSONExpr(t *testing.T) {
	cases := []struct {
		name      string
		input     int
		layer     int
		want      string
		wantTrans bool
	}{
		{"same srid", 4326, 4326, "ST_SetSRID(ST_GeomFromGeoJSON($1), 4326)", false},
		{"default to 4326", 0, 4326, "ST_SetSRID(ST_GeomFromGeoJSON($1), 4326)", false},
		{"reproject", 4326, 3857, "ST_Transform(ST_SetSRID(ST_GeomFromGeoJSON($1), 4326), 3857)", true},
		{"reproject from 3857", 3857, 4490, "ST_Transform(ST_SetSRID(ST_GeomFromGeoJSON($1), 3857), 4490)", true},
	}
	for _, c := range cases {
		got := geomFromGeoJSONExpr("$1", c.input, c.layer)
		if got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
		if strings.Contains(got, "ST_Transform") != c.wantTrans {
			t.Errorf("%s: transform presence mismatch: %q", c.name, got)
		}
	}
}

func TestColumnType(t *testing.T) {
	if _, err := columnType("text"); err != nil {
		t.Fatal(err)
	}
	if _, err := columnType("nope"); err == nil {
		t.Fatal("expected error for unknown type")
	}
}
