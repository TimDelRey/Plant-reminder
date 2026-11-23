import { Navigate, NavLink, Outlet, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import { api, unwrap } from "./api/client";
import { useAuth } from "./auth";
import { Brand } from "./components/ui";
import Login from "./pages/Login";
import Register from "./pages/Register";
import { ForgotPassword, ResetPassword } from "./pages/Reset";
import Dashboard from "./pages/Dashboard";
import Channels from "./pages/Channels";
import Account from "./pages/Account";

function Protected() {
  const { user, loading, setUser } = useAuth();
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const plantsActive = pathname === "/" || pathname.startsWith("/plants/");
  if (loading) return <div className="empty">Загрузка…</div>;
  if (!user) return <Navigate to="/login" replace />;

  async function logout() {
    try { await unwrap(api.POST("/api/auth/logout")); } catch { /* всё равно выходим */ }
    setUser(null);
    navigate("/login");
  }

  return (
    <div className="app">
      <header className="topbar">
        <NavLink to="/" className="brand" aria-label="На главную"><Brand /></NavLink>
        <nav className="tabs" aria-label="Разделы">
          <NavLink to="/" className={() => `tab${plantsActive ? " active" : ""}`} data-testid="tab-plants">Растения</NavLink>
          <NavLink to="/channels" className={({ isActive }) => `tab${isActive ? " active" : ""}`} data-testid="tab-channels">Каналы уведомлений</NavLink>
        </nav>
        <div className="top-right">
          <NavLink to="/account" className="link-btn" data-testid="nav-account">Аккаунт</NavLink>
          <span className="hint" data-testid="user-email">{user.email}</span>
          <button className="link-btn" onClick={logout} data-testid="logout-btn">Выйти</button>
        </div>
      </header>
      <Outlet />
    </div>
  );
}

function PublicOnly() {
  const { user, loading } = useAuth();
  if (loading) return <div className="empty">Загрузка…</div>;
  if (user) return <Navigate to="/" replace />;
  return <Outlet />;
}

export default function App() {
  return (
    <Routes>
      <Route element={<PublicOnly />}>
        <Route path="/login" element={<Login />} />
        <Route path="/register" element={<Register />} />
      </Route>
      <Route path="/forgot-password" element={<ForgotPassword />} />
      <Route path="/reset-password" element={<ResetPassword />} />
      <Route element={<Protected />}>
        <Route path="/" element={<Dashboard />} />
        <Route path="/plants/:id" element={<Dashboard />} />
        <Route path="/channels" element={<Channels />} />
        <Route path="/account" element={<Account />} />
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
