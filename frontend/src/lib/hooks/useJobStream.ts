"use client";

/**
 * P5-M15 — SSE hook for per-job progress stream.
 * Reuses the pattern from JobProgressPanel (M13) but extracted as a
 * standalone hook so dashboard widgets can subscribe without rendering
 * the full panel.
 *
 * SSE connect to /api/v1/jobs/{jobId}/stream?token={jwt}.
 * Falls back to 2-second polling if SSE errors (SSE_STREAM_UNAVAILABLE).
 */

import * as React from "react";
import { API_BASE_URL } from "@/lib/api";
import type { JobStatus } from "@/lib/schemas/jobs.schema";

const POLL_INTERVAL_MS = 2_000;
const JOBS_BASE = "/api/v1/jobs";

export interface StreamJobState {
  status: JobStatus;
  progress: number;
  currentStep?: string;
  startedAt?: string;
  estimatedCompletionAt?: string;
  result?: unknown;
  error?: unknown;
  canCancel?: boolean;
}

export interface UseJobStreamResult {
  state: StreamJobState;
  isConnectedViaSSE: boolean;
}

function getToken(): string {
  if (typeof window === "undefined") return "";
  return localStorage.getItem("blips_token") ?? "";
}

/**
 * Subscribe to SSE for a job. Falls back to polling if SSE unavailable.
 * Returns null state while loading (status='queued', progress=0).
 *
 * @param jobId - the job to subscribe to; pass null/'' to skip
 * @param onComplete - callback when SSE 'completed' received
 * @param onFail - callback when SSE 'failed' received
 */
export function useJobStream(
  jobId: string | null | undefined,
  onComplete?: (result: unknown) => void,
  onFail?: (error: unknown) => void,
): UseJobStreamResult {
  const [state, setState] = React.useState<StreamJobState>({
    status: "queued",
    progress: 0,
  });
  const [isConnectedViaSSE, setIsConnectedViaSSE] = React.useState(false);

  const onCompleteRef = React.useRef(onComplete);
  const onFailRef = React.useRef(onFail);
  onCompleteRef.current = onComplete;
  onFailRef.current = onFail;

  React.useEffect(() => {
    if (!jobId) return;

    let es: EventSource | null = null;
    let pollInterval: ReturnType<typeof setInterval> | null = null;
    let cancelled = false;

    const poll = async () => {
      try {
        const token = getToken();
        const headers: Record<string, string> = { Accept: "application/json" };
        if (token) headers["Authorization"] = `Bearer ${token}`;
        const res = await fetch(`${API_BASE_URL}${JOBS_BASE}/${jobId}`, { headers });
        if (!res.ok || cancelled) return;
        const json = (await res.json()) as { data: StreamJobState };
        const s = json.data;
        setState(s);
        if (s.status === "completed") {
          if (pollInterval) clearInterval(pollInterval);
          onCompleteRef.current?.(s.result);
        }
        if (s.status === "failed" || s.status === "cancelled") {
          if (pollInterval) clearInterval(pollInterval);
          onFailRef.current?.(s.error);
        }
      } catch {
        /* ignore poll errors */
      }
    };

    const startSSE = () => {
      const token = getToken();
      es = new EventSource(
        `${API_BASE_URL}${JOBS_BASE}/${jobId}/stream?token=${encodeURIComponent(token)}`,
      );

      es.addEventListener("progress", (e) => {
        if (cancelled) return;
        try {
          setState((prev) => ({
            ...prev,
            ...(JSON.parse(e.data) as Partial<StreamJobState>),
          }));
        } catch { /* ignore */ }
      });

      es.addEventListener("completed", (e) => {
        if (cancelled) return;
        try {
          const data = JSON.parse(e.data) as Partial<StreamJobState>;
          const next: StreamJobState = { status: "completed", progress: 100, ...data };
          setState(next);
          onCompleteRef.current?.(data.result);
        } catch { /* ignore */ }
        es?.close();
        setIsConnectedViaSSE(false);
      });

      es.addEventListener("failed", (e) => {
        if (cancelled) return;
        try {
          const data = JSON.parse(e.data) as Partial<StreamJobState>;
          setState({ status: "failed", progress: 0, ...data });
          onFailRef.current?.(data.error);
        } catch { /* ignore */ }
        es?.close();
        setIsConnectedViaSSE(false);
      });

      es.addEventListener("close", () => {
        es?.close();
        setIsConnectedViaSSE(false);
      });

      es.onerror = () => {
        es?.close();
        es = null;
        setIsConnectedViaSSE(false);
        if (!cancelled) {
          pollInterval = setInterval(() => void poll(), POLL_INTERVAL_MS);
        }
      };

      es.onopen = () => {
        setIsConnectedViaSSE(true);
      };
    };

    startSSE();

    return () => {
      cancelled = true;
      es?.close();
      if (pollInterval) clearInterval(pollInterval);
      setIsConnectedViaSSE(false);
    };
  }, [jobId]);

  return { state, isConnectedViaSSE };
}
