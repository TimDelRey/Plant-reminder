import type { CareEvent, EventCategory, ChannelName } from "./api/client";

export const MONTHS = ["янв", "фев", "мар", "апр", "май", "июн", "июл", "авг", "сен", "окт", "ноя", "дек"];
export const MONTH_NAMES_FULL = ["Январь", "Февраль", "Март", "Апрель", "Май", "Июнь", "Июль", "Август", "Сентябрь", "Октябрь", "Ноябрь", "Декабрь"];

export const CATEGORIES: EventCategory[] = [
  "Полив", "Влагозарядковый полив", "Обрезка санитарная", "Обрезка формирующая", "Подкормка",
  "Обработка от болезней и вредителей", "Побелка стволов", "Защита от грызунов", "Удаление поросли",
  "Прореживание завязей", "Уборка листвы и падалицы", "Перекопка приствольного круга", "Сбор урожая", "Другое",
];

export const CHANNELS: { id: ChannelName; name: string; icon: string; desc: string; field: string }[] = [
  { id: "max", name: "MAX", icon: "💬", desc: "Национальный мессенджер. Пользователь один раз пишет боту, дальше напоминания приходят автоматически.", field: "MAX" },
  { id: "vk", name: "VK", icon: "🔵", desc: "Сообщения через бота сообщества VK. Нужно разрешить сообщения от сообщества и написать ему.", field: "VK" },
  { id: "telegram", name: "Telegram", icon: "✈️", desc: "Одна кнопка «Открыть бота» — и канал подключён.", field: "Telegram" },
];
export const channelMeta = (id: string) => CHANNELS.find((c) => c.id === id);

export const PRESETS = [
  { id: "once", label: "Разово", kind: "manual" as const, rule: null },
  { id: "daily", label: "Каждый день", kind: "rrule" as const, rule: "FREQ=DAILY" },
  { id: "weekly", label: "Раз в неделю", kind: "rrule" as const, rule: "FREQ=WEEKLY" },
  { id: "biweekly", label: "Раз в 2 недели", kind: "rrule" as const, rule: "FREQ=WEEKLY;INTERVAL=2" },
  { id: "monthly", label: "Раз в месяц", kind: "rrule" as const, rule: "FREQ=MONTHLY" },
  { id: "quarterly", label: "Раз в квартал", kind: "rrule" as const, rule: "FREQ=MONTHLY;INTERVAL=3" },
  { id: "yearly", label: "Ежегодно", kind: "rrule" as const, rule: "FREQ=YEARLY" },
  { id: "phenology", label: "По фенофазе, вручную…", kind: "manual" as const, rule: null },
];

export function presetOf(ev: CareEvent): string {
  if (ev.recurrence_kind === "rrule") {
    const p = PRESETS.find((x) => x.rule === ev.recurrence_rule);
    return p ? p.id : "custom";
  }
  return ev.recurrence_note === "Разово" || ev.recurrence_note === "" ? "once" : "phenology";
}

export function parseDate(s: string): Date {
  const [y, m, d] = s.split("-").map(Number);
  return new Date(y, m - 1, d);
}
export function todayStr(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}
export function fmt(s: string): string {
  const d = parseDate(s);
  return `${d.getDate()} ${MONTHS[d.getMonth()]}`;
}
export function daysUntil(s: string): number {
  const t = new Date();
  t.setHours(0, 0, 0, 0);
  return Math.round((parseDate(s).getTime() - t.getTime()) / 86400000);
}
export const relLabel = (d: number) => (d < 0 ? "просрочено" : d === 0 ? "сегодня" : d === 1 ? "завтра" : `через ${d} дн.`);
export const relClass = (d: number) => (d <= 3 ? "soon" : "ok");
export function pluralEvents(n: number): string {
  const n10 = n % 10, n100 = n % 100;
  if (n100 >= 11 && n100 <= 14) return "событий";
  if (n10 === 1) return "событие";
  if (n10 >= 2 && n10 <= 4) return "события";
  return "событий";
}
export function fmtDateTime(s: string): string {
  return new Date(s).toLocaleString("ru-RU", { day: "numeric", month: "long", hour: "2-digit", minute: "2-digit" });
}
