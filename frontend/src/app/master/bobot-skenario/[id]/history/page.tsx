"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { useParams } from "next/navigation";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { AuditHistoryTable } from "@/components/blips/AuditHistoryTable";
import { bobotSkenarioApi } from "@/lib/api/bobot-skenario.api";
import { usePermissions } from "@/lib/stores/auth.store";
import type { AuditHistoryEntry } from "@/lib/api/bobot-skenario.api";

export default function BobotSkenarioHistoryPage() {
  const { id } = useParams<{ id: string }>();
  const perms = usePermissions();

  const [cursor, setCursor] = React.useState<string | null>(null);
  const [allEntries, setAllEntries] = React.useState<AuditHistoryEntry[]>([]);

  const { data, isLoading } = useQuery({
    queryKey: ["bobot-skenario-history", id, cursor],
    queryFn: () =>
      bobotSkenarioApi.getHistory(id, {
        limit: 50,
        cursor: cursor ?? undefined,
      }),
    enabled: !!id,
  });

  React.useEffect(() => {
    if (data?.data) {
      setAllEntries((prev) =>
        cursor ? [...prev, ...data.data] : data.data,
      );
    }
  }, [data, cursor]);

  const handleLoadMore = () => {
    if (data?.pagination.nextCursor) {
      setCursor(data.pagination.nextCursor);
    }
  };

  return (
    <div className="container mx-auto py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/bobot-skenario" className="hover:underline">
          Bobot Skenario ECL
        </Link>
        <span className="mx-1.5">/</span>
        <Link href={`/master/bobot-skenario/${id}`} className="hover:underline">
          Detail
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Riwayat Audit</span>
      </nav>

      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">
          Riwayat Audit — Bobot Skenario ECL
        </h1>
        <Button variant="outline" size="sm" asChild>
          <Link href={`/master/bobot-skenario/${id}`}>&larr; Kembali ke Detail</Link>
        </Button>
      </div>

      <AuditHistoryTable
        entries={allEntries}
        isLoading={isLoading && cursor === null}
        isAuditRole={perms.isAuditRole()}
        hasMore={data?.pagination.hasMore ?? false}
        onLoadMore={handleLoadMore}
      />
    </div>
  );
}
