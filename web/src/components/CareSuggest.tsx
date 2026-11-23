import { useCallback, useEffect, useState } from "react";
import {
  ApiError, api, errorMessage, unwrap, type CareSuggestion, type ReferenceSearch, type SuggestedEvent,
} from "../api/client";
import { channelMeta, fmt } from "../lib";

type Row = { ev: SuggestedEvent; on: boolean };

const QUALITY: Record<string, { label: string; cls: string }> = {
  exact: { label: "точное совпадение", cls: "ok" },
  approximate: { label: "приблизительно", cls: "warn" },
  chosen: { label: "выбрано вручную", cls: "ok" },
};

/**
 * Подбор ухода по справочнику Perenual. Ничего не сохраняется, пока пользователь не подтвердит выбранные события.
 * Явно показывает: что нашлось в справочнике, насколько точно совпало, по каким аспектам ухода есть данные,
 * и какие ещё виды можно взять (например, другие сливы), если нужного сорта нет.
 */
export default function CareSuggest({ plantId, plantName, onCancel, onImported }: {
  plantId: number; plantName: string; onCancel: () => void; onImported: () => void | Promise<void>;
}) {
  const [sug, setSug] = useState<CareSuggestion | null>(null);
  const [rows, setRows] = useState<Row[]>([]);
  const [search, setSearch] = useState<ReferenceSearch | null>(null);
  const [q, setQ] = useState(plantName);
  const [error, setError] = useState("");
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);

  const loadSuggestion = useCallback(async (speciesId?: number) => {
    setLoading(true); setError("");
    try {
      const s = await unwrap(api.GET("/api/plants/{id}/care-suggestions", {
        params: { path: { id: plantId }, query: speciesId ? { species_id: speciesId } : {} },
      }));
      setSug(s); setRows((s.events ?? []).map((ev) => ({ ev, on: true })));
    } catch (e) { setError(errorMessage(e)); } finally { setLoading(false); }
  }, [plantId]);

  const runSearch = useCallback(async (text: string) => {
    if (text.trim().length < 2) return;
    try { setSearch(await unwrap(api.GET("/api/plant-reference/search", { params: { query: { q: text.trim() } } }))); }
    catch (e) { setError(errorMessage(e)); }
  }, []);

  useEffect(() => { void loadSuggestion(); void runSearch(plantName); }, [loadSuggestion, runSearch, plantName]);

  const chosen = rows.filter((r) => r.on);
  const patch = (i: number, p: Partial<SuggestedEvent>) => setRows((rs) => rs.map((r, k) => (k === i ? { ...r, ev: { ...r.ev, ...p } } : r)));

  async function save() {
    setBusy(true); setError(""); setFieldErrors({});
    try {
      const events = chosen.map(({ ev: { source: _s, ...e } }) => e); // source — служебное поле
      await unwrap(api.POST("/api/plants/{id}/events/import", { params: { path: { id: plantId } }, body: { events } }));
      await onImported();
    } catch (e) {
      setError(errorMessage(e));
      if (e instanceof ApiError) setFieldErrors(e.fields);
    } finally { setBusy(false); }
  }

  const m = sug?.match;
  return (
    <div className="suggest" data-testid="care-suggest" style={{ textAlign: "left" }}>
      <form className="suggest-search" onSubmit={(e) => { e.preventDefault(); void runSearch(q); }} data-testid="suggest-search-form">
        <label htmlFor="sg-q" className="hint">Поиск в справочнике (можно по-русски, по-английски или по латыни)</label>
        <div style={{ display: "flex", gap: 8 }}>
          <input id="sg-q" type="text" value={q} onChange={(e) => setQ(e.target.value)} data-testid="suggest-q" placeholder="Слива, plum, Prunus domestica" />
          <button type="submit" className="btn ghost" data-testid="suggest-search-btn" style={{ border: "1px solid var(--border)" }}>Найти</button>
        </div>
      </form>

      {loading && <div className="empty" data-testid="suggest-loading">Запрашиваем справочник для «{plantName}»…</div>}
      {error && <div className="form-error" role="alert" data-testid="suggest-error" style={{ marginTop: 8 }}>{error}</div>}

      {!loading && sug && (
        <>
          {m ? (
            <div className="suggest-match" data-testid="suggest-match">
              {m.image_url && <img src={m.image_url} alt="" width={56} height={56} style={{ borderRadius: 10, objectFit: "cover" }} />}
              <div>
                <strong>{m.common_name}</strong> <span className="hint">· {m.scientific}</span>{" "}
                <span className={`q-badge ${QUALITY[m.quality]?.cls ?? ""}`} data-testid="suggest-quality">{QUALITY[m.quality]?.label ?? m.quality}</span>
                <div className="hint" data-testid="suggest-quality-note">{m.quality_note}</div>
                {(m.sunlight ?? []).length > 0 && <div className="hint">Освещение: {(m.sunlight ?? []).join(", ")}</div>}
              </div>
            </div>
          ) : (
            <div className="banner" data-testid="suggest-nomatch" style={{ margin: "10px 0" }}>
              <strong>В справочнике не подтянулось.</strong> {sug.notice}
            </div>
          )}

          {(sug.coverage ?? []).length > 0 && (
            <ul className="coverage" data-testid="suggest-coverage">
              {(sug.coverage ?? []).map((c) => (
                <li key={c.field} className={c.available ? "yes" : "no"} data-testid="coverage-item" data-field={c.field} data-available={c.available}>
                  <span aria-hidden="true">{c.available ? "✓" : "✗"}</span> <strong>{c.label}:</strong> {c.detail}
                </li>
              ))}
            </ul>
          )}

          {m && (sug.events ?? []).length === 0 && <div className="banner" data-testid="suggest-notice" style={{ margin: "10px 0" }}>{sug.notice}</div>}

          {rows.length > 0 && (
            <div className="suggest-list" data-testid="suggest-list">
              {rows.map((r, i) => {
                const fe = (k: string) => fieldErrors[`events[${i}].${k}`];
                return (
                  <div key={i} className={`suggest-row${r.on ? "" : " off"}`} data-testid="suggest-row">
                    <label className="suggest-check">
                      <input type="checkbox" checked={r.on} onChange={(e) => setRows((rs) => rs.map((x, k) => (k === i ? { ...x, on: e.target.checked } : x)))} data-testid="suggest-check" aria-label={`Добавить: ${r.ev.title}`} />
                      <span><strong>{r.ev.title}</strong><span className="hint"> · {r.ev.recurrence_note}</span><span className="tag cat" style={{ marginLeft: 8 }}>по данным Perenual</span></span>
                    </label>
                    <div className="hint" style={{ margin: "2px 0 6px 26px" }}>{r.ev.description}</div>
                    <div style={{ margin: "0 0 0 26px", display: "flex", gap: 10, alignItems: "center", flexWrap: "wrap" }}>
                      <label className="hint" htmlFor={`sg-date-${i}`}>Первая дата ({fmt(r.ev.next_due_at)})</label>
                      <input type="date" id={`sg-date-${i}`} value={r.ev.next_due_at} disabled={!r.on} data-testid="suggest-date" onChange={(e) => patch(i, { next_due_at: e.target.value })} style={{ width: "auto" }} />
                      <span className="hint">
                        {r.ev.channels.length ? r.ev.channels.map((c) => `${channelMeta(c)?.icon ?? ""} ${channelMeta(c)?.name ?? c}`).join(", ") : "каналы не выбраны — подключите их на вкладке «Каналы»"}
                      </span>
                    </div>
                    {(fe("next_due_at") || fe("title") || fe("recurrence_rule")) && <div className="field-error" role="alert">{fe("next_due_at") ?? fe("title") ?? fe("recurrence_rule")}</div>}
                  </div>
                );
              })}
            </div>
          )}
        </>
      )}

      {search && (
        <div className="candidates" data-testid="suggest-candidates">
          <div className="candidates-head">Что есть в справочнике по запросу «{search.query || q}»</div>
          {search.query_note && <div className="hint" data-testid="suggest-query-note" style={{ marginBottom: 6 }}>{search.query_note}</div>}
          {(search.results ?? []).length === 0 && <div className="hint" data-testid="suggest-no-results">{search.notice || "Ничего не найдено."}</div>}
          {(search.results ?? []).map((c) => (
            <div key={c.id} className={`cand-row${m && m.species_id === c.id ? " current" : ""}`} data-testid="candidate" data-species-id={c.id}>
              {c.image_url ? (
                <a href={c.image_url} target="_blank" rel="noreferrer" title="Открыть фото крупнее" style={{ flex: "none" }}>
                  <img src={c.image_url} alt={c.common_name || c.scientific} width={64} height={64} loading="lazy" referrerPolicy="no-referrer"
                    className="cand-img" data-testid="candidate-image" />
                </a>
              ) : (
                <div className="cand-img cand-noimg" data-testid="candidate-noimage" title="Фото в справочнике нет">🌱</div>
              )}
              <div style={{ minWidth: 0, flex: 1 }}>
                <strong>{c.common_name || c.scientific}</strong> <span className="hint">· {c.scientific}</span>
                <div className="hint">{c.details_available ? "можно запросить данные ухода" : "данные ухода недоступны на текущем тарифе"}</div>
              </div>
              <button type="button" className="btn ghost" disabled={!c.details_available || loading || (!!m && m.species_id === c.id)} onClick={() => void loadSuggestion(c.id)}
                style={{ border: "1px solid var(--border)", whiteSpace: "nowrap" }} data-testid="candidate-pick" title={c.details_available ? "Построить план по этому виду" : "Подробности по виду закрыты тарифом"}>
                {m && m.species_id === c.id ? "выбрано" : "Взять уход"}
              </button>
            </div>
          ))}
        </div>
      )}

      {sug?.attribution && <div className="hint" style={{ marginTop: 8 }} data-testid="suggest-attribution">{sug.attribution}</div>}
      <div className="form-actions" style={{ marginTop: 12 }}>
        <button type="button" className="btn ghost" onClick={onCancel} data-testid="suggest-cancel">Отмена</button>
        <button type="button" className="btn primary" disabled={busy || chosen.length === 0} onClick={save} data-testid="suggest-save">
          Добавить выбранные ({chosen.length})
        </button>
      </div>
    </div>
  );
}
