"use client";

import * as React from "react";
import { Loader2, X, Minimize2 } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { apiPost } from "@/lib/api";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type JobStatus =
  | "queued"
  | "running"
  | "completed"
  | "failed"
  | "cancelled";

export interface JobState {
  status: JobStatus;
  progress: number;
  currentStep?: string;
  startedAt?: string;
  estimatedCompletionAt?: string;
  result?: unknown;
  error?: unknown;
  canCancel?: boolean;
}

export interface JobProgressPanelProps {
  jobId: string;
  title: string;
  onComplete?: (result: unknown) => void;
  onFail?: (error: unknown) => void;
  showCancel?: boolean;
  showBackground?: boolean;
  variant?: "inline" | "panel";
  className?: string;
}

// ---------------------------------------------------------------------------
// SSE hook with polling fallback
// ---------------------------------------------------------------------------

function useJobProgress(
  jobId: string,
  onComplete?: (result: unknown) => void,
  onFail?: (error: unknown) => void,
): JobState {
  const [state, setState] = React.useState<JobState>({
    status: "queued",
    progress: 0,
  });

  React.useEffect(() => {
    if (!jobId) return;

    let es: EventSource | null = null;
    let pollInterval: ReturnType<typeof setInterval> | null = null;
    let cancelled = false;

    const poll = async () => {
      try {
        const res = await fetch(`/api/v1/jobs/${jobId}`, {
          headers: {
            Authorization: `Bearer ${typeof window !== "undefined" ? localStorage.getItem("blips_token") ?? "" : ""}`,
          },
        });
        if (!res.ok) return;
        const json = (await res.json()) as { data: JobState };
        const s = json.data;
        if (cancelled) return;
        setState(s);
        if (s.status === "completed") {
          if (pollInterval) clearInterval(pollInterval);
          onComplete?.(s.result);
        }
        if (s.status === "failed" || s.status === "cancelled") {
          if (pollInterval) clearInterval(pollInterval);
          onFail?.(s.error);
        }
      } catch {
        // Silently ignore poll errors
      }
    };

    const startSSE = () => {
      const token =
        typeof window !== "undefined"
          ? (localStorage.getItem("blips_token") ?? "")
          : "";
      es = new EventSource(`/api/v1/jobs/${jobId}/stream?token=${encodeURIComponent(token)}`);

      es.addEventListener("progress", (e) => {
        if (cancelled) return;
        try {
          setState((prev) => ({ ...prev, ...(JSON.parse(e.data) as Partial<JobState>) }));
        } catch {
          /* ignore */
        }
      });

      es.addEventListener("completed", (e) => {
        if (cancelled) return;
        try {
          const data = JSON.parse(e.data) as Partial<JobState>;
          setState({ status: "completed", progress: 100, ...data });
          onComplete?.(data.result);
        } catch {
          /* ignore */
        }
        es?.close();
      });

      es.addEventListener("failed", (e) => {
        if (cancelled) return;
        try {
          const data = JSON.parse(e.data) as Partial<JobState>;
          setState({ status: "failed", progress: 0, ...data });
          onFail?.(data.error);
        } catch {
          /* ignore */
        }
        es?.close();
      });

      es.onerror = () => {
        es?.close();
        es = null;
        // Fallback to polling
        if (!cancelled) {
          pollInterval = setInterval(() => void poll(), 2000);
        }
      };
    };

    startSSE();

    return () => {
      cancelled = true;
      es?.close();
      if (pollInterval) clearInterval(pollInterval);
    };
  }, [jobId, onComplete, onFail]);

  return state;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function JobProgressPanel({
  jobId,
  title,
  onComplete,
  onFail,
  showCancel = false,
  showBackground = true,
  variant = "panel",
  className,
}: JobProgressPanelProps) {
  const [hidden, setHidden] = React.useState(false);
  const [cancelling, setCancelling] = React.useState(false);

  const state = useJobProgress(jobId, onComplete, onFail);
  const { status, progress, currentStep, estimatedCompletionAt, canCancel } = state;

  const handleCancel = async () => {
    setCancelling(true);
    try {
      await apiPost(`/api/v1/jobs/${jobId}/cancel`, {});
    } finally {
      setCancelling(false);
    }
  };

  if (
    hidden ||
    status === "completed" ||
    status === "failed" ||
    status === "cancelled"
  ) {
    return null;
  }

  const isInline = variant === "inline";

  return (
    <Card
      className={cn(
        isInline ? "border-0 shadow-none bg-muted/30" : "shadow-md",
        className,
      )}
      role="region"
      aria-label={`Kemajuan: ${title}`}
    >
      <CardContent className={cn("pt-4 pb-3", isInline && "px-3 py-2")}>
        <div className="flex items-start justify-between gap-2">
          <div className="flex items-center gap-2 min-w-0">
            <Loader2
              className="h-4 w-4 animate-spin text-primary flex-shrink-0"
              aria-hidden="true"
            />
            <span className="text-sm font-medium truncate">{title}</span>
          </div>
          <div className="flex items-center gap-1 flex-shrink-0">
            {showBackground && (
              <Button
                variant="ghost"
                size="sm"
                className="h-6 px-2 text-xs"
                onClick={() => setHidden(true)}
                aria-label="Pindah ke background"
              >
                <Minimize2 className="h-3 w-3" aria-hidden="true" />
                {!isInline && <span className="ml-1">Background</span>}
              </Button>
            )}
            {(showCancel || canCancel) && (
              <Button
                variant="ghost"
                size="sm"
                className="h-6 px-2 text-xs text-destructive"
                onClick={() => void handleCancel()}
                disabled={cancelling}
                aria-label="Batalkan proses"
              >
                <X className="h-3 w-3" aria-hidden="true" />
                {!isInline && <span className="ml-1">Batalkan</span>}
              </Button>
            )}
          </div>
        </div>

        {/* Progress bar */}
        <div className="mt-2 space-y-1">
          <progress
            value={progress}
            max={100}
            className="w-full h-1.5 rounded-full overflow-hidden"
            aria-label={`Kemajuan: ${progress}%`}
          />
          <div className="flex justify-between text-xs text-muted-foreground">
            <span
              aria-live="polite"
              aria-atomic="true"
              className="truncate max-w-[70%]"
            >
              {currentStep ?? (status === "queued" ? "Menunggu..." : "Memproses...")}
            </span>
            <span>{progress}%</span>
          </div>
          {estimatedCompletionAt && (
            <p className="text-xs text-muted-foreground">
              ETA: {estimatedCompletionAt}
            </p>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
