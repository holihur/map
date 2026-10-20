import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { CRSInput } from "@/components/CRSInput";

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

const crsList = {
  crs: [
    { srid: 4326, authName: "EPSG", authSrid: 4326, name: "WGS 84" },
    { srid: 3857, authName: "EPSG", authSrid: 3857, name: "WGS 84 / Pseudo-Mercator" },
    { srid: 4490, authName: "EPSG", authSrid: 4490, name: "China Geodetic Coordinate System 2000" },
  ],
};

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("CRSInput", () => {
  it("加载并展示坐标系候选列表", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => jsonResponse(crsList)));

    const { container } = render(<CRSInput value={4326} onChange={() => {}} />);

    await waitFor(() => {
      expect(container.querySelectorAll("datalist option")).toHaveLength(3);
    });
    // 说明文字应展示当前 SRID 的名称
    expect(screen.getAllByText(/WGS 84/).length).toBeGreaterThan(0);
  });

  it("输入有效 SRID 时回调 onChange", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => jsonResponse(crsList)));
    const onChange = vi.fn();

    const { container } = render(<CRSInput value={4326} onChange={onChange} />);
    const input = container.querySelector("input") as HTMLInputElement;
    expect(input).toBeTruthy();

    await userEvent.clear(input);
    await userEvent.type(input, "3857");

    expect(onChange).toHaveBeenCalled();
    expect(onChange).toHaveBeenLastCalledWith(3857);
  });

  it("加载失败时不影响手动输入", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => Promise.reject(new Error("network"))));
    const onChange = vi.fn();

    const { container } = render(<CRSInput value={4326} onChange={onChange} />);
    const input = container.querySelector("input") as HTMLInputElement;

    await userEvent.clear(input);
    await userEvent.type(input, "4490");
    expect(onChange).toHaveBeenLastCalledWith(4490);
  });
});
