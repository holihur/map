// Package tiles 定义瓦片矩阵集（TileMatrixSet）与瓦片范围计算。
package tiles

import "math"

// MercatorExtent 是 Web Mercator（EPSG:3857）在米制下的半幅范围。
const MercatorExtent = 20037508.342789244

// Grid 一个瓦片矩阵集。
type Grid struct {
	ID       string     `json:"id"`
	CRS      string     `json:"crs"`
	SRID     int        `json:"srid"`
	TileSize int        `json:"tileSize"`
	Bounds   [4]float64 `json:"bounds"`
	Scheme   string     `json:"scheme"` // quad（2^z × 2^z）| crs84（2^(z+1) × 2^z）
}

var grids = map[string]Grid{
	"WebMercatorQuad": {
		ID:       "WebMercatorQuad",
		CRS:      "EPSG:3857",
		SRID:     3857,
		TileSize: 256,
		Bounds:   [4]float64{-MercatorExtent, -MercatorExtent, MercatorExtent, MercatorExtent},
		Scheme:   "quad",
	},
	"WorldCRS84Quad": {
		ID:       "WorldCRS84Quad",
		CRS:      "CRS:84",
		SRID:     4326,
		TileSize: 256,
		Bounds:   [4]float64{-180, -90, 180, 90},
		Scheme:   "crs84",
	},
}

// Default 返回默认矩阵集（WebMercatorQuad）。
func Default() Grid { return grids["WebMercatorQuad"] }

// Get 按 ID 获取矩阵集。
func Get(id string) (Grid, bool) {
	g, ok := grids[id]
	return g, ok
}

// All 返回全部矩阵集（稳定的展示顺序）。
func All() []Grid {
	return []Grid{grids["WebMercatorQuad"], grids["WorldCRS84Quad"]}
}

// MatrixWidth 返回某缩放级的列数。
func (g Grid) MatrixWidth(z int) int {
	if z < 0 || z > 30 {
		return 0
	}
	if g.Scheme == "crs84" {
		return 1 << uint(z+1)
	}
	return 1 << uint(z)
}

// MatrixHeight 返回某缩放级的行数。
func (g Grid) MatrixHeight(z int) int {
	if z < 0 || z > 30 {
		return 0
	}
	return 1 << uint(z)
}

// Contains 判断瓦片坐标是否在矩阵集范围内。
func (g Grid) Contains(z, x, y int) bool {
	if z < 0 || z > 30 || x < 0 || y < 0 {
		return false
	}
	return x < g.MatrixWidth(z) && y < g.MatrixHeight(z)
}

// Envelope 返回瓦片在矩阵集坐标系下的矩形范围 [minx,miny,maxx,maxy]。
func (g Grid) Envelope(z, x, y int) [4]float64 {
	mw := float64(g.MatrixWidth(z))
	mh := float64(g.MatrixHeight(z))
	if mw == 0 || mh == 0 {
		return [4]float64{}
	}
	spanX := (g.Bounds[2] - g.Bounds[0]) / mw
	spanY := (g.Bounds[3] - g.Bounds[1]) / mh
	minx := g.Bounds[0] + float64(x)*spanX
	maxy := g.Bounds[3] - float64(y)*spanY
	return [4]float64{minx, maxy - spanY, minx + spanX, maxy}
}

// Resolution 返回某缩放级每像素代表的地图单位。
func (g Grid) Resolution(z int) float64 {
	return math.Abs(g.Envelope(z, 0, 0)[2]-g.Envelope(z, 0, 0)[0]) / float64(g.TileSize)
}
