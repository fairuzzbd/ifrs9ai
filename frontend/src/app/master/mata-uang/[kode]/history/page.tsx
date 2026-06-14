"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { useParams } from "next/navigation";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { AuditHistoryTable } from "@/components/blips/AuditHistoryTable";
import { mataUangApi } from "@/lib/api/mata-uang.api";
import { usePermissions } from "@/lib/stores/auth.store";

export default function MataUangHistoryPage() {
  const { kode } = useParams<{ kode: string }>();
  const perms = usePermissions();

  const [cursor, setCursor] = React.useState<string | null>(null);
  const [allEntries, setAllEntries] = React.useState<
    Awaited<
      ReturnType<typeof mataUangApi.getHistory>
    >["data"]
  >([]);

  const { data, isLoading } = useQuery({
    queryKey: ["mata-uang-history", kode, cursor],
    queryFn: () =>
      mataUangApi.getHistory(kode, {
        limit: 50,
        cursor: cursor ?? undefined,
      }),
    enabled: !!kode,
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
        <Link href="/master/mata-uang" className="hover:underline">
          Mata Uang
        </Link>
        <span className="mx-1.5">/</span>
        <Link
          href={`/master/mata-uang/${kode}`}
          className="hover:underline"
        >
          {kode}
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Riwayat Audit</span>
      </nav>

      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">
          Riwayat Audit — Mata Uang {kode}
        </h1>
        <Button variant="outline" size="sm" asChild>
          <Link href={`/master/mata-uang/${kode}`}>
            &larr; Kembali ke Detail
          </Link>
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
