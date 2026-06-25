"use client";

/**
 * P5-M15 — HashChainStatusBadge: KPI card for audit hash-chain verification status.
 */

import * as React from "react";
import Link from "next/link";
import { CheckCircle, AlertTriangle, Loader2, HelpCircle } from "lucide-react";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

export type HashChainStatus = "VERIFIED" | "MISMATCH" | "VERIFYING" | "UNKNOWN";

export interface HashChainStatusBadgeProps {
  status: HashChainStatus;
  lastRunAt?: string;
  jobId?: string;
  mismatchCount?: number;
  loading?: boolean;
  className?: string;
}

export function HashChainStatusBadge({
  status,
  lastRunAt,
  jobId,
  mismatchCount,
  loading = false,
  className,
}: HashChainStatusBadgeProps) {
  if (loading) {
    return <Skeleton className={cn("h-16 w-full", className)} />;
  }

  if (status === "VERIFIED") {
    return (
      <div
        className={cn("flex items-start gap-3 rounded-lg border border-green-200 bg-green-50 p-3", className)}
        role="status"
        aria-live="polite"
        aria-label="Status Hash-chain: Terverifikasi"
      >
        <CheckCircle className="h-5 w-5 text-green-700 flex-shrink-0 mt-0.5" aria-hidden="true" />
        <div className="space-y-0.5">
          <p className="text-sm font-semibold text-green-800">Hash-chain VERIFIED</p>
          {lastRunAt && (
            <p className="text-xs text-green-700">
              Last run: {lastRunAt}
            </p>
          )}
          {jobId && (
            <Link
              href={`/jobs/${jobId}`}
              className="text-xs text-green-700 underline-offset-2 hover:underline"
            >
              Lihat detail →
            </Link>
          )}
        </div>
      </div>
    );
  }

  if (status === "MISMATCH") {
    return (
      <div
        className={cn(
          "flex items-start gap-3 rounded-lg border border-red-300 bg-red-50 p-3",
          className,
        )}
        role="alert"
        aria-live="assertive"
        aria-label="PERINGATAN: Hash-chain MISMATCH terdeteksi"
      >
        <AlertTriangle className="h-5 w-5 text-red-700 flex-shrink-0 mt-0.5" aria-hidden="true" />
        <div className="space-y-1">
          <p className="text-sm font-bold text-red-800">
            PERINGATAN: Hash-chain MISMATCH Terdeteksi!
          </p>
          {lastRunAt && (
            <p className="text-xs text-red-700">Verifikasi terakhir: {lastRunAt}</p>
          )}
          {mismatchCount != null && (
            <p className="text-xs text-red-700">
              {mismatchCount} entry dengan hash tidak valid.
            </p>
          )}
          <p className="text-xs text-red-700 font-medium">
            Tindakan segera diperlukan. Hubungi IT Admin.
          </p>
          <div className="flex gap-2 mt-1">
            {jobId && (
              <Link
                href={`/jobs/${jobId}`}
                className="text-xs text-red-700 underline-offset-2 hover:underline"
              >
                Lihat detail job →
              </Link>
            )}
          </div>
        </div>
      </div>
    );
  }

  if (status === "VERIFYING") {
    return (
      <div
        className={cn(
          "flex items-center gap-3 rounded-lg border border-blue-200 bg-blue-50 p-3",
          className,
        )}
        role="status"
        aria-live="polite"
      >
        <Loader2 className="h-5 w-5 text-blue-700 animate-spin" aria-hidden="true" />
        <p className="text-sm text-blue-800">Verifikasi sedang berjalan...</p>
      </div>
    );
  }

  // UNKNOWN
  return (
    <div
      className={cn(
        "flex items-center gap-3 rounded-lg border border-muted p-3",
        className,
      )}
      role="status"
    >
      <HelpCircle className="h-5 w-5 text-muted-foreground" aria-hidden="true" />
      <div>
        <p className="text-sm text-muted-foreground">Status tidak diketahui</p>
        <p className="text-xs text-muted-foreground">Belum ada run verifikasi hash-chain.</p>
      </div>
    </div>
  );
}
