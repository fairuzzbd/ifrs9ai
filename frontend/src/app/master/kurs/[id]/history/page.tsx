"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { useParams } from "next/navigation";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { AuditHistoryTable } from "@/components/blips/AuditHistoryTable";
import { kursApi } from "@/lib/api/kurs.api";
import { usePermissions } from "@/lib/stores/auth.store";
import type { KursAuditHistoryEntry } from "@/lib/api/kurs.api";

export default function KursHistoryPage() {
  const { id } = useParams<{ id: string }>();
  const perms = usePermissions();

  const [cursor, setCursor] = React.useState<string | null>(null);
  const [allEntries, setAllEntries] = React.useState<KursAuditHistoryEntry[]>([]);

  const { data, isLoading } = useQuery({
    queryKey: ["kurs-history", id, cursor],
    queryFn: () =>
      kursApi.getHistory(id, { limit: 50, cursor: cursor ?? undefined }),
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

  // Adapt entries to the AuditHistoryTable interface (which expects actorUsername)
  const adaptedEntries = allEntries.map((e) => ({
    ...e,
    actorUsername: e.actorUsername ?? e.actorUserId,
  }));

  return (
    <div className="container mx-auto py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/kurs" className="hover:underline">
          Kurs
        </Link>
        <span className="mx-1.5">/</span>
        <Link href={`/master/kurs/${id}`} className="hover:underline">
          {id}
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Riwayat Audit</span>
      </nav>

      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">Riwayat Audit — Kurs</h1>
        <Button variant="outline" size="sm" asChild>
          <Link href={`/master/kurs/${id}`}>&larr; Kembali ke Detail</Link>
        </Button>
      </div>

      <AuditHistoryTable
        entries={adaptedEntries}
        isLoading={isLoading && cursor === null}
        isAuditRole={perms.isAuditRole()}
        hasMore={data?.pagination.hasMore ?? false}
        onLoadMore={handleLoadMore}
      />
    </div>
  );
}
