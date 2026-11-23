import createClient from "openapi-fetch";
import type { components, paths } from "./schema";

export type Schemas = components["schemas"];
export type City = Schemas["City"];
export type User = Schemas["User"];
export type Plant = Schemas["Plant"];
export type PlantDetail = Schemas["PlantDetail"];
export type CareEvent = Schemas["CareEvent"];
export type EventInput = Schemas["EventInput"];
export type UpcomingEvent = Schemas["UpcomingEvent"];
export type ChannelStatus = Schemas["ChannelStatus"];
export type ChannelName = Schemas["ChannelName"];
export type EventCategory = Schemas["EventCategory"];
export type ApiToken = Schemas["ApiToken"];
export type CareSuggestion = Schemas["CareSuggestion"];
export type SuggestedEvent = Schemas["SuggestedEvent"];
export type ReferenceSearch = Schemas["ReferenceSearch"];
export type Candidate = Schemas["Candidate"];

export const api = createClient<paths>({ baseUrl: "/", credentials: "same-origin" });

export class ApiError extends Error {
  status: number;
  code: string;
  fields: Record<string, string>;
  constructor(status: number, code: string, message: string, fields: Record<string, string> = {}) {
    super(message);
    this.status = status;
    this.code = code;
    this.fields = fields;
  }
}

type Result<T> = { data?: T; error?: unknown; response: Response };

/** Разворачивает ответ openapi-fetch: возвращает data или бросает ApiError в едином формате. */
export async function unwrap<T>(p: Promise<Result<T>>): Promise<T> {
  let res: Result<T>;
  try {
    res = await p;
  } catch {
    throw new ApiError(0, "network", "Нет связи с сервером. Проверьте подключение и попробуйте ещё раз.");
  }
  if (res.response.ok) return res.data as T;
  const body = res.error as { error?: { code?: string; message?: string; fields?: Record<string, string> } } | undefined;
  const e = body?.error;
  const err = new ApiError(
    res.response.status,
    e?.code ?? "error",
    e?.message ?? `Ошибка ${res.response.status}`,
    e?.fields ?? {},
  );
  if (err.status === 401) window.dispatchEvent(new CustomEvent("api:unauthorized"));
  throw err;
}

export function errorMessage(e: unknown): string {
  return e instanceof ApiError || e instanceof Error ? e.message : "Что-то пошло не так";
}
