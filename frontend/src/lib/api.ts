import type { HealthResponse } from "@/types/health";

/**
 * Base URL backend API. Diambil dari env publik Next.js,
 * fallback ke localhost:8081 untuk dev lokal tanpa konfigurasi.
 */
export const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8081";

/**
 * Error terstruktur dari pemanggilan API.
 * `status` 0 berarti kegagalan jaringan (backend tidak terjangkau).
 */
export class ApiError extends Error {
  readonly status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

/**
 * Helper fetch generic. Menggabungkan path relatif dengan base URL,
 * memparsing JSON, dan melempar {@link ApiError} pada respons non-2xx
 * atau kegagalan jaringan.
 *
 * @param path Path relatif terhadap base URL, mis. "/healthz".
 * @param init Opsi fetch standar.
 */
export async function apiFetch<T>(
  path: string,
  init?: RequestInit,
): Promise<T> {
  const url = `${API_BASE_URL}${path}`;

  let response: Response;
  try {
    response = await fetch(url, {
      ...init,
      headers: {
        Accept: "application/json",
        ...init?.headers,
      },
    });
  } catch (cause) {
    const message =
      cause instanceof Error ? cause.message : "Kegagalan jaringan";
    throw new ApiError(
      `Tidak dapat terhubung ke backend (${url}): ${message}`,
      0,
    );
  }

  if (!response.ok) {
    throw new ApiError(
      `Backend mengembalikan status ${response.status} ${response.statusText}`,
      response.status,
    );
  }

  return (await response.json()) as T;
}

/**
 * Ambil status kesehatan backend dari `GET /healthz`.
 */
export function getHealth(): Promise<HealthResponse> {
  return apiFetch<HealthResponse>("/healthz");
}
