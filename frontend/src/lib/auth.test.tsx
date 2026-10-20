import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReactNode } from "react";

import { AuthProvider, useAuth } from "@/lib/auth";
import { setAuthToken } from "@/lib/api";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const adminUser = { id: 1, username: "admin", role: "admin", createdAt: "", updatedAt: "" };
const viewerUser = { id: 2, username: "bob", role: "viewer", createdAt: "", updatedAt: "" };

const wrapper = ({ children }: { children: ReactNode }) => <AuthProvider>{children}</AuthProvider>;

beforeEach(() => {
  setAuthToken(null);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("AuthProvider", () => {
  it("无 token 时不请求 /me，且 can 为 false", async () => {
    const fetchMock = vi.fn(async () => jsonResponse(adminUser));
    vi.stubGlobal("fetch", fetchMock);

    const { result } = renderHook(() => useAuth(), { wrapper });
    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(fetchMock).not.toHaveBeenCalled();
    expect(result.current.user).toBeNull();
    expect(result.current.can("viewer")).toBe(false);
  });

  it("有 token 时恢复会话并按角色判断权限", async () => {
    setAuthToken("saved-token");
    const fetchMock = vi.fn(async () => jsonResponse(adminUser));
    vi.stubGlobal("fetch", fetchMock);

    const { result } = renderHook(() => useAuth(), { wrapper });
    await waitFor(() => expect(result.current.user?.username).toBe("admin"));

    expect(result.current.can("admin")).toBe(true);
    expect(result.current.can("editor")).toBe(true);
    expect(result.current.can("viewer")).toBe(true);
  });

  it("viewer 不能执行 editor 操作", async () => {
    setAuthToken("t");
    vi.stubGlobal("fetch", vi.fn(async () => jsonResponse(viewerUser)));

    const { result } = renderHook(() => useAuth(), { wrapper });
    await waitFor(() => expect(result.current.user?.role).toBe("viewer"));

    expect(result.current.can("viewer")).toBe(true);
    expect(result.current.can("editor")).toBe(false);
    expect(result.current.can("admin")).toBe(false);
  });

  it("login 设置用户，logout 清除", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.includes("/api/auth/login")) {
        return jsonResponse({ token: "new-token", user: adminUser });
      }
      return jsonResponse(adminUser);
    });
    vi.stubGlobal("fetch", fetchMock);

    const { result } = renderHook(() => useAuth(), { wrapper });
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.login("admin", "secret");
    });
    expect(result.current.user?.username).toBe("admin");
    expect(localStorage.getItem("mapserver_token")).toBe("new-token");

    act(() => result.current.logout());
    expect(result.current.user).toBeNull();
    expect(localStorage.getItem("mapserver_token")).toBeNull();
  });

  it("token 失效时自动登出", async () => {
    setAuthToken("bad");
    vi.stubGlobal("fetch", vi.fn(async () => jsonResponse({ error: "unauthorized" }, 401)));

    const { result } = renderHook(() => useAuth(), { wrapper });
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.user).toBeNull();
    expect(localStorage.getItem("mapserver_token")).toBeNull();
  });
});
