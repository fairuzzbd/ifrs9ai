"use client";

import * as React from "react";
import { TrendingUp, Users, ShieldCheck, AlertTriangle, ShieldX } from "lucide-react";
import { Card, CardContent } from "@/components/ui/card";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Formatter
// ---------------------------------------------------------------------------

function formatIDRCompact(value: string | undefined): string {
  if (!value) return "—";
  const num = parseFloat(value);
  if (isNaN(num)) return value;
  if (num >= 1_000_000_000) {
    return `Rp ${(num / 1_000_000_000).toFixed(2)} M`;
  }
  if (num >= 1_000_000) {
    return `Rp ${(num / 1_000_000).toFixed(2)} Jt`;
  }
  return new Intl.NumberFormat("id-ID", {
    style: "currency",
    currency: "IDR",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(num);
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface PortfolioSummaryKPIProps {
  totalEclWeightedIdr: string;
  totalInstrumen: number;
  stage1Count: number;
  stage2Count: number;
  stage3Count: number;
  errorCount?: number;
  className?: string;
}

// ---------------------------------------------------------------------------
// KPI Card
// ---------------------------------------------------------------------------

interface KpiCardProps {
  title: string;
  value: React.ReactNode;
  icon: React.ElementType;
  iconClass?: string;
  bgClass?: string;
}

function KpiCard({ title, value, icon: Icon, iconClass, bgClass }: KpiCardProps) {
  return (
    <Card className={cn("flex-1 min-w-0", bgClass)}>
      <CardContent className="pt-4 pb-3">
        <div className="flex items-start justify-between gap-2">
          <div className="min-w-0">
            <p className="text-xs text-muted-foreground truncate">{title}</p>
            <p className="text-xl font-bold mt-1 truncate">{value}</p>
          </div>
          <Icon
            className={cn("h-8 w-8 flex-shrink-0 opacity-70", iconClass)}
            aria-hidden="true"
          />
        </div>
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function PortfolioSummaryKPI({
  totalEclWeightedIdr,
  totalInstrumen,
  stage1Count,
  stage2Count,
  stage3Count,
  errorCount = 0,
  className,
}: PortfolioSummaryKPIProps) {
  return (
    <div
      className={cn("flex flex-wrap gap-3", className)}
      role="region"
      aria-label="KPI Ringkasan Portofolio"
    >
      <KpiCard
        title="Total ECL Weighted"
        value={formatIDRCompact(totalEclWeightedIdr)}
        icon={TrendingUp}
        iconClass="text-primary"
      />
      <KpiCard
        title="Total Instrumen"
        value={totalInstrumen.toLocaleString("id-ID")}
        icon={Users}
        iconClass="text-blue-500"
      />
      <KpiCard
        title="Stage 1"
        value={stage1Count.toLocaleString("id-ID")}
        icon={ShieldCheck}
        iconClass="text-green-600"
        bgClass="border-green-100"
      />
      <KpiCard
        title="Stage 2"
        value={stage2Count.toLocaleString("id-ID")}
        icon={AlertTriangle}
        iconClass="text-amber-600"
        bgClass="border-amber-100"
      />
      <KpiCard
        title="Stage 3"
        value={stage3Count.toLocaleString("id-ID")}
        icon={ShieldX}
        iconClass="text-red-600"
        bgClass="border-red-100"
      />
      {errorCount > 0 && (
        <KpiCard
          title="Error"
          value={errorCount.toLocaleString("id-ID")}
          icon={ShieldX}
          iconClass="text-destructive"
          bgClass="border-destructive/20"
        />
      )}
    </div>
  );
}
