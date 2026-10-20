// Package models 定义 MapServer 的领域模型。
package models

import (
	"encoding/json"
	"strconv"
	"time"
)

// FieldType 图层字段类型。
type FieldType string

const (
	FieldText      FieldType = "text"
	FieldInteger   FieldType = "integer"
	FieldBigInt    FieldType = "bigint"
	FieldDouble    FieldType = "double"
	FieldBoolean   FieldType = "boolean"
	FieldTimestamp FieldType = "timestamptz"
	FieldDate      FieldType = "date"
)

// Field 图层的一个属性字段（对应物理表的一列）。
type Field struct {
	Name     string    `json:"name"`
	Type     FieldType `json:"type"`
	Nullable bool      `json:"nullable,omitempty"`
}

// Layer 图层元数据，对应 GeoServer 的 featureType / Mapbox 的 source。
type Layer struct {
	ID           int64           `json:"id"`
	Name         string          `json:"name"`
	Title        string          `json:"title"`
	Description  string          `json:"description"`
	GeometryType string          `json:"geometryType"` // GEOMETRY / POINT / LINESTRING / POLYGON 等
	SRID         int             `json:"srid"`
	TableName    string          `json:"tableName"`
	Fields       []Field         `json:"fields"`
	MinZoom      int             `json:"minZoom"`
	MaxZoom      int             `json:"maxZoom"`
	Bounds       *[4]float64     `json:"bounds,omitempty"` // minx,miny,maxx,maxy (EPSG:4326)
	Style        json.RawMessage `json:"style"`
	StyleName    string          `json:"styleName,omitempty"`
	Metadata     json.RawMessage `json:"metadata"`
	FeatureCount int64           `json:"featureCount"`
	CreatedAt    time.Time       `json:"createdAt"`
	UpdatedAt    time.Time       `json:"updatedAt"`
}

// CreateLayerRequest 创建图层的请求体。
type CreateLayerRequest struct {
	Name         string          `json:"name"`
	Title        string          `json:"title"`
	Description  string          `json:"description"`
	GeometryType string          `json:"geometryType"`
	SRID         int             `json:"srid"`
	Fields       []Field         `json:"fields"`
	MinZoom      *int            `json:"minZoom"`
	MaxZoom      *int            `json:"maxZoom"`
	Style        json.RawMessage `json:"style"`
	Metadata     json.RawMessage `json:"metadata"`
}

// UpdateLayerRequest 更新图层的请求体（字段可选）。
type UpdateLayerRequest struct {
	Title       *string         `json:"title"`
	Description *string         `json:"description"`
	MinZoom     *int            `json:"minZoom"`
	MaxZoom     *int            `json:"maxZoom"`
	Style       json.RawMessage `json:"style"`
	StyleName   *string         `json:"styleName"`
	Metadata    json.RawMessage `json:"metadata"`
}

// Style 一套 MapLibre Style 规范样式，或一份 SLD 文档。
type Style struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	IsDefault bool            `json:"isDefault"`
	Style     json.RawMessage `json:"style"`
	SLD       string          `json:"sld,omitempty"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

// User 系统用户。角色：admin / editor / viewer。
type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	Role         string    `json:"role"`
	TokenVersion int       `json:"-"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// LoginRequest 登录请求体。
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// CreateUserRequest 创建用户请求体。
type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

// UpdateUserRequest 更新用户请求体。
type UpdateUserRequest struct {
	Password *string `json:"password"`
	Role     *string `json:"role"`
}

// CRS 坐标参考系（来自 PostGIS 的 spatial_ref_sys）。
type CRS struct {
	SRID     int    `json:"srid"`
	AuthName string `json:"authName"`
	AuthSRID int    `json:"authSrid"`
	Name     string `json:"name"`
}

// TransformLayerRequest 转换图层坐标系请求。
type TransformLayerRequest struct {
	SRID int `json:"srid"`
}

// CreateStyleRequest 创建样式请求体。
type CreateStyleRequest struct {
	Name      string          `json:"name"`
	IsDefault bool            `json:"isDefault"`
	Style     json.RawMessage `json:"style"`
	SLD       string          `json:"sld,omitempty"`
}

// GeomTypeSQL 返回用于 CREATE TABLE 的几何类型表达式。
func (l *Layer) GeomTypeSQL() string {
	gt := l.GeometryType
	if gt == "" {
		gt = "GEOMETRY"
	}
	srid := l.SRID
	if srid == 0 {
		srid = 4326
	}
	return "geometry(" + gt + "," + strconv.Itoa(srid) + ")"
}

// PropertyColumns 返回除空间/主键/时间戳外的属性列名。
func (l *Layer) PropertyColumns() []string {
	cols := make([]string, 0, len(l.Fields))
	for _, f := range l.Fields {
		cols = append(cols, f.Name)
	}
	return cols
}
