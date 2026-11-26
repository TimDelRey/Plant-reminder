import { useCallback, useEffect, useRef, useState } from "react";
import { api, unwrap, errorMessage, type ChannelName, type ChannelStatus } from "../api/client";
import { CHANNELS, fmtDateTime } from "../lib";

const POLL_MS = 2500;
const POLL_MAX_MS = 2 * 60 * 1000;

export default function Channels() {
  const [list, setList] = useState<ChannelStatus[] | null>(null);
  const [error, setError] = useState("");

  const patch = useCallback((s: ChannelStatus) => setList((cur) => (cur ? cur.map((c) => (c.channel === s.channel ? s : c)) : cur)), []);

  useEffect(() => {
    unwrap(api.GET("/api/channels")).then(setList).catch((e) => setError(errorMessage(e)));
  }, []);

  if (error) return <div className="form-error" role="alert">{error}</div>;
  if (!list) return <div className="empty">Загрузка…</div>;

  return (
    <div data-testid="channels-page">
      <div className="banner">
        Выберите, где получать напоминания. Бот не может написать вам первым, поэтому подключение всегда начинаете вы:
        нажмите «Подключить» и отправьте боту выданный код. Напоминания о событиях приходят во все подключённые каналы, отдельно настраивать их в событиях не нужно.
      </div>
      <div className="channels">
        {CHANNELS.map((meta) => {
          const st = list.find((c) => c.channel === meta.id);
          return st ? <ChannelCard key={meta.id} meta={meta} st={st} onChange={patch} /> : null;
        })}
      </div>
    </div>
  );
}

function ChannelCard({ meta, st, onChange }: { meta: (typeof CHANNELS)[number]; st: ChannelStatus; onChange: (s: ChannelStatus) => void }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [timedOut, setTimedOut] = useState(false);
  const [dismissed, setDismissed] = useState(false);
  const pollRef = useRef<number | undefined>(undefined);
  const startedRef = useRef(0);
  const knownRef = useRef(st.recipients.length);
  const channel: ChannelName = meta.id;

  const stopPolling = useCallback(() => { window.clearInterval(pollRef.current); pollRef.current = undefined; }, []);
  const startPolling = useCallback(() => {
    stopPolling();
    startedRef.current = Date.now();
    pollRef.current = window.setInterval(async () => {
      if (Date.now() - startedRef.current > POLL_MAX_MS) { stopPolling(); setTimedOut(true); return; }
      try {
        const s = await unwrap(api.GET("/api/channels/{channel}/status", { params: { path: { channel } } }));
        onChange(s);
        if (!s.link || s.recipients.length > knownRef.current) stopPolling();
      } catch {}
    }, POLL_MS);
  }, [channel, onChange, stopPolling]);

  useEffect(() => { if (st.link && !pollRef.current) startPolling(); }, []);
  useEffect(() => stopPolling, [stopPolling]);
  useEffect(() => { knownRef.current = st.recipients.length; }, [st.recipients.length]);

  async function connect() {
    setBusy(true); setError(""); setTimedOut(false); setDismissed(false);
    try {
      knownRef.current = st.recipients.length;
      const s = await unwrap(api.POST("/api/channels/{channel}/connect", { params: { path: { channel } } }));
      onChange(s); startPolling();
    } catch (e) { setError(errorMessage(e)); } finally { setBusy(false); }
  }
  async function removeRecipient(id: number) {
    setBusy(true); setError("");
    try {
      onChange(await unwrap(api.DELETE("/api/channels/{channel}/recipients/{id}", { params: { path: { channel, id } } })));
    } catch (e) { setError(errorMessage(e)); } finally { setBusy(false); }
  }
  function cancelPending() { stopPolling(); setDismissed(true); }

  const connected = st.recipients.length > 0;
  const showLink = !!st.link && !dismissed;
  const label = connected ? `✓ Подключено: ${st.recipients.length}` : showLink ? "Ожидает подтверждения" : "Не подключено";

  return (
    <div className="panel channel-card" data-testid="channel-card" data-channel={meta.id} data-status={st.status}>
      <div className="channel-head">
        <span className="channel-icon" aria-hidden="true">{meta.icon}</span>
        <span className="channel-title">{meta.name}</span>
        <span className={`badge ${connected ? "connected" : showLink ? "pending" : "disconnected"}`} data-testid="channel-status">{label}</span>
      </div>
      <div className="channel-desc">{meta.desc}</div>

      {!st.configured && !connected && (
        <div className="form-error" data-testid="channel-not-configured">Канал не настроен на сервере: не задан токен бота. Обратитесь к администратору.</div>
      )}

      {connected && (
        <ul className="recipients" data-testid="channel-recipients" style={{ listStyle: "none", padding: 0, margin: "8px 0" }}>
          {st.recipients.map((r, i) => (
            <li key={r.id} data-testid="channel-recipient" style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 8, padding: "4px 0" }}>
              <span>Получатель {i + 1} <span className="hint">· с {fmtDateTime(r.connected_at)}</span></span>
              <button className="btn outline" onClick={() => removeRecipient(r.id)} disabled={busy} data-testid="recipient-remove">Отключить</button>
            </li>
          ))}
        </ul>
      )}

      {showLink && st.link && (
        <div className="link-box" data-testid="channel-link">
          <div className="hint">{st.link.instruction}</div>
          <div>Код подключения: <span className="code" data-testid="channel-code">{st.link.code}</span></div>
          <div className="hint">Действует до {fmtDateTime(st.link.expires_at)}. Код отправляет тот человек, которому нужны напоминания, со своего аккаунта.</div>
          {st.link.deep_link && <a className="btn primary" href={st.link.deep_link} target="_blank" rel="noreferrer" data-testid="channel-deeplink">Открыть бота</a>}
          {!timedOut && <div className="hint" role="status">Ждём подтверждения…</div>}
          {timedOut && <div className="hint" role="status" data-testid="channel-timeout">Подтверждение не пришло. Отправьте код боту и нажмите «Проверить».</div>}
        </div>
      )}

      {error && <div className="form-error" role="alert">{error}</div>}

      <div className="form-actions" style={{ justifyContent: "flex-start" }}>
        {showLink ? (
          <>
            <button className="btn outline" onClick={startPolling} disabled={busy} data-testid="channel-check">Проверить</button>
            <button className="btn ghost" onClick={cancelPending} disabled={busy} data-testid="channel-cancel">Отмена</button>
          </>
        ) : (
          <button className={connected ? "btn outline" : "btn primary"} onClick={connect} disabled={busy || !st.configured} data-testid="channel-connect">
            {connected ? "Добавить получателя" : "Подключить"}
          </button>
        )}
      </div>
    </div>
  );
}
