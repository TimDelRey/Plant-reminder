import { useEffect, useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { api, unwrap, errorMessage, type ApiToken, type City } from "../api/client";
import { useAuth } from "../auth";
import CityPicker from "../components/CityPicker";
import { Field, FormError, fieldErr } from "../components/ui";
import { fmtDateTime } from "../lib";

export default function Account() {
  return (
    <div data-testid="account-page">
      <h1 className="page-title">Аккаунт</h1>
      <CitySection />
      <PasswordSection />
      <TokensSection />
      <DeleteSection />
      <div className="footer-note" data-testid="geonames-attribution">
        Справочник городов основан на данных <a href="https://www.geonames.org/" target="_blank" rel="noreferrer">GeoNames</a>, лицензия{" "}
        <a href="https://creativecommons.org/licenses/by/4.0/" target="_blank" rel="noreferrer">CC BY 4.0</a>.
      </div>
    </div>
  );
}

function CitySection() {
  const { user, setUser } = useAuth();
  const [city, setCity] = useState<City | null>(user?.city ?? null);
  const [error, setError] = useState<unknown>(null);
  const [ok, setOk] = useState(false);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!city) { setError(new Error("Выберите город из списка подсказок")); return; }
    setBusy(true); setError(null); setOk(false);
    try { setUser(await unwrap(api.PATCH("/api/me", { body: { city_id: city.id } }))); setOk(true); }
    catch (err) { setError(err); } finally { setBusy(false); }
  }
  return (
    <section className="panel section" aria-labelledby="h-city">
      <h3 id="h-city">Город и часовой пояс</h3>
      <form onSubmit={submit} className="inline-form" style={{ marginTop: 0 }} data-testid="city-form" noValidate>
        <CityPicker id="acc-city" value={city} onChange={(c) => { setCity(c); setOk(false); }} error={fieldErr(error, "city_id")} testid="account-city" />
        <p className="hint" style={{ margin: 0 }}>Напоминания приходят утром по местному времени этого города. Сейчас: {user?.city.name}, {user?.city.timezone}.</p>
        <FormError error={error} />
        {ok && <div className="form-ok" role="status" data-testid="city-saved">Город сохранён</div>}
        <div className="form-actions"><button className="btn primary" type="submit" disabled={busy} data-testid="city-submit">Сохранить город</button></div>
      </form>
    </section>
  );
}

function PasswordSection() {
  const [cur, setCur] = useState("");
  const [next, setNext] = useState("");
  const [error, setError] = useState<unknown>(null);
  const [ok, setOk] = useState(false);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true); setError(null); setOk(false);
    try {
      await unwrap(api.PATCH("/api/me/password", { body: { current_password: cur, new_password: next } }));
      setOk(true); setCur(""); setNext("");
    } catch (err) { setError(err); } finally { setBusy(false); }
  }
  return (
    <section className="panel section" aria-labelledby="h-pass">
      <h3 id="h-pass">Смена пароля</h3>
      <form onSubmit={submit} className="inline-form" style={{ marginTop: 0 }} data-testid="password-form" noValidate>
        <Field id="pw-current" label="Текущий пароль" error={fieldErr(error, "current_password")}>
          <input id="pw-current" name="current_password" type="password" autoComplete="current-password" data-testid="password-current" value={cur} onChange={(e) => setCur(e.target.value)} />
        </Field>
        <Field id="pw-new" label="Новый пароль (не короче 8 символов)" error={fieldErr(error, "new_password")}>
          <input id="pw-new" name="new_password" type="password" autoComplete="new-password" data-testid="password-new" value={next} onChange={(e) => setNext(e.target.value)} />
        </Field>
        <FormError error={error} />
        {ok && <div className="form-ok" role="status" data-testid="password-saved">Пароль изменён</div>}
        <div className="form-actions"><button className="btn primary" type="submit" disabled={busy} data-testid="password-submit">Сменить пароль</button></div>
      </form>
    </section>
  );
}

function TokensSection() {
  const [tokens, setTokens] = useState<ApiToken[]>([]);
  const [name, setName] = useState("");
  const [days, setDays] = useState("");
  const [created, setCreated] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  const load = () => unwrap(api.GET("/api/tokens")).then(setTokens).catch((e) => setError(e));
  useEffect(() => { void load(); }, []);

  async function create(e: FormEvent) {
    e.preventDefault();
    setBusy(true); setError(null); setCreated(null); setCopied(false);
    try {
      const body = { name: name.trim(), ...(days ? { expires_in_days: Number(days) } : {}) };
      const res = await unwrap(api.POST("/api/tokens", { body }));
      setCreated(res.token); setName(""); setDays("");
      await load();
    } catch (err) { setError(err); } finally { setBusy(false); }
  }
  async function revoke(t: ApiToken) {
    if (!window.confirm(`Отозвать токен «${t.name}»?`)) return;
    try { await unwrap(api.DELETE("/api/tokens/{id}", { params: { path: { id: t.id } } })); await load(); }
    catch (err) { setError(err); }
  }
  async function copy() {
    if (!created) return;
    try { await navigator.clipboard.writeText(created); setCopied(true); } catch { setCopied(false); }
  }

  return (
    <section className="panel section" aria-labelledby="h-tokens">
      <h3 id="h-tokens">API-токены</h3>
      <p className="hint" style={{ marginTop: 0 }}>
        Токен даёт доступ к API от вашего имени: передавайте его в заголовке <code>Authorization: Bearer …</code>. Документация — <a href="/api/docs" target="_blank" rel="noreferrer" data-testid="docs-link">/api/docs</a>.
      </p>
      {created && (
        <div className="link-box" style={{ marginBottom: 12 }} data-testid="token-created">
          <strong>Токен создан. Скопируйте его сейчас — повторно он показан не будет.</strong>
          <div className="token-secret" data-testid="token-value">{created}</div>
          <div className="form-actions" style={{ justifyContent: "flex-start" }}>
            <button className="btn outline" onClick={copy} data-testid="token-copy">{copied ? "✓ Скопировано" : "Копировать"}</button>
            <button className="btn ghost" onClick={() => setCreated(null)} data-testid="token-hide">Скрыть</button>
          </div>
        </div>
      )}
      <form onSubmit={create} className="inline-form" style={{ marginTop: 0 }} data-testid="token-form" noValidate>
        <div className="form-row">
          <Field id="tok-name" label="Название токена" error={fieldErr(error, "name")}>
            <input id="tok-name" name="name" type="text" placeholder="Например: скрипт автоматизации" data-testid="token-name" value={name} onChange={(e) => setName(e.target.value)} />
          </Field>
          <Field id="tok-days" label="Срок действия, дней (пусто — бессрочный)" error={fieldErr(error, "expires_in_days")}>
            <input id="tok-days" name="expires_in_days" type="number" min={1} max={3650} data-testid="token-days" value={days} onChange={(e) => setDays(e.target.value)} />
          </Field>
        </div>
        <FormError error={error} />
        <div className="form-actions"><button className="btn primary" type="submit" disabled={busy} data-testid="token-submit">Создать токен</button></div>
      </form>
      <div data-testid="token-list" style={{ marginTop: 10 }}>
        {tokens.length === 0 && <div className="hint">Токенов пока нет.</div>}
        {tokens.map((t) => (
          <div className="token-row" key={t.id} data-testid="token-row">
            <div>
              <strong>{t.name}</strong>
              <div className="hint">
                создан {fmtDateTime(t.created_at)}
                {t.last_used_at ? ` · использован ${fmtDateTime(t.last_used_at)}` : " · не использовался"}
                {t.expires_at ? ` · истекает ${fmtDateTime(t.expires_at)}` : " · бессрочный"}
              </div>
            </div>
            <button className="btn outline" onClick={() => revoke(t)} data-testid="token-revoke">Отозвать</button>
          </div>
        ))}
      </div>
    </section>
  );
}

function DeleteSection() {
  const { setUser } = useAuth();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const [password, setPassword] = useState("");
  const [sure, setSure] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!sure) { setError(new Error("Подтвердите, что понимаете последствия")); return; }
    setBusy(true); setError(null);
    try {
      await unwrap(api.DELETE("/api/me", { body: { password } }));
      setUser(null); navigate("/login");
    } catch (err) { setError(err); } finally { setBusy(false); }
  }
  return (
    <section className="panel section" aria-labelledby="h-del">
      <h3 id="h-del">Удаление аккаунта</h3>
      <p className="hint" style={{ marginTop: 0 }}>Будут безвозвратно удалены ваши растения, события, подключённые каналы, сессии и токены.</p>
      {!open ? (
        <button className="btn outline" style={{ color: "var(--clay)" }} onClick={() => setOpen(true)} data-testid="delete-account-open">Удалить аккаунт…</button>
      ) : (
        <form onSubmit={submit} className="inline-form" style={{ marginTop: 0 }} data-testid="delete-account-form" noValidate>
          <Field id="del-password" label="Введите пароль для подтверждения" error={fieldErr(error, "password")}>
            <input id="del-password" name="password" type="password" autoComplete="current-password" data-testid="delete-account-password" value={password} onChange={(e) => setPassword(e.target.value)} />
          </Field>
          <div className="checks"><label><input type="checkbox" checked={sure} onChange={(e) => setSure(e.target.checked)} data-testid="delete-account-confirm" /> Я понимаю, что это действие необратимо</label></div>
          <FormError error={error} />
          <div className="form-actions">
            <button type="button" className="btn ghost" onClick={() => { setOpen(false); setError(null); }} data-testid="delete-account-cancel">Отмена</button>
            <button type="submit" className="btn danger" disabled={busy} data-testid="delete-account-submit">Удалить аккаунт навсегда</button>
          </div>
        </form>
      )}
    </section>
  );
}
