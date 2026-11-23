import { useCallback, useEffect, useState, type FormEvent } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import {
  api, unwrap, errorMessage, type CareEvent, type ChannelName, type ChannelStatus, type EventCategory,
  type EventInput, type Plant, type PlantDetail, type UpcomingEvent,
} from "../api/client";
import { Field, FormError, fieldErr } from "../components/ui";
import CareSuggest from "../components/CareSuggest";
import {
  CATEGORIES, CHANNELS, MONTHS, MONTH_NAMES_FULL, PRESETS, channelMeta, daysUntil, fmt, parseDate, pluralEvents,
  presetOf, relClass, relLabel, todayStr,
} from "../lib";

function useIsDesktop() {
  const q = "(min-width: 761px)";
  const [v, setV] = useState(() => window.matchMedia(q).matches);
  useEffect(() => {
    const m = window.matchMedia(q);
    const h = () => setV(m.matches);
    m.addEventListener("change", h);
    return () => m.removeEventListener("change", h);
  }, []);
  return v;
}

export default function Dashboard() {
  const params = useParams();
  const navigate = useNavigate();
  const desktop = useIsDesktop();
  const [plants, setPlants] = useState<Plant[] | null>(null);
  const [upcoming, setUpcoming] = useState<UpcomingEvent[]>([]);
  const [loadError, setLoadError] = useState("");
  const [showPlantForm, setShowPlantForm] = useState(false);
  const [version, setVersion] = useState(0);
  const reload = useCallback(() => setVersion((v) => v + 1), []);

  useEffect(() => {
    let alive = true;
    (async () => {
      try {
        const [pl, up] = await Promise.all([
          unwrap(api.GET("/api/plants")),
          unwrap(api.GET("/api/events/upcoming", { params: { query: { limit: 6 } } })),
        ]);
        if (alive) { setPlants(pl); setUpcoming(up); setLoadError(""); }
      } catch (e) { if (alive) setLoadError(errorMessage(e)); }
    })();
    return () => { alive = false; };
  }, [version]);

  const paramId = params.id ? Number(params.id) : null;
  const selectedId = paramId ?? (desktop && plants && plants.length ? plants[0].id : null);

  if (loadError) return <div className="form-error" role="alert">{loadError}</div>;
  if (!plants) return <div className="empty">Загрузка…</div>;

  return (
    <div data-testid="dashboard">
      <div className="upcoming" data-testid="upcoming" aria-label="Ближайшие дела">
        {upcoming.length === 0 && <div className="banner" style={{ margin: 0, width: "100%" }}>Ближайших дел пока нет — добавьте растение и события ухода.</div>}
        {upcoming.map((u) => {
          const d = daysUntil(u.event.next_due_at);
          return (
            <Link to={`/plants/${u.plant.id}`} key={u.event.id} className="panel upcoming-card" data-testid="upcoming-card" style={{ textDecoration: "none", color: "inherit" }}>
              <span className={`when ${d < 0 ? "late" : relClass(d)}`}>{relLabel(d)} · {fmt(u.event.next_due_at)}</span>
              <div className="ev-title">{u.event.title}</div>
              <div className="ev-plant">{u.plant.emoji} {u.plant.name}</div>
            </Link>
          );
        })}
      </div>

      <div className={`plants-layout${paramId !== null || selectedId !== null ? " has-selection" : ""}`}>
        <div className="plants-side">
          <div className="plant-list" data-testid="plant-list">
            {plants.map((p) => (
              <Link key={p.id} to={`/plants/${p.id}`} className={`plant-card${p.id === selectedId ? " selected" : ""}`} data-testid="plant-card" data-plant-id={p.id}>
                <span className="plant-emoji" aria-hidden="true">{p.emoji}</span>
                <span className="plant-meta">
                  <span className="plant-name">{p.name}</span>
                  {p.next_due_at && <span className="plant-next hint">ближайшее событие: {fmt(p.next_due_at)}</span>}
                </span>
              </Link>
            ))}
          </div>
          <button type="button" className="add-link" style={{ marginTop: 10 }} onClick={() => setShowPlantForm(true)} data-testid="add-plant-btn">+ Добавить растение</button>
          {showPlantForm && (
            <PlantForm
              onCancel={() => setShowPlantForm(false)}
              onCreated={(p) => { setShowPlantForm(false); reload(); navigate(`/plants/${p.id}`); }}
            />
          )}
        </div>

        <div className="panel detail" data-testid="plant-detail">
          {selectedId === null
            ? <div className="empty">Выберите растение слева или добавьте новое.</div>
            : <PlantView key={selectedId} id={selectedId} onChanged={reload} onDeleted={() => { reload(); navigate("/"); }} />}
        </div>
      </div>
    </div>
  );
}

function PlantForm({ onCancel, onCreated }: { onCancel: () => void; onCreated: (p: Plant) => void }) {
  const [name, setName] = useState("");
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true); setError(null);
    try { onCreated(await unwrap(api.POST("/api/plants", { body: { name: name.trim() } }))); }
    catch (err) { setError(err); } finally { setBusy(false); }
  }
  return (
    <form className="inline-form" onSubmit={submit} data-testid="plant-form" noValidate>
      <Field id="pl-name" label="Название" error={fieldErr(error, "name")}>
        <input type="text" id="pl-name" name="name" required autoFocus placeholder="Например: Фикус Бенджамина" data-testid="plant-name" value={name} onChange={(e) => setName(e.target.value)} />
      </Field>
      <FormError error={error} />
      <div className="form-actions">
        <button type="button" className="btn ghost" onClick={onCancel} data-testid="plant-cancel">Отмена</button>
        <button type="submit" className="btn primary" disabled={busy} data-testid="plant-submit">Добавить растение</button>
      </div>
    </form>
  );
}

function YearStrip({ events }: { events: CareEvent[] }) {
  const cur = new Date().getMonth();
  const byMonth: CareEvent[][] = Array.from({ length: 12 }, () => []);
  events.forEach((ev) => byMonth[parseDate(ev.next_due_at).getMonth()].push(ev));
  return (
    <div className="year-strip" data-testid="year-strip" role="img" aria-label="События по месяцам">
      {MONTHS.map((label, m) => {
        const n = byMonth[m].length;
        return (
          <div key={m} className={`year-month${m === cur ? " current" : ""}`} title={n ? `${n} ${pluralEvents(n)} в ${MONTH_NAMES_FULL[m].toLowerCase()}` : undefined}>
            <div className="year-dots">{byMonth[m].map((ev) => <span key={ev.id} className="year-dot" />)}</div>
            <div className="year-month-label">{label}</div>
          </div>
        );
      })}
    </div>
  );
}

function PlantView({ id, onChanged, onDeleted }: { id: number; onChanged: () => void; onDeleted: () => void }) {
  const navigate = useNavigate();
  const [plant, setPlant] = useState<PlantDetail | null>(null);
  const [error, setError] = useState("");
  const [showAll, setShowAll] = useState(false);
  const [cat, setCat] = useState<EventCategory | null>(null);
  const [form, setForm] = useState<{ event: CareEvent | null } | null>(null);
  const [suggestOpen, setSuggestOpen] = useState(false);
  const [actionError, setActionError] = useState("");

  const load = useCallback(async () => {
    try { setPlant(await unwrap(api.GET("/api/plants/{id}", { params: { path: { id } } }))); setError(""); }
    catch (e) { setError(errorMessage(e)); }
  }, [id]);
  useEffect(() => { void load(); }, [load]);

  const refresh = async () => { await load(); onChanged(); };

  if (error) return <div className="form-error" role="alert" data-testid="plant-error">{error}</div>;
  if (!plant) return <div className="empty">Загрузка…</div>;

  const events = plant.events;
  const active = events.filter((e) => e.active);
  const sortedActive = [...active].sort((a, b) => a.next_due_at.localeCompare(b.next_due_at));
  const nearest = sortedActive[0];

  async function remove() {
    if (!window.confirm(`Удалить растение «${plant!.name}» вместе со всеми событиями?`)) return;
    try { await unwrap(api.DELETE("/api/plants/{id}", { params: { path: { id } } })); onDeleted(); }
    catch (e) { setActionError(errorMessage(e)); }
  }
  async function complete(ev: CareEvent) {
    setActionError("");
    try { await unwrap(api.POST("/api/events/{id}/complete", { params: { path: { id: ev.id } } })); await refresh(); }
    catch (e) { setActionError(errorMessage(e)); }
  }

  const usedCats = CATEGORIES.filter((c) => events.some((e) => e.category === c));
  const activeCat = cat && usedCats.includes(cat) ? cat : null;
  const visible = [...(activeCat ? events.filter((e) => e.category === activeCat) : events)]
    .sort((a, b) => a.next_due_at.localeCompare(b.next_due_at));

  return (
    <div>
      <button className="back-link" onClick={() => navigate("/")} data-testid="back-btn">← к списку растений</button>
      <div className="detail-head">
        <h2 data-testid="plant-title">{plant.emoji} {plant.name}</h2>
        <button className="icon-btn danger" onClick={remove} data-testid="delete-plant">Удалить растение</button>
      </div>
      {actionError && <div className="form-error" role="alert" style={{ marginBottom: 10 }}>{actionError}</div>}

      {events.length === 0 ? (
        <div className="empty-import" data-testid="empty-import">
          <p>У этого растения пока нет событий ухода.</p>
          {!suggestOpen && (
            <button type="button" className="btn primary" onClick={() => setSuggestOpen(true)} data-testid="import-care-btn">
              ⇣ Подобрать уход в справочнике растений
            </button>
          )}
          {suggestOpen && (
            <CareSuggest plantId={id} plantName={plant.name} onCancel={() => setSuggestOpen(false)}
              onImported={async () => { setSuggestOpen(false); await refresh(); }} />
          )}
          <div style={{ marginTop: 14 }}>
            <button type="button" className="add-link" onClick={() => setForm({ event: null })} data-testid="add-event-btn">+ Добавить событие ухода</button>
          </div>
          {form && <EventForm plantId={id} event={form.event} onClose={() => setForm(null)} onSaved={async () => { setForm(null); await refresh(); }} />}
        </div>
      ) : (
        <>
          <div data-testid="brief">
            <YearStrip events={active} />
            <div className="brief-summary">
              <span className="brief-count" data-testid="brief-count">{events.length} {pluralEvents(events.length)} в году</span>
              {nearest && (() => { const d = daysUntil(nearest.next_due_at); return (
                <span className="brief-next" data-testid="brief-next">
                  <span className={`when-inline ${d < 0 ? "late" : relClass(d)}`}>{relLabel(d)}</span>
                  {nearest.title} · {fmt(nearest.next_due_at)}
                </span>); })()}
            </div>
          </div>
          <button type="button" className="add-link" onClick={() => setShowAll(!showAll)} data-testid="toggle-all-btn">
            {showAll ? "Свернуть ↑" : `Показать все события (${events.length}) ↓`}
          </button>
          {showAll && (
            <div data-testid="events-expanded">
              {usedCats.length > 1 && (
                <div className="cat-tabs" role="tablist" aria-label="Категории событий" style={{ marginTop: 12 }}>
                  <button role="tab" aria-selected={activeCat === null} className={`cat-tab${activeCat === null ? " active" : ""}`} onClick={() => setCat(null)} data-testid="cat-tab-all">
                    Все<span className="cat-tab-count">{events.length}</span>
                  </button>
                  {usedCats.map((c) => (
                    <button key={c} role="tab" aria-selected={activeCat === c} className={`cat-tab${activeCat === c ? " active" : ""}`} onClick={() => setCat(c)} data-testid="cat-tab" data-cat={c}>
                      {c}<span className="cat-tab-count">{events.filter((e) => e.category === c).length}</span>
                    </button>
                  ))}
                </div>
              )}
              <div data-testid="events-list">
                {visible.length === 0 && <div className="empty">Событий этой категории нет.</div>}
                {visible.map((ev) => (
                  <div className="event-row" key={ev.id} data-testid="event-row" data-event-id={ev.id}>
                    <div className="event-main">
                      <div className="event-title" data-testid="event-title">{ev.title}</div>
                      {ev.description && <div className="event-desc">{ev.description}</div>}
                      <div className="event-tags">
                        <span className="tag cat">{ev.category}</span>
                        {ev.channels.map((c) => <span className="tag" key={c}>{channelMeta(c)?.icon} {channelMeta(c)?.name}</span>)}
                        {!ev.active && <span className="tag off">неактивно</span>}
                      </div>
                    </div>
                    <div className="event-side">
                      <div className="event-date"><span data-testid="event-date">{fmt(ev.next_due_at)}</span><span className="rec">{ev.recurrence_note}</span></div>
                      <div className="event-btns">
                        <button type="button" className="event-edit-btn done" onClick={() => complete(ev)} data-testid="event-complete">✓ Отметить выполненным</button>
                        <button type="button" className="event-edit-btn" onClick={() => setForm({ event: ev })} data-testid="event-edit">✎ Изменить</button>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
              <button type="button" className="add-link" style={{ marginTop: 14 }} onClick={() => setForm({ event: null })} data-testid="add-event-btn">+ Добавить событие ухода</button>
            </div>
          )}
          {form && <EventForm plantId={id} event={form.event} onClose={() => setForm(null)} onSaved={async () => { setForm(null); setCat(null); setShowAll(true); await refresh(); }} />}
        </>
      )}
    </div>
  );
}

function EventForm({ plantId, event, onClose, onSaved }: { plantId: number; event: CareEvent | null; onClose: () => void; onSaved: () => void }) {
  const editing = event !== null;
  const [title, setTitle] = useState(event?.title ?? "");
  const [next, setNext] = useState(event?.next_due_at ?? todayStr());
  const [category, setCategory] = useState<EventCategory>(event?.category ?? "Полив");
  const [description, setDescription] = useState(event?.description ?? "");
  const [preset, setPreset] = useState(event ? presetOf(event) : "once");
  const [note, setNote] = useState(event && presetOf(event) === "phenology" ? event.recurrence_note : "");
  const [channels, setChannels] = useState<ChannelName[]>(event?.channels ?? []);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  // для нового события по умолчанию отмечены подключённые каналы
  useEffect(() => {
    if (editing) return;
    unwrap(api.GET("/api/channels")).then((list: ChannelStatus[]) =>
      setChannels(list.filter((c) => c.status === "connected").map((c) => c.channel)),
    ).catch(() => {});
  }, [editing]);

  const toggle = (c: ChannelName) => setChannels((cur) => (cur.includes(c) ? cur.filter((x) => x !== c) : [...cur, c]));

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true); setError(null);
    let recurrence_kind: EventInput["recurrence_kind"] = "manual";
    let recurrence_rule: string | null = null;
    let recurrence_note = "Разово";
    if (preset === "custom" && event) {
      recurrence_kind = event.recurrence_kind; recurrence_rule = event.recurrence_rule ?? null; recurrence_note = event.recurrence_note;
    } else {
      const p = PRESETS.find((x) => x.id === preset)!;
      recurrence_kind = p.kind; recurrence_rule = p.rule;
      recurrence_note = preset === "phenology" ? note.trim() : p.label;
    }
    const body: EventInput = { title: title.trim(), category, description: description.trim(), recurrence_kind, recurrence_rule, recurrence_note, next_due_at: next, channels, active: event?.active ?? true };
    try {
      if (event) await unwrap(api.PATCH("/api/events/{id}", { params: { path: { id: event.id } }, body }));
      else await unwrap(api.POST("/api/plants/{id}/events", { params: { path: { id: plantId } }, body }));
      onSaved();
    } catch (err) { setError(err); } finally { setBusy(false); }
  }

  async function remove() {
    if (!event || !window.confirm("Удалить это событие?")) return;
    try { await unwrap(api.DELETE("/api/events/{id}", { params: { path: { id: event.id } } })); onSaved(); }
    catch (err) { setError(err); }
  }

  return (
    <form className="inline-form" onSubmit={submit} data-testid="event-form" noValidate style={{ textAlign: "left" }}>
      <div style={{ fontWeight: 600, fontSize: 14 }}>{editing ? "Изменить событие" : "Новое событие ухода"}</div>
      <div className="form-row">
        <Field id="ev-title" label="Название" error={fieldErr(error, "title")}>
          <input type="text" id="ev-title" name="title" required placeholder="Например: обработка от мучнистой росы" data-testid="event-title-input" value={title} onChange={(e) => setTitle(e.target.value)} />
        </Field>
        <Field id="ev-next" label="Следующая дата" error={fieldErr(error, "next_due_at")}>
          <input type="date" id="ev-next" name="next_due_at" required data-testid="event-date-input" value={next} onChange={(e) => setNext(e.target.value)} />
        </Field>
      </div>
      <Field id="ev-cat" label="Категория события" error={fieldErr(error, "category")}>
        <select id="ev-cat" name="category" data-testid="event-category" value={category} onChange={(e) => setCategory(e.target.value as EventCategory)}>
          {CATEGORIES.map((c) => <option key={c} value={c}>{c}</option>)}
        </select>
      </Field>
      <Field id="ev-desc" label="Что и как делать" error={fieldErr(error, "description")}>
        <textarea id="ev-desc" name="description" placeholder="Чем обработать / сколько воды / какое удобрение" data-testid="event-description" value={description} onChange={(e) => setDescription(e.target.value)} />
      </Field>
      <Field id="ev-rec" label="Периодичность" error={fieldErr(error, "recurrence_rule") ?? fieldErr(error, "recurrence_kind")}>
        <select id="ev-rec" name="recurrence" data-testid="event-recurrence" value={preset} onChange={(e) => setPreset(e.target.value)}>
          {PRESETS.map((p) => <option key={p.id} value={p.id}>{p.label}</option>)}
          {preset === "custom" && <option value="custom">Своё правило ({event?.recurrence_note})</option>}
        </select>
      </Field>
      {preset === "phenology" && (
        <Field id="ev-note" label="Описание периодичности" error={fieldErr(error, "recurrence_note")}>
          <input type="text" id="ev-note" name="recurrence_note" placeholder="Например: ежегодно, до распускания почек" data-testid="event-note" value={note} onChange={(e) => setNote(e.target.value)} />
        </Field>
      )}
      <div role="group" aria-labelledby="ev-ch-label">
        <label id="ev-ch-label">Каналы напоминания</label>
        <div className="checks">
          {CHANNELS.map((c) => (
            <label key={c.id}>
              <input type="checkbox" name="channels" value={c.id} checked={channels.includes(c.id)} onChange={() => toggle(c.id)} data-testid={`event-channel-${c.id}`} /> {c.icon} {c.name}
            </label>
          ))}
        </div>
        {fieldErr(error, "channels") && <div className="field-error" role="alert">{fieldErr(error, "channels")}</div>}
      </div>
      <FormError error={error} />
      <div className="form-actions">
        {editing && <button type="button" className="btn ghost" style={{ color: "var(--clay)", marginRight: "auto" }} onClick={remove} data-testid="event-delete">Удалить событие</button>}
        <button type="button" className="btn ghost" onClick={onClose} data-testid="event-cancel">Отмена</button>
        <button type="submit" className="btn primary" disabled={busy} data-testid="event-submit">{editing ? "Сохранить изменения" : "Сохранить событие"}</button>
      </div>
    </form>
  );
}
