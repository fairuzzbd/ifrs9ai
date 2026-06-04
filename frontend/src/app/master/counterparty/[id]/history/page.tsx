"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { useParams } from "next/navigation";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { AuditHistoryTable } from "@/components/blips/AuditHistoryTable";
import { counterpartyApi } from "@/lib/api/counterparty.api";
import { usePermissions } from "@/lib/stores/auth.store";

export default function CounterpartyHistoryPage() {
  const { id } = useParams<{ id: string }>();
  const perms = usePermissions();

  const [cursor, setCursor] = React.useState<string | null>(null);
  const [allEntries, setAllEntries] = React.useState<
    Awaited<ReturnType<typeof counterpartyApi.getHistory>>["data"]
  >([]);

  const { data, isLoading } = useQuery({
    queryKey: ["counterparty-history", id, cursor],
    queryFn: () => counterpartyApi.getHistory(id, { limit: 50, cursor: cursor ?? undefined }),
    enabled: !!id,
  });

  React.useEffect(() => {
    if (data?.data) {
      setAllEntries((prev) => cursor ? [...prev, ...data.data] : data.data);
    }
  }, [data, cursor]);

  const handleLoadMore = () => {
    if (data?.pagination.nextCursor) setCursor(data.pagination.nextCursor);
  };

  return (
    <div className="container mx-auto py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/counterparty" className="hover:underline">Counterparty</Link>
        <span className="mx-1.5">/</span>
        <Link href={`/master/counterparty/${id}`} className="hover:underline">{id}</Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Riwayat Audit</span>
      </nav>

      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">Riwayat Audit — Counterparty</h1>
        <Button variant="outline" size="sm" asChild>
          <Link href={`/master/counterparty/${id}`}>&larr; Kembali ke Detail</Link>
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
