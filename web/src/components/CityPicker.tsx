import { useEffect, useRef, useState } from "react";
import { api, unwrap, type City } from "../api/client";

type Props = { id: string; value: City | null; onChange: (c: City | null) => void; error?: string; label?: string; testid?: string };

export default function CityPicker({ id, value, onChange, error, label = "Город", testid = "city-input" }: Props) {
  const [text, setText] = useState(value ? value.name : "");
  const [items, setItems] = useState<City[]>([]);
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const timer = useRef<number | undefined>(undefined);
  const seq = useRef(0);

  useEffect(() => { if (value) setText(value.name); }, [value]);

  function search(q: string) {
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(async () => {
      const my = ++seq.current;
      try {
        const res = await unwrap(api.GET("/api/cities", { params: { query: { q, limit: 12 } } }));
        if (my === seq.current) { setItems(res); setActive(0); }
      } catch {}
    }, 150);
  }

  function pick(c: City) {
    onChange(c);
    setText(c.name);
    setOpen(false);
  }

  return (
    <div className="combo">
      <label htmlFor={id}>{label}</label>
      <input
        id={id} name="city" type="text" autoComplete="off" role="combobox" aria-expanded={open} aria-controls={`${id}-list`}
        aria-autocomplete="list" placeholder="Начните вводить название города" data-testid={testid}
        className={error ? "invalid" : ""} value={text}
        onChange={(e) => { setText(e.target.value); onChange(null); setOpen(true); search(e.target.value); }}
        onFocus={() => { setOpen(true); search(text); }}
        onBlur={() => window.setTimeout(() => setOpen(false), 150)}
        onKeyDown={(e) => {
          if (!open || !items.length) return;
          if (e.key === "ArrowDown") { e.preventDefault(); setActive((a) => Math.min(a + 1, items.length - 1)); }
          else if (e.key === "ArrowUp") { e.preventDefault(); setActive((a) => Math.max(a - 1, 0)); }
          else if (e.key === "Enter") { e.preventDefault(); pick(items[active]); }
          else if (e.key === "Escape") setOpen(false);
        }}
      />
      {open && items.length > 0 && (
        <ul className="combo-list" id={`${id}-list`} role="listbox" data-testid="city-options">
          {items.map((c, i) => (
            <li key={c.id} role="option" aria-selected={i === active} data-testid="city-option" onMouseDown={(e) => { e.preventDefault(); pick(c); }}>
              <span>{c.name}</span><span className="sub">{c.region}</span>
            </li>
          ))}
        </ul>
      )}
      {value && <div className="combo-selected" data-testid="city-selected">✓ {value.name}, {value.region} · {value.timezone}</div>}
      {error && <div className="field-error" role="alert">{error}</div>}
    </div>
  );
}
