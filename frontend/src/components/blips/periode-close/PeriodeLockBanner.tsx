"use client";

import * as React from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, Clock, Lock } from "lucide-react";
import { cn } from "@/lib/utils";
import { periodeStatusApi } from "@/lib/api/periode-close.api";
import type { StatusPeriode } from "@/lib/schemas/periode-close.schema";

// ---------------------------------------------------------------------------
// Banner config per status
// ---------------------------------------------------------------------------

interface BannerConfig {
  Icon: React.ElementType;
  colorClass: string;
  bgClass: string;
  titleFn: (kode: string) => string;
  messageFn: (kode: string, tanggal: string) => string;
  pulse?: boolean;
}

const BANNER_CONFIG: Partial<Record<StatusPeriode, BannerConfig>> = {
  SOFT_CLOSED: {
    Icon: AlertTriangle,
    colorClass: "text-amber-700",
    bgClass: "bg-amber-50 border-amber-300",
    titleFn: (kode) => `PERIODE SOFT-CLOSED — ${kode}`,
    messageFn: (kode, tanggal) =>
      `Periode ${kode} sudah soft-closed pada ${tanggal}. Mutasi tidak diizinkan. GL delivery retry masih diperbolehkan.`,
  },
  HARD_CLOSE_PENDING: {
    Icon: Clock,
    colorClass: "text-orange-700",
    bgClass: "bg-orange-50 border-orange-300",
    titleFn: (kode) => `PERIODE MENUNGGU APPROVAL CFO — ${kode}`,
    messageFn: (kode) =>
      `Hard-close request diajukan untuk ${kode}. Menunggu approval CFO dengan step-up MFA. Mutasi tidak diizinkan.`,
    pulse: true,
  },
  CLOSED: {
    Icon: Lock,
    colorClass: "text-green-800",
    bgClass: "bg-green-50 border-green-400",
    titleFn: (kode) => `PERIODE DITUTUP FINAL — ${kode}`,
    messageFn: (kode, tanggal) =>
      `Periode ${kode} hard-closed pada ${tanggal}. Mutasi tidak bisa dilakukan. Hubungi CFO untuk reopen (grace window tersedia).`,
  },
};

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface PeriodeLockBannerProps {
  periodeId: string;
  /** Optional: pre-loaded data to skip the fetch */
  periodeKode?: string;
  statusPeriode?: StatusPeriode;
  tanggal?: string;
  graceExpiresAt?: string;
}

// ---------------------------------------------------------------------------
// Component (S2-AC1, S3-AC4)
// ---------------------------------------------------------------------------

function fmt(iso: string | null | undefined): string {
  if (!iso) return "—";
  return new Date(iso).toLocaleString("id-ID", {
    day: "2-digit",
    month: "long",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    timeZone: "Asia/Jakarta",
  });
}

export function PeriodeLockBanner({
  periodeId,
  periodeKode: propKode,
  statusPeriode: propStatus,
  tanggal: propTanggal,
}: PeriodeLockBannerProps) {
  // Lazy fetch — only if props not pre-loaded
  const needsFetch = !propStatus;

  const { data } = useQuery({
    queryKey: ["periode-buku", "detail", periodeId],
    queryFn: () => periodeStatusApi.get(periodeId),
    enabled: needsFetch,
    staleTime: 30_000,
    // Fail silent — banner just won't show if fetch fails (design spec §4.3)
    retry: 0,
  });

  const status: StatusPeriode | undefined =
    propStatus ?? (data?.data.statusPeriode as StatusPeriode | undefined);
  const kode = propKode ?? data?.data.periodeKode ?? "";

  // Determine tanggal based on status
  let tanggal = propTanggal ?? "";
  if (!propTanggal && data?.data) {
    const d = data.data;
    if (status === "SOFT_CLOSED") {
      tanggal = fmt(d.tanggalSoftClose);
    } else if (status === "CLOSED") {
      tanggal = fmt(d.tanggalHardClose);
    }
  }

  // Don't render if OPEN or no status yet
  if (!status || status === "OPEN") return null;

  const config = BANNER_CONFIG[status];
  if (!config) return null;

  const { Icon } = config;

  return (
    <div
      role="alert"
      aria-live="assertive"
      className={cn(
        "flex items-center justify-between gap-4 border rounded-md px-4 py-3 mb-4",
        config.bgClass,
      )}
    >
      <div className="flex items-start gap-3 min-w-0">
        <div className="shrink-0 mt-0.5">
          {config.pulse ? (
            <span className="relative flex h-5 w-5">
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-orange-400 opacity-60 motion-reduce:animate-none" />
              <Icon className={cn("relative h-5 w-5", config.colorClass)} aria-hidden="true" />
            </span>
          ) : (
            <Icon className={cn("h-5 w-5", config.colorClass)} aria-hidden="true" />
          )}
        </div>
        <div className="min-w-0">
          <p className={cn("text-sm font-semibold", config.colorClass)}>
            {config.titleFn(kode)}
          </p>
          <p className={cn("text-xs mt-0.5", config.colorClass, "opacity-80")}>
            {config.messageFn(kode, tanggal)}
          </p>
        </div>
      </div>

      {periodeId && (
        <Link
          href={`/periode-buku/${periodeId}`}
          className={cn(
            "shrink-0 text-xs font-medium underline whitespace-nowrap",
            config.colorClass,
            "focus:outline-none focus-visible:ring-2 focus-visible:ring-offset-1 focus-visible:ring-current rounded",
          )}
          aria-label={`Lihat detail periode ${kode}`}
        >
          Lihat detail periode &rarr;
        </Link>
      )}
    </div>
  );
}
