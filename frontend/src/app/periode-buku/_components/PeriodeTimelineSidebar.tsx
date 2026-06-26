"use client";

import * as React from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { periodeStatusApi } from "@/lib/api/periode-close.api";
import { PeriodeStatusBadge } from "@/components/blips/periode-close/PeriodeStatusBadge";
import type { StatusPeriode } from "@/lib/schemas/periode-close.schema";
import { BULAN_LABELS } from "@/lib/schemas/periode-close.schema";

interface Props {
  canCreate: boolean;
}

export function PeriodeTimelineSidebar({ canCreate }: Props) {
  const pathname = usePathname();

  const { data } = useQuery({
    queryKey: ["periode-buku", "sidebar", "list"],
    queryFn: () =>
      periodeStatusApi.list({
        limit: 12,
        sort: "tahun_buku:desc,bulan:desc",
      }),
    staleTime: 30_000,
  });

  const rows = data?.data ?? [];
  const hasMore = data?.pagination?.hasMore ?? false;

  return (
    <>
      {/* Timeline items */}
      <div className="space-y-1">
        {rows.map((row, idx) => {
          const href = `/periode-buku/${row.id}`;
          const isActive = pathname === href;
          const isLast = idx === rows.length - 1;

          return (
            <div key={row.id} className="flex gap-2">
              {/* Timeline line */}
              <div className="flex flex-col items-center pt-1.5">
                <div
                  className={`h-2.5 w-2.5 rounded-full shrink-0 ${
                    row.statusPeriode === "OPEN"
                      ? "bg-green-500"
                      : row.statusPeriode === "SOFT_CLOSED"
                        ? "bg-yellow-500"
                        : row.statusPeriode === "CLOSED"
                          ? "bg-red-500"
                          : "bg-blue-500"
                  }`}
                />
                {!isLast && (
                  <div className="w-0.5 bg-border flex-1 min-h-[20px]" />
                )}
              </div>

              {/* Item card */}
              <Link
                href={href}
                className={`flex-1 rounded-lg px-3 py-2 text-sm transition-colors hover:bg-muted/50 ${
                  isActive
                    ? "border-l-2 border-primary bg-primary/5 font-medium"
                    : ""
                }`}
                aria-current={isActive ? "page" : undefined}
              >
                <div className="font-mono text-xs text-muted-foreground">
                  {row.periodeKode}
                </div>
                <div className="font-medium">
                  {BULAN_LABELS[row.bulan as keyof typeof BULAN_LABELS] ?? String(row.bulan)}{" "}
                  {row.tahunBuku}
                </div>
                <div className="mt-1">
                  <PeriodeStatusBadge
                    status={row.statusPeriode as StatusPeriode}
                    size="sm"
                  />
                </div>
              </Link>
            </div>
          );
        })}
      </div>

      {/* "Lihat lebih" link */}
      {hasMore && (
        <div className="pt-2 pb-1">
          <Link
            href="/periode-buku"
            className="text-xs text-muted-foreground hover:text-foreground hover:underline"
          >
            Lihat lebih banyak periode &rarr;
          </Link>
        </div>
      )}

      {/* + Periode Baru button */}
      {canCreate && (
        <div className="pt-4 border-t">
          <Button size="sm" variant="outline" className="w-full" asChild>
            <Link href="/periode-buku/new">
              <Plus className="mr-1.5 h-3.5 w-3.5" aria-hidden="true" />
              Periode Baru
            </Link>
          </Button>
        </div>
      )}
    </>
  );
}
