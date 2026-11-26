import { useCallback, useEffect, useRef, useState, type FormEvent } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import {
  api, unwrap, errorMessage, type CareEvent, type EventCategory,
  type EventInput, type Plant, type PlantDetail, type UpcomingEvent,
} from "../api/client";
import { Field, FormError, fieldErr } from "../components/ui";
import CareSuggest from "../components/CareSuggest";
import {
  CATEGORIES, MONTHS, MONTH_NAMES_FULL, PRESETS, daysUntil, fmt, parseDate, pluralEvents,
  fmtDay, isDoneOnce, presetOf, relClass, relLabel, todayStr,
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
  const [showDone, setShowDone] = useState(false);
  const [cat, setCat] = useState<EventCategory | null>(null);
  const [form, setForm] = useState<{ event: CareEvent | null; reopen?: boolean } | null>(null);
  const [suggestOpen, setSuggestOpen] = useState(false);
  const [actionError, setActionError] = useState("");
  const [notice, setNotice] = useState<{ eventId: number; text: string } | null>(null);

  const load = useCallback(async () => {
    try { setPlant(await unwrap(api.GET("/api/plants/{id}", { params: { path: { id } } }))); setError(""); }
    catch (e) { setError(errorMessage(e)); }
  }, [id]);
  useEffect(() => { void load(); }, [load]);

  const refresh = async () => { await load(); onChanged(); };

  if (error) return <div className="form-error" role="alert" data-testid="plant-error">{error}</div>;
  if (!plant) return <div className="empty">Загрузка…</div>;

  const events = plant.events;
  const todo = events.filter((e) => !isDoneOnce(e));
  const done = events.filter(isDoneOnce).sort((a, b) => (b.completed_at ?? "").localeCompare(a.completed_at ?? ""));
  const nearest = [...todo.filter((e) => e.active)].sort((a, b) => a.next_due_at.localeCompare(b.next_due_at))[0];

  async function remove() {
    if (!window.confirm(`Удалить растение «${plant!.name}» вместе со всеми событиями?`)) return;
    try { await unwrap(api.DELETE("/api/plants/{id}", { params: { path: { id } } })); onDeleted(); }
    catch (e) { setActionError(errorMessage(e)); }
  }
  async function complete(ev: CareEvent) {
    setActionError(""); setNotice(null);
    try {
      const res = await unwrap(api.POST("/api/events/{id}/complete", { params: { path: { id: ev.id } } }));
      setNotice({
        eventId: ev.id,
        text: res.recurrence_kind === "once"
          ? `«${ev.title}» выполнено и перенесено в раздел «Выполнено».`
          : res.active ? `«${ev.title}» выполнено. Следующий раз: ${fmt(res.next_due_at)}.` : `«${ev.title}» выполнено. Повторений больше нет.`,
      });
      await refresh();
    } catch (e) { setActionError(errorMessage(e)); }
  }
  async function undo(ev: CareEvent) {
    setActionError(""); setNotice(null);
    try { await unwrap(api.POST("/api/events/{id}/undo-complete", { params: { path: { id: ev.id } } })); await refresh(); }
    catch (e) { setActionError(errorMessage(e)); }
  }

  const usedCats = CATEGORIES.filter((c) => todo.some((e) => e.category === c));
  const activeCat = cat && usedCats.includes(cat) ? cat : null;
  const visible = [...(activeCat ? todo.filter((e) => e.category === activeCat) : todo)]
    .sort((a, b) => a.next_due_at.localeCompare(b.next_due_at));
  const closeForm = () => setForm(null);
  const saved = async () => { setForm(null); setNotice(null); await refresh(); };

  return (
    <div>
      <button className="back-link" onClick={() => navigate("/")} data-testid="back-btn">← к списку растений</button>
      <div className="detail-head">
        <h2 data-testid="plant-title">{plant.emoji} {plant.name}</h2>
        <button className="icon-btn danger" onClick={remove} data-testid="delete-plant">Удалить растение</button>
      </div>
      {actionError && <div className="form-error" role="alert" style={{ marginBottom: 10 }}>{actionError}</div>}
      {notice && (
        <div className="notice" role="status" data-testid="complete-notice">
          <span>✓ {notice.text}</span>
          <span className="notice-actions">
            {events.find((e) => e.id === notice.eventId)?.can_undo && (
              <button type="button" className="inline-link" onClick={() => { const ev = events.find((e) => e.id === notice.eventId); if (ev) void undo(ev); }} data-testid="notice-undo">Отменить</button>
            )}
            <button type="button" className="inline-link" onClick={() => setNotice(null)} aria-label="Скрыть сообщение">×</button>
          </span>
        </div>
      )}

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
          {form && <EventForm plantId={id} event={form.event} onClose={closeForm} onSaved={saved} />}
        </div>
      ) : (
        <>
          <div data-testid="brief">
            <YearStrip events={todo.filter((e) => e.active)} />
            <div className="brief-summary">
              <span className="brief-count" data-testid="brief-count">{todo.length} {pluralEvents(todo.length)} в плане{done.length > 0 ? ` · выполнено: ${done.length}` : ""}</span>
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
                    Все<span className="cat-tab-count">{todo.length}</span>
                  </button>
                  {usedCats.map((c) => (
                    <button key={c} role="tab" aria-selected={activeCat === c} className={`cat-tab${activeCat === c ? " active" : ""}`} onClick={() => setCat(c)} data-testid="cat-tab" data-cat={c}>
                      {c}<span className="cat-tab-count">{todo.filter((e) => e.category === c).length}</span>
                    </button>
                  ))}
                </div>
              )}
              <div data-testid="events-list">
                {visible.length === 0 && <div className="empty">{todo.length === 0 ? "Все события выполнены." : "Событий этой категории нет."}</div>}
                {visible.map((ev) => form?.event?.id === ev.id ? (
                  <EventForm key={ev.id} plantId={id} event={ev} onClose={closeForm} onSaved={saved} />
                ) : (
                  <EventRow key={ev.id} ev={ev} onComplete={() => complete(ev)} onEdit={() => setForm({ event: ev })} onUndo={() => undo(ev)} />
                ))}
              </div>
              <button type="button" className="add-link" style={{ marginTop: 14 }} onClick={() => setForm({ event: null })} data-testid="add-event-btn">+ Добавить событие ухода</button>

              {done.length > 0 && (
                <div className="done-section" data-testid="done-section">
                  <button type="button" className="add-link" onClick={() => setShowDone(!showDone)} data-testid="toggle-done-btn">
                    {showDone ? "Скрыть выполненные ↑" : `Выполнено (${done.length}) ↓`}
                  </button>
                  {showDone && done.map((ev) => form?.event?.id === ev.id ? (
                    <EventForm key={ev.id} plantId={id} event={ev} reopen={form.reopen} onClose={closeForm} onSaved={saved} />
                  ) : (
                    <DoneRow key={ev.id} ev={ev} onUndo={() => undo(ev)} onReschedule={() => setForm({ event: ev, reopen: true })} />
                  ))}
                </div>
              )}
            </div>
          )}
          {form && form.event === null && <EventForm plantId={id} event={null} onClose={closeForm} onSaved={async () => { setForm(null); setCat(null); setShowAll(true); await refresh(); }} />}
        </>
      )}
    </div>
  );
}

function EventRow({ ev, onComplete, onEdit, onUndo }: { ev: CareEvent; onComplete: () => void; onEdit: () => void; onUndo: () => void }) {
  const d = daysUntil(ev.next_due_at);
  const chip = !ev.active ? { cls: "off", text: "не активно" } : { cls: d < 0 ? "late" : relClass(d), text: relLabel(d) };
  return (
    <div className="event-row" data-testid="event-row" data-event-id={ev.id}>
      <div className="event-main">
        <div className="event-title" data-testid="event-title">{ev.title}</div>
        <div className="event-meta">{ev.category} · {ev.recurrence_note || "Разово"}</div>
        {ev.description && <div className="event-desc">{ev.description}</div>}
        {ev.completed_at && ev.can_undo && (
          <div className="event-done-note" data-testid="event-done-note">
            ✓ Выполнено {fmtDay(ev.completed_at)} · <button type="button" className="inline-link" onClick={onUndo} data-testid="event-undo">Отменить</button>
          </div>
        )}
      </div>
      <div className="event-side">
        <div className="event-date">
          <span data-testid="event-date">{fmt(ev.next_due_at)}</span>
          <span className={`when-inline ${chip.cls}`}>{chip.text}</span>
        </div>
        <div className="event-btns">
          <button type="button" className="event-edit-btn done" onClick={onComplete} data-testid="event-complete" title="Отметить выполненным">✓ Выполнено</button>
          <button type="button" className="event-edit-btn" onClick={onEdit} data-testid="event-edit">✎ Изменить</button>
        </div>
      </div>
    </div>
  );
}

function DoneRow({ ev, onUndo, onReschedule }: { ev: CareEvent; onUndo: () => void; onReschedule: () => void }) {
  return (
    <div className="event-row done" data-testid="done-row" data-event-id={ev.id}>
      <div className="event-main">
        <div className="event-title">{ev.title}</div>
        <div className="event-meta">{ev.category} · выполнено {ev.completed_at ? fmtDay(ev.completed_at) : ""} (по плану на {fmt(ev.next_due_at)})</div>
      </div>
      <div className="event-side">
        <div className="event-btns">
          <button type="button" className="event-edit-btn" onClick={onUndo} data-testid="done-undo" title="Вернуть в план на прежнюю дату">↩ Отменить</button>
          <button type="button" className="event-edit-btn" onClick={onReschedule} data-testid="done-reschedule" title="Запланировать это событие ещё раз">↻ Запланировать снова</button>
        </div>
      </div>
    </div>
  );
}

function EventForm({ plantId, event, reopen = false, onClose, onSaved }: { plantId: number; event: CareEvent | null; reopen?: boolean; onClose: () => void; onSaved: () => void }) {
  const editing = event !== null;
  const [title, setTitle] = useState(event?.title ?? "");
  const [next, setNext] = useState(reopen ? todayStr() : event?.next_due_at ?? todayStr());
  const [category, setCategory] = useState<EventCategory>(event?.category ?? "Полив");
  const [description, setDescription] = useState(event?.description ?? "");
  const [preset, setPreset] = useState(event ? presetOf(event) : "once");
  const [note, setNote] = useState(event && presetOf(event) === "phenology" ? event.recurrence_note : "");
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const formRef = useRef<HTMLFormElement>(null);

  useEffect(() => {
    formRef.current?.scrollIntoView({ behavior: "smooth", block: "center" });
    formRef.current?.querySelector<HTMLInputElement>("input")?.focus({ preventScroll: true });
  }, []);

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
    const body: EventInput = { title: title.trim(), category, description: description.trim(), recurrence_kind, recurrence_rule, recurrence_note, next_due_at: next, channels: event?.channels ?? [], active: event?.active ?? true };
    try {
      if (event) await unwrap(api.PATCH("/api/events/{id}", { params: { path: { id: event.id } }, body: reopen ? { ...body, reopen: true } : body }));
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
    <form ref={formRef} className="inline-form" onSubmit={submit} data-testid="event-form" noValidate style={{ textAlign: "left" }}>
      <div style={{ fontWeight: 600, fontSize: 14 }}>{reopen ? "Запланировать снова" : editing ? "Изменить событие" : "Новое событие ухода"}</div>
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
      <FormError error={error} />
      <div className="form-actions">
        {editing && <button type="button" className="btn ghost" style={{ color: "var(--clay)", marginRight: "auto" }} onClick={remove} data-testid="event-delete">Удалить событие</button>}
        <button type="button" className="btn ghost" onClick={onClose} data-testid="event-cancel">Отмена</button>
        <button type="submit" className="btn primary" disabled={busy} data-testid="event-submit">{reopen ? "Запланировать" : editing ? "Сохранить изменения" : "Сохранить событие"}</button>
      </div>
    </form>
  );
}
