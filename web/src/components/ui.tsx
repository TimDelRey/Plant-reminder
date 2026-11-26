import type { ReactNode } from "react";
import { ApiError } from "../api/client";

export function Brand({ className = "" }: { className?: string }) {
  return (
    <span className={`brand ${className}`}>
      <svg viewBox="0 0 24 24" fill="none" aria-hidden="true">
        <path d="M12 21c0-6 4-9 8-10-1 6-4 9-8 10Z" fill="var(--leaf)" />
        <path d="M12 21c0-7-4-11-8-12 1 7 4 11 8 12Z" fill="var(--leaf-deep)" />
      </svg>
      Уход за растениями
    </span>
  );
}

export function Field({ id, label, error, children }: { id: string; label: string; error?: string; children: ReactNode }) {
  return (
    <div>
      <label htmlFor={id}>{label}</label>
      {children}
      {error && <div className="field-error" role="alert" data-testid={`error-${id}`}>{error}</div>}
    </div>
  );
}

export function FormError({ error }: { error: unknown }) {
  if (!error) return null;
  const e = error as ApiError;
  if (e instanceof ApiError && Object.keys(e.fields).length > 0 && e.code === "validation_failed") {
    return <div className="form-error" role="alert" data-testid="form-error">{e.message}</div>;
  }
  return <div className="form-error" role="alert" data-testid="form-error">{(error as Error).message ?? "Ошибка"}</div>;
}

export const fieldErr = (error: unknown, name: string): string | undefined =>
  error instanceof ApiError ? error.fields[name] : undefined;
