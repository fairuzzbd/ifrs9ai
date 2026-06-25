"use client";

import * as React from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { cn } from "@/lib/utils";
import { dlqApi } from "@/lib/api/jurnal.api";
import { usePermissions } from "@/lib/stores/auth.store";

interface Tab {
  label: string;
  href: string;
  permission: string;
  badge?: number;
}

export function JurnalTabNav() {
  const pathname = usePathname();
  const { can } = usePermissions();

  // Fetch DLQ count for badge
  const { data: dlqData } = useQuery({
    queryKey: ["jurnal-dlq-count"],
    queryFn: () =>
      dlqApi.list({
        limit: 1,
        "filter[status]": "FAILED",
      }),
    staleTime: 60_000,
    enabled: can("jurnal.dlq.read"),
  });

  const dlqCount = dlqData?.pagination?.totalEstimate ?? 0;

  const tabs: Tab[] = [
    { label: "Header", href: "/jurnal/header", permission: "jurnal.read" },
    {
      label: "DLQ",
      href: "/jurnal/dlq",
      permission: "jurnal.dlq.read",
      badge: dlqCount > 0 ? dlqCount : undefined,
    },
    { label: "Resolve", href: "/jurnal/resolve", permission: "jurnal.resolve" },
  ];

  const visibleTabs = tabs.filter((t) => can(t.permission));

  return (
    <nav
      role="tablist"
      aria-label="Navigasi Jurnal"
      className="flex gap-1 border-b px-6"
    >
      {visibleTabs.map((tab) => {
        const isActive =
          pathname === tab.href || pathname.startsWith(tab.href + "/");
        return (
          <Link
            key={tab.href}
            href={tab.href}
            prefetch
            role="tab"
            aria-selected={isActive}
            className={cn(
              "flex items-center gap-1.5 px-4 py-3 text-sm font-medium border-b-2 transition-colors -mb-px",
              isActive
                ? "border-primary text-primary"
                : "border-transparent text-muted-foreground hover:text-foreground hover:border-muted-foreground/50",
            )}
          >
            {tab.label}
            {tab.badge !== undefined && (
              <span
                aria-label={`${tab.badge} entri DLQ menunggu`}
                className="ml-1 flex h-5 min-w-5 items-center justify-center rounded-full bg-destructive px-1.5 text-xs font-semibold text-destructive-foreground"
              >
                {tab.badge > 99 ? "99+" : tab.badge}
              </span>
            )}
          </Link>
        );
      })}
    </nav>
  );
}
