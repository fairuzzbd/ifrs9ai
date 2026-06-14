import { v4 as uuidv4 } from "uuid";

export const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8081";

// ---------------------------------------------------------------------------
// Error types
// ---------------------------------------------------------------------------

export interface ApiErrorDetail {
  field: string;
  rule: string;
  message: string;
}

export interface ApiErrorBody {
  code: string;
  message: string;
  details: ApiErrorDetail[];
  traceId: string;
}

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details: ApiErrorDetail[];
  readonly traceId: string;

  constructor(status: number, body: ApiErrorBody) {
    super(body.message);
    this.name = "ApiError";
    this.status = status;
    this.code = body.code;
    this.details = body.details ?? [];
    this.traceId = body.traceId ?? "";
  }
}

export class NetworkError extends Error {
  readonly status = 0;
  readonly code = "NETWORK_ERROR";
  readonly traceId = "";
  readonly details: ApiErrorDetail[] = [];

  constructor(cause: unknown) {
    super(
      cause instanceof Error ? cause.message : "Gagal terhubung ke server",
    );
    this.name = "NetworkError";
  }
}

// ---------------------------------------------------------------------------
// Pagination types
// ---------------------------------------------------------------------------

export interface Pagination {
  nextCursor: string | null;
  hasMore: boolean;
  totalEstimate: number | null;
  limit: number;
}

export interface SortSpec {
  col: string;
  dir: "asc" | "desc";
}

export interface ListResponse<T> {
  data: T[];
  pagination: Pagination;
  appliedSort?: SortSpec[];
  appliedFilter?: Record<string, unknown>;
  meta: { traceId: string };
}

export interface SingleResponse<T> {
  data: T;
  meta: { traceId: string };
}

// ---------------------------------------------------------------------------
// Core fetch wrapper
// ---------------------------------------------------------------------------

async function baseFetch<T>(
  path: string,
  init?: RequestInit & { mutating?: boolean },
): Promise<T> {
  const url = `${API_BASE_URL}${path}`;
  const headers: Record<string, string> = {
    Accept: "application/json",
    ...(init?.headers as Record<string, string>),
  };

  if (init?.mutating && !headers["Idempotency-Key"]) {
    headers["Idempotency-Key"] = uuidv4();
  }

  if (init?.body && !headers["Content-Type"]) {
    headers["Content-Type"] = "application/json";
  }

  const token =
    typeof window !== "undefined" ? localStorage.getItem("blips_token") : null;
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  let response: Response;
  try {
    response = await fetch(url, { ...init, headers });
  } catch (cause) {
    throw new NetworkError(cause);
  }

  if (!response.ok) {
    let body: { error?: ApiErrorBody };
    try {
      body = await response.json();
    } catch {
      body = {
        error: {
          code: "INTERNAL",
          message: `HTTP ${response.status} ${response.statusText}`,
          details: [],
          traceId: "",
        },
      };
    }
    throw new ApiError(
      response.status,
      body.error ?? {
        code: "INTERNAL",
        message: "Terjadi kesalahan server",
        details: [],
        traceId: "",
      },
    );
  }

  // Handle 204 No Content
  if (response.status === 204) {
    return undefined as T;
  }

  return (await response.json()) as T;
}

// ---------------------------------------------------------------------------
// HTTP methods
// ---------------------------------------------------------------------------

export function apiGet<T>(
  path: string,
  init?: Omit<RequestInit, "method">,
): Promise<T> {
  return baseFetch<T>(path, { ...init, method: "GET" });
}

export function apiPost<T>(
  path: string,
  body: unknown,
  idempotencyKey?: string,
): Promise<T> {
  const headers: Record<string, string> = {};
  if (idempotencyKey) {
    headers["Idempotency-Key"] = idempotencyKey;
  }
  return baseFetch<T>(path, {
    method: "POST",
    body: JSON.stringify(body),
    headers,
    mutating: true,
  });
}

export function apiPut<T>(
  path: string,
  body: unknown,
  idempotencyKey?: string,
): Promise<T> {
  const headers: Record<string, string> = {};
  if (idempotencyKey) {
    headers["Idempotency-Key"] = idempotencyKey;
  }
  return baseFetch<T>(path, {
    method: "PUT",
    body: JSON.stringify(body),
    headers,
    mutating: true,
  });
}

export function apiDelete<T>(path: string, idempotencyKey?: string): Promise<T> {
  const headers: Record<string, string> = {};
  if (idempotencyKey) {
    headers["Idempotency-Key"] = idempotencyKey;
  }
  return baseFetch<T>(path, { method: "DELETE", headers, mutating: true });
}

// ---------------------------------------------------------------------------
// Query string builder
// ---------------------------------------------------------------------------

export function buildQueryString(
  params: Record<string, string | number | boolean | null | undefined>,
): string {
  const entries = Object.entries(params).filter(
    ([, v]) => v !== null && v !== undefined && v !== "",
  );
  if (entries.length === 0) return "";
  return (
    "?" +
    entries
      .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(String(v))}`)
      .join("&")
  );
}

// ---------------------------------------------------------------------------
// Type guards
// ---------------------------------------------------------------------------

export function isApiError(err: unknown): err is ApiError {
  return err instanceof ApiError;
}

export function isValidationError(err: unknown): err is ApiError {
  return isApiError(err) && err.code === "VALIDATION_FAILED";
}

// ---------------------------------------------------------------------------
// Legacy / convenience
// ---------------------------------------------------------------------------

import type { HealthResponse } from "@/types/health";

export function getHealth(): Promise<HealthResponse> {
  return apiGet<HealthResponse>("/healthz");
}
