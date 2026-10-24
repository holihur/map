package api

import (
	"context"
	"net/http"
	"testing"
)

func TestOGCAPIFeatures(t *testing.T) {
	srv := newServer(t, testCfg)

	// landing + conformance
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/ogcapi", "", nil, ""), nil)
	var conf struct {
		ConformsTo []string `json:"conformsTo"`
	}
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/ogcapi/conformance", "", nil, ""), &conf)
	if len(conf.ConformsTo) < 3 {
		t.Fatalf("conformance = %+v", conf.ConformsTo)
	}

	// collections
	var cols struct {
		Collections []struct {
			ID string `json:"id"`
		} `json:"collections"`
	}
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/ogcapi/collections", "", nil, ""), &cols)
	found := false
	for _, c := range cols.Collections {
		if c.ID == "cities" {
			found = true
		}
	}
	if !found {
		t.Fatalf("collections missing cities: %+v", cols.Collections)
	}

	// items pagination + links
	var items struct {
		Type           string `json:"type"`
		NumberMatched  int64  `json:"numberMatched"`
		NumberReturned int    `json:"numberReturned"`
		Features       []any  `json:"features"`
		Links          []struct {
			Rel string `json:"rel"`
		} `json:"links"`
	}
	resp := doReq(t, http.MethodGet, srv.URL+"/ogcapi/collections/cities/items?limit=2", "", nil, "")
	if ct := resp.Header.Get("Content-Type"); ct == "" {
		t.Fatal("missing content type")
	}
	readJSON(t, resp, &items)
	if items.Type != "FeatureCollection" || items.NumberMatched != 10 || items.NumberReturned != 2 || len(items.Features) != 2 {
		t.Fatalf("items = %+v", items)
	}
	if len(items.Links) == 0 {
		t.Fatal("items missing links")
	}

	// single item
	var feat struct {
		ID int64 `json:"id"`
	}
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/ogcapi/collections/cities/items/1", "", nil, ""), &feat)
	if feat.ID != 1 {
		t.Fatalf("item id = %d", feat.ID)
	}

	// bbox filter
	var bboxItems struct {
		NumberMatched int64 `json:"numberMatched"`
	}
	readJSON(t, doReq(t, http.MethodGet, srv.URL+"/ogcapi/collections/cities/items?bbox=115,39,117,41", "", nil, ""), &bboxItems)
	if bboxItems.NumberMatched < 1 {
		t.Fatalf("bbox numberMatched = %d", bboxItems.NumberMatched)
	}

	// unknown collection
	wantStatus(t, doReq(t, http.MethodGet, srv.URL+"/ogcapi/collections/does_not_exist/items", "", nil, ""), http.StatusNotFound)

	// health / readiness
	wantStatus(t, doReq(t, http.MethodGet, srv.URL+"/healthz", "", nil, ""), http.StatusOK)
	wantStatus(t, doReq(t, http.MethodGet, srv.URL+"/readyz", "", nil, ""), http.StatusOK)
}

func TestAIBaseURLValidation(t *testing.T) {
	ctx := context.Background()
	if validAIBaseURL(ctx, "http://127.0.0.1:11434/v1", false) != true {
		t.Fatal("loopback should be allowed for local models")
	}
	if validAIBaseURL(ctx, "http://192.168.1.10/v1", false) {
		t.Fatal("private IP should be rejected")
	}
	if !validAIBaseURL(ctx, "http://192.168.1.10/v1", true) {
		t.Fatal("private IP should be allowed with AI_ALLOW_PRIVATE")
	}
	if validAIBaseURL(ctx, "ftp://example.com", false) {
		t.Fatal("non-http scheme should be rejected")
	}
}
