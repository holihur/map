package models

import "testing"

func TestGeomTypeSQL(t *testing.T) {
	cases := []struct {
		in   Layer
		want string
	}{
		{Layer{GeometryType: "POINT", SRID: 4326}, "geometry(POINT,4326)"},
		{Layer{GeometryType: "", SRID: 3857}, "geometry(GEOMETRY,3857)"},
		{Layer{GeometryType: "POLYGON", SRID: 0}, "geometry(POLYGON,4326)"},
	}
	for _, c := range cases {
		if got := c.in.GeomTypeSQL(); got != c.want {
			t.Errorf("GeomTypeSQL(%+v) = %q, want %q", c.in, got, c.want)
		}
	}
}
