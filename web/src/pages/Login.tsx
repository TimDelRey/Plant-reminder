import { useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { api, unwrap } from "../api/client";
import { useAuth } from "../auth";
import { Brand, Field, FormError, fieldErr } from "../components/ui";

export default function Login() {
  const { setUser } = useAuth();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true); setError(null);
    try {
      setUser(await unwrap(api.POST("/api/auth/login", { body: { email, password } })));
    } catch (err) { setError(err); } finally { setBusy(false); }
  }

  return (
    <div className="auth-wrap">
      <Brand className="auth-brand" />
      <div className="panel auth-card">
        <h1>Вход</h1>
        <form onSubmit={submit} data-testid="login-form" noValidate>
          <Field id="login-email" label="Email" error={fieldErr(error, "email")}>
            <input id="login-email" name="email" type="email" autoComplete="email" data-testid="login-email" value={email} onChange={(e) => setEmail(e.target.value)} required />
          </Field>
          <Field id="login-password" label="Пароль" error={fieldErr(error, "password")}>
            <input id="login-password" name="password" type="password" autoComplete="current-password" data-testid="login-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
          </Field>
          <FormError error={error} />
          <button className="btn primary" type="submit" disabled={busy} data-testid="login-submit">Войти</button>
        </form>
        <div className="auth-links">
          <Link to="/register" data-testid="link-register">Зарегистрироваться</Link>
          <Link to="/forgot-password" data-testid="link-forgot">Забыли пароль?</Link>
        </div>
      </div>
    </div>
  );
}
