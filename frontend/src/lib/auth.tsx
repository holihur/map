import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";

import {
  fetchMe,
  getAuthToken,
  login as apiLogin,
  logout as apiLogout,
  setAuthToken,
  UNAUTHORIZED_EVENT,
  type AuthUser,
  type Role,
} from "@/lib/api";

const RANK: Record<Role, number> = { admin: 3, editor: 2, viewer: 1 };

interface AuthContextValue {
  user: AuthUser | null;
  loading: boolean;
  login: (username: string, password: string) => Promise<void>;
  logout: () => void;
  /** 当前用户是否至少具备某角色。 */
  can: (role: Role) => boolean;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<AuthUser | null>(null);
  const [loading, setLoading] = useState(true);

  const logout = useCallback(() => {
    apiLogout();
    setUser(null);
  }, []);

  // 启动时尝试用已保存的 token 恢复会话
  useEffect(() => {
    let cancelled = false;
    (async () => {
      if (!getAuthToken()) {
        setLoading(false);
        return;
      }
      try {
        const me = await fetchMe();
        if (!cancelled) setUser(me);
      } catch {
        setAuthToken(null);
        if (!cancelled) setUser(null);
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  // 收到 401 时自动登出
  useEffect(() => {
    const handler = () => logout();
    window.addEventListener(UNAUTHORIZED_EVENT, handler);
    return () => window.removeEventListener(UNAUTHORIZED_EVENT, handler);
  }, [logout]);

  const login = useCallback(async (username: string, password: string) => {
    const { user: u } = await apiLogin(username, password);
    setUser(u);
  }, []);

  const can = useCallback(
    (role: Role) => (user ? RANK[user.role] >= RANK[role] : false),
    [user]
  );

  const value = useMemo(
    () => ({ user, loading, login, logout, can }),
    [user, loading, login, logout, can]
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth 必须在 AuthProvider 内使用");
  return ctx;
}
