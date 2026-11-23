import { useState, type FormEvent } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { api, unwrap } from "../api/client";
import { Brand, Field, FormError, fieldErr } from "../components/ui";

export function ForgotPassword() {
  const [email, setEmail] = useState("");
  const [sent, setSent] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true); setError(null);
    try {
      await unwrap(api.POST("/api/auth/password-reset/request", { body: { email } }));
      setSent(true);
    } catch (err) { setError(err); } finally { setBusy(false); }
  }

  return (
    <div className="auth-wrap">
      <Brand className="auth-brand" />
      <div className="panel auth-card">
        <h1>Сброс пароля</h1>
        {sent ? (
          <div className="form-ok" role="status" data-testid="forgot-sent">
            Если аккаунт с таким email существует, мы отправили на него письмо со ссылкой для сброса пароля. Ссылка действует 1 час.
          </div>
        ) : (
          <form onSubmit={submit} data-testid="forgot-form" noValidate>
            <p className="hint" style={{ margin: 0 }}>Укажите email, с которым регистрировались — отправим ссылку для создания нового пароля.</p>
            <Field id="forgot-email" label="Email" error={fieldErr(error, "email")}>
              <input id="forgot-email" name="email" type="email" autoComplete="email" data-testid="forgot-email" value={email} onChange={(e) => setEmail(e.target.value)} required />
            </Field>
            <FormError error={error} />
            <button className="btn primary" type="submit" disabled={busy} data-testid="forgot-submit">Отправить ссылку</button>
          </form>
        )}
        <div className="auth-links"><Link to="/login">← Ко входу</Link></div>
      </div>
    </div>
  );
}

export function ResetPassword() {
  const [params] = useSearchParams();
  const token = params.get("token") ?? "";
  const navigate = useNavigate();
  const [password, setPassword] = useState("");
  const [error, setError] = useState<unknown>(null);
  const [done, setDone] = useState(false);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true); setError(null);
    try {
      await unwrap(api.POST("/api/auth/password-reset/confirm", { body: { token, password } }));
      setDone(true);
      window.setTimeout(() => navigate("/login"), 2500);
    } catch (err) { setError(err); } finally { setBusy(false); }
  }

  return (
    <div className="auth-wrap">
      <Brand className="auth-brand" />
      <div className="panel auth-card">
        <h1>Новый пароль</h1>
        {!token ? (
          <div className="form-error" data-testid="reset-no-token">В ссылке нет токена. Запросите сброс пароля заново.</div>
        ) : done ? (
          <div className="form-ok" role="status" data-testid="reset-done">Пароль изменён. Сейчас перенаправим на страницу входа…</div>
        ) : (
          <form onSubmit={submit} data-testid="reset-form" noValidate>
            <Field id="reset-password" label="Новый пароль (не короче 8 символов)" error={fieldErr(error, "password")}>
              <input id="reset-password" name="password" type="password" autoComplete="new-password" data-testid="reset-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
            </Field>
            <FormError error={error} />
            <button className="btn primary" type="submit" disabled={busy} data-testid="reset-submit">Сохранить пароль</button>
          </form>
        )}
        <div className="auth-links"><Link to="/login">← Ко входу</Link><Link to="/forgot-password">Запросить ссылку снова</Link></div>
      </div>
    </div>
  );
}
