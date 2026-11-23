import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from "react";
import { api, unwrap, type User } from "./api/client";

type Ctx = { user: User | null; loading: boolean; setUser: (u: User | null) => void; refresh: () => Promise<void> };
const AuthCtx = createContext<Ctx>({ user: null, loading: true, setUser: () => {}, refresh: async () => {} });
export const useAuth = () => useContext(AuthCtx);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);

  const refresh = useCallback(async () => {
    try {
      setUser(await unwrap(api.GET("/api/me")));
    } catch {
      setUser(null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void refresh(); }, [refresh]);
  useEffect(() => {
    const h = () => setUser(null);
    window.addEventListener("api:unauthorized", h);
    return () => window.removeEventListener("api:unauthorized", h);
  }, []);

  return <AuthCtx.Provider value={{ user, loading, setUser, refresh }}>{children}</AuthCtx.Provider>;
}
