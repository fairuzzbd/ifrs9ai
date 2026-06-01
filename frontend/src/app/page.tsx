"use client";

import { useCallback, useEffect, useState } from "react";
import { getHealth } from "@/lib/api";
import type { HealthResponse } from "@/types/health";

type LoadState =
  | { status: "loading" }
  | { status: "success"; data: HealthResponse }
  | { status: "error"; message: string };

export default function HomePage() {
  const [state, setState] = useState<LoadState>({ status: "loading" });

  const loadHealth = useCallback(async () => {
    setState({ status: "loading" });
    try {
      const data = await getHealth();
      setState({ status: "success", data });
    } catch (error) {
      const message =
        error instanceof Error
          ? error.message
          : "Terjadi kesalahan tidak diketahui saat menghubungi backend.";
      setState({ status: "error", message });
    }
  }, []);

  useEffect(() => {
    void loadHealth();
  }, [loadHealth]);

  return (
    <main className="flex min-h-screen flex-col items-center justify-center bg-slate-50 px-4 py-10">
      <div className="w-full max-w-md rounded-2xl border border-slate-200 bg-white p-8 shadow-sm">
        <header className="mb-6 text-center">
          <h1 className="text-2xl font-semibold text-slate-900">BLIPS IFRS9</h1>
          <p className="mt-1 text-sm text-slate-500">
            PSAK 71 / IFRS 9 — PT Tugu Reasuransi Indonesia
          </p>
        </header>

        <section
          aria-live="polite"
          aria-busy={state.status === "loading"}
          className="rounded-xl bg-slate-50 p-5"
        >
          <h2 className="mb-4 text-sm font-medium uppercase tracking-wide text-slate-500">
            Status Backend
          </h2>

          {state.status === "loading" && (
            <div className="flex items-center gap-3 text-slate-600">
              <span
                className="h-4 w-4 animate-spin rounded-full border-2 border-slate-300 border-t-slate-600"
                aria-hidden="true"
              />
              <span>Menghubungi backend…</span>
            </div>
          )}

          {state.status === "error" && (
            <div
              role="alert"
              className="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-700"
            >
              <p className="font-semibold">Gagal terhubung ke backend</p>
              <p className="mt-1 break-words">{state.message}</p>
            </div>
          )}

          {state.status === "success" && (
            <dl className="space-y-3 text-sm">
              <div className="flex items-center justify-between">
                <dt className="text-slate-500">Status</dt>
                <dd>
                  <span className="inline-flex items-center gap-1.5 rounded-full bg-green-100 px-2.5 py-0.5 font-medium text-green-700">
                    <span
                      className="h-2 w-2 rounded-full bg-green-500"
                      aria-hidden="true"
                    />
                    {state.data.status}
                  </span>
                </dd>
              </div>
              <div className="flex items-center justify-between">
                <dt className="text-slate-500">Service</dt>
                <dd className="font-medium text-slate-900">
                  {state.data.service}
                </dd>
              </div>
              <div className="flex items-center justify-between">
                <dt className="text-slate-500">Versi</dt>
                <dd className="font-medium text-slate-900">
                  {state.data.version}
                </dd>
              </div>
              <div className="flex items-center justify-between">
                <dt className="text-slate-500">Timestamp</dt>
                <dd className="font-mono text-xs text-slate-700">
                  {state.data.timestamp}
                </dd>
              </div>
            </dl>
          )}
        </section>

        <button
          type="button"
          onClick={() => void loadHealth()}
          disabled={state.status === "loading"}
          className="mt-6 w-full rounded-lg bg-slate-900 px-4 py-2 text-sm font-medium text-white transition hover:bg-slate-800 disabled:cursor-not-allowed disabled:opacity-50"
        >
          Muat ulang status
        </button>
      </div>
    </main>
  );
}
