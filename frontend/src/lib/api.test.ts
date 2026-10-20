import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  API_BASE,
  ApiError,
  UNAUTHORIZED_EVENT,
  deleteFeature,
  exportAllUrl,
  exportUrl,
  getAuthToken,
  importLayer,
  listLayers,
  rasterTileUrlTemplate,
  setAuthToken,
  tileUrlTemplate,
  tileUrlTemplateTMS,
  wmsGetFeatureInfoUrl,
  wmsGetMapUrl,
  wfsCapabilitiesUrl,
} from "@/lib/api";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

beforeEach(() => {
  setAuthToken(null);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("auth token", () => {
  it("设置与清除 token", () => {
    expect(getAuthToken()).toBeNull();
    setAuthToken("abc");
    expect(getAuthToken()).toBe("abc");
    expect(localStorage.getItem("mapserver_token")).toBe("abc");
    setAuthToken(null);
    expect(getAuthToken()).toBeNull();
    expect(localStorage.getItem("mapserver_token")).toBeNull();
  });

  it("请求时附带 Bearer 头", async () => {
    setAuthToken("tok-123");
    const fetchMock = vi.fn(async (_url: string, _init?: RequestInit) => jsonResponse({ layers: [] }));
    vi.stubGlobal("fetch", fetchMock);

    await listLayers();

    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    const headers = init.headers as Record<string, string>;
    expect(headers.Authorization).toBe("Bearer tok-123");
    expect(headers["Content-Type"]).toBe("application/json");
  });
});

describe("error handling", () => {
  it("非 2xx 抛出 ApiError 并带服务端消息", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => jsonResponse({ error: "图层不存在" }, 404)));
    await expect(listLayers()).rejects.toBeInstanceOf(ApiError);
    await expect(listLayers()).rejects.toThrow("图层不存在");
  });

  it("401 触发 UNAUTHORIZED_EVENT", async () => {
    const handler = vi.fn();
    window.addEventListener(UNAUTHORIZED_EVENT, handler);
    vi.stubGlobal("fetch", vi.fn(async () => jsonResponse({ error: "unauthorized" }, 401)));

    await expect(listLayers()).rejects.toBeInstanceOf(ApiError);
    expect(handler).toHaveBeenCalledTimes(1);
    window.removeEventListener(UNAUTHORIZED_EVENT, handler);
  });

  it("204 返回 undefined", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(null, { status: 204 })));
    await expect(deleteFeature("l", 1)).resolves.toBeUndefined();
  });
});

describe("multipart 上传", () => {
  it("不设置 JSON Content-Type", async () => {
    const fetchMock = vi.fn(async (_url: string, _init?: RequestInit) =>
      jsonResponse({ imported: 3, format: "geojson", layer: "l" })
    );
    vi.stubGlobal("fetch", fetchMock);

    const file = new File(["{}"], "a.geojson", { type: "application/json" });
    const res = await importLayer("l", file, 4326);

    expect(res.imported).toBe(3);
    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(init.body).toBeInstanceOf(FormData);
    const headers = (init.headers as Record<string, string>) ?? {};
    expect(headers["Content-Type"]).toBeUndefined();
  });
});

describe("URL 构造", () => {
  it("瓦片模板", () => {
    expect(tileUrlTemplate("cities")).toBe(`${API_BASE}/api/tiles/cities/{z}/{x}/{y}.mvt`);
    expect(tileUrlTemplateTMS("cities", "WorldCRS84Quad")).toBe(
      `${API_BASE}/api/tiles/cities/WorldCRS84Quad/{z}/{x}/{y}.mvt`
    );
    expect(rasterTileUrlTemplate("cities", "WebMercatorQuad")).toBe(
      `${API_BASE}/api/tiles/cities/WebMercatorQuad/{z}/{x}/{y}.png`
    );
  });

  it("导出地址", () => {
    expect(exportUrl("cities", "gpkg")).toBe(`${API_BASE}/api/layers/cities/export?format=gpkg`);
    expect(exportUrl("cities", "csv", 3857)).toContain("outSrid=3857");
    expect(exportAllUrl()).toBe(`${API_BASE}/api/export?format=gpkg`);
    expect(wfsCapabilitiesUrl()).toContain("REQUEST=GetCapabilities");
  });

  it("WMS GetMap 处理 4326 轴序", () => {
    const url = wmsGetMapUrl({
      layer: "countries",
      srid: 4326,
      bbox: [110, 20, 130, 50], // minx,miny,maxx,maxy
      width: 800,
      height: 400,
      swapAxes: true,
    });
    // 1.3.0 下应输出 miny,minx,maxy,maxx
    expect(decodeURIComponent(url)).toContain("BBOX=20,110,50,130");
    expect(url).toContain("LAYERS=countries");
    expect(url).toContain("WIDTH=800");

    const noSwap = wmsGetMapUrl({
      layer: "cities",
      srid: 3857,
      bbox: [1, 2, 3, 4],
      width: 10,
      height: 10,
    });
    expect(decodeURIComponent(noSwap)).toContain("BBOX=1,2,3,4");
  });

  it("WMS GetFeatureInfo 地址", () => {
    const url = wmsGetFeatureInfoUrl({
      layer: "cities",
      srid: 4326,
      bbox: [0, 0, 10, 10],
      width: 100,
      height: 100,
      i: 5,
      j: 6,
      swapAxes: true,
    });
    expect(url).toContain("REQUEST=GetFeatureInfo");
    expect(url).toContain("I=5");
    expect(url).toContain("J=6");
  });
});
