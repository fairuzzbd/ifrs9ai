"use client";

import * as React from "react";
import {
  FileText, CheckCircle2, XCircle, Loader2,
  AlertTriangle, CheckCheck, Clock, RotateCcw, Ban,
} from "lucide-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import type { BulkBatchStatus } from "@/lib/schemas/bulkupload.schema";
import { BULK_BATCH_STATUS_LABELS } from "@/lib/schemas/bulkupload.schema";

interface BadgeConfig {
  colorClass: string;
  Icon: React.ElementType;
  tooltip: string;
  animate?: boolean;
}

const BADGE_CONFIG: Record<BulkBatchStatus, BadgeConfig> = {
  PARSED: {
    Icon: FileText,
    colorClass: "bg-slate-50 text-slate-700 border-slate-300",
    tooltip: "File berhasil di-parse. Jalankan DRY_RUN untuk validasi 4-tahap sebelum commit.",
  },
  DRY_RUN_PASSED: {
    Icon: CheckCircle2,
    colorClass: "bg-blue-50 text-blue-700 border-blue-300",
    tooltip: "Validasi 4-tahap lulus (Stage 1–3 semua pass). Siap untuk COMMIT dalam TTL 1 jam.",
  },
  DRY_RUN_FAILED: {
    Icon: XCircle,
    colorClass: "bg-red-50 text-red-700 border-red-300",
    tooltip: "Validasi gagal (Stage 1–3 ada error). COMMIT diblokir. Perbaiki baris bermasalah dan upload ulang.",
  },
  COMMITTING: {
    Icon: Loader2,
    colorClass: "bg-yellow-50 text-yellow-700 border-yellow-300",
    tooltip: "Asynq worker sedang commit instrumen ke mst.instrumen. Pantau progress via JobProgressPanel.",
    animate: true,
  },
  COMMITTED: {
    Icon: CheckCircle2,
    colorClass: "bg-green-50 text-green-700 border-green-300",
    tooltip: "Semua baris berhasil di-INSERT ke mst.instrumen (status PENDING_APPROVAL_BULK). Menunggu approve ROLE-APPR-TR.",
  },
  PARTIAL_COMMIT: {
    Icon: AlertTriangle,
    colorClass: "bg-amber-50 text-amber-700 border-amber-300",
    tooltip: "Sebagian baris berhasil commit; ada baris FAILED (partial commit). Approve masih diizinkan untuk baris COMMITTED.",
  },
  APPROVED: {
    Icon: CheckCheck,
    colorClass: "bg-green-50 text-green-700 border-green-300",
    tooltip: "Batch di-approve ROLE-APPR-TR. Instrumen COMMITTED menjadi ACTIVE. Flagged rows tetap PENDING_CLASSIFICATION.",
  },
  ROLLBACK_PENDING: {
    Icon: Clock,
    colorClass: "bg-orange-50 text-orange-700 border-orange-300",
    tooltip: "CFO sudah mengajukan rollback. Menunggu konfirmasi rollback-approve dengan step-up MFA.",
  },
  ROLLED_BACK: {
    Icon: RotateCcw,
    colorClass: "bg-slate-50 text-slate-500 border-slate-300",
    tooltip: "Batch di-rollback CFO. Semua instrumen soft-deleted (DEC-018). Status terminal — tidak bisa diubah.",
  },
};

const SIZE_CLASS = {
  sm: "text-xs px-1.5 py-0.5 gap-1",
  default: "text-sm px-2 py-1 gap-1.5",
};

const ICON_SIZE = {
  sm: "h-3 w-3",
  default: "h-4 w-4",
};

export interface BulkBatchStatusBadgeProps {
  status: BulkBatchStatus;
  size?: "sm" | "default";
  className?: string;
}

export function BulkBatchStatusBadge({ status, size = "default", className }: BulkBatchStatusBadgeProps) {
  const config = BADGE_CONFIG[status];
  const { Icon } = config;
  const label = BULK_BATCH_STATUS_LABELS[status];

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          className={cn(
            "inline-flex items-center rounded-md border font-medium cursor-default",
            SIZE_CLASS[size],
            config.colorClass,
            className,
          )}
          role="status"
          aria-label={`Status batch: ${label}`}
        >
          <Icon
            className={cn(ICON_SIZE[size], config.animate && "animate-spin")}
            aria-hidden="true"
          />
          <span>{label}</span>
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <p className="max-w-xs text-xs">{config.tooltip}</p>
      </TooltipContent>
    </Tooltip>
  );
}
