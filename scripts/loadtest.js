// k6 负载测试：k6 run scripts/loadtest.js
// 环境变量：BASE_URL（默认 http://localhost:8420）、TOKEN（可选，读写接口需要）、LAYER（默认 cities）
import http from "k6/http";
import { check, sleep } from "k6";

export const options = {
  scenarios: {
    tiles: {
      executor: "constant-vus",
      vus: Number(__ENV.VUS || 20),
      duration: __ENV.DURATION || "30s",
      exec: "tiles",
    },
    features: {
      executor: "constant-vus",
      vus: Number(__ENV.VUS || 10),
      duration: __ENV.DURATION || "30s",
      exec: "features",
      startTime: "2s",
    },
  },
  thresholds: {
    http_req_failed: ["rate<0.01"],
    http_req_duration: ["p(95)<800"],
  },
};

const BASE = __ENV.BASE_URL || "http://localhost:8420";
const LAYER = __ENV.LAYER || "cities";
const headers = __ENV.TOKEN ? { Authorization: `Bearer ${__ENV.TOKEN}` } : {};

export function tiles() {
  const z = 2 + Math.floor(Math.random() * 5);
  const n = 2 ** z;
  const x = Math.floor(Math.random() * n);
  const y = Math.floor(Math.random() * n);
  const res = http.get(`${BASE}/api/tiles/${LAYER}/${z}/${x}/${y}.mvt`, { headers });
  check(res, { "tile ok": (r) => r.status === 200 || r.status === 204 });
}

export function features() {
  const res = http.get(`${BASE}/api/layers/${LAYER}/features?limit=50`, { headers });
  check(res, { "features ok": (r) => r.status === 200 });

  const ogc = http.get(`${BASE}/ogcapi/collections/${LAYER}/items?limit=20`, { headers });
  check(ogc, { "ogc ok": (r) => r.status === 200 });
  sleep(0.2);
}
