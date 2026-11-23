import { useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { api, unwrap, type City } from "../api/client";
import { useAuth } from "../auth";
import CityPicker from "../components/CityPicker";
import { Brand, Field, FormError, fieldErr } from "../components/ui";

export default function Register() {
  const { setUser } = useAuth();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [city, setCity] = useState<City | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [cityErr, setCityErr] = useState<string>();
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setError(null); setCityErr(undefined);
    if (!city) { setCityErr("Выберите город из списка подсказок"); return; }
    setBusy(true);
    try {
      setUser(await unwrap(api.POST("/api/auth/register", { body: { email, password, city_id: city.id } })));
    } catch (err) { setError(err); } finally { setBusy(false); }
  }

  return (
    <div className="auth-wrap">
      <Brand className="auth-brand" />
      <div className="panel auth-card">
        <h1>Регистрация</h1>
        <form onSubmit={submit} data-testid="register-form" noValidate>
          <Field id="reg-email" label="Email" error={fieldErr(error, "email")}>
            <input id="reg-email" name="email" type="email" autoComplete="email" data-testid="register-email" value={email} onChange={(e) => setEmail(e.target.value)} required />
          </Field>
          <Field id="reg-password" label="Пароль (не короче 8 символов)" error={fieldErr(error, "password")}>
            <input id="reg-password" name="password" type="password" autoComplete="new-password" data-testid="register-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
          </Field>
          <CityPicker id="reg-city" value={city} onChange={setCity} error={cityErr ?? fieldErr(error, "city_id")} testid="register-city" />
          <p className="hint" style={{ margin: 0 }}>Город нужен, чтобы присылать напоминания утром по вашему местному времени.</p>
          <FormError error={error} />
          <button className="btn primary" type="submit" disabled={busy} data-testid="register-submit">Создать аккаунт</button>
        </form>
        <div className="auth-links"><Link to="/login" data-testid="link-login">Уже есть аккаунт? Войти</Link></div>
      </div>
    </div>
  );
}
