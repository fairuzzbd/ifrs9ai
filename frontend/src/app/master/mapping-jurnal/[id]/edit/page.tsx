"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { MappingJurnalForm } from "@/app/master/mapping-jurnal/_components/MappingJurnalForm";
import { mappingJurnalApi } from "@/lib/api/mapping-jurnal.api";
import { notify } from "@/lib/notify";

export default function EditMappingJurnalPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();

  const { data, isLoading, isError } = useQuery({
    queryKey: ["mapping-jurnal", id],
    queryFn: () => mappingJurnalApi.get(id),
    enabled: !!id,
  });

  const item = data?.data;

  React.useEffect(() => {
    if (!isLoading && item) {
      if (
        item.workflowStatus !== "DRAFT" &&
        item.workflowStatus !== "RETURNED"
      ) {
        notify.error({
          code: "MASTER_APPROVED_NO_EDIT",
          message: `Mapping jurnal "${item.namaEvent}" tidak bisa diedit karena status ${item.workflowStatus}.`,
          traceId: "",
        });
        router.push(`/master/mapping-jurnal/${id}`);
      }
    }
  }, [item, isLoading, id, router]);

  if (isLoading) {
    return (
      <div className="container mx-auto max-w-4xl py-6 space-y-4">
        <Skeleton className="h-4 w-48" />
        <Skeleton className="h-8 w-64" />
        <div className="rounded-lg border p-6 space-y-4">
          {Array.from({ length: 8 }).map((_, i) => (
            <Skeleton key={i} className="h-10 w-full" />
          ))}
        </div>
      </div>
    );
  }

  if (isError || !item) {
    return (
      <div className="container mx-auto max-w-4xl py-6 space-y-4">
        <p className="text-sm text-destructive">
          Gagal memuat data mapping jurnal.
        </p>
        <Button variant="outline" asChild>
          <Link href="/master/mapping-jurnal">Kembali ke Daftar</Link>
        </Button>
      </div>
    );
  }

  return (
    <div className="container mx-auto max-w-4xl py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/mapping-jurnal" className="hover:underline">
          Mapping Jurnal
        </Link>
        <span className="mx-1.5">/</span>
        <Link
          href={`/master/mapping-jurnal/${id}`}
          className="hover:underline"
        >
          {item.eventCode}
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Edit</span>
      </nav>
      <h1 className="text-2xl font-semibold">
        Edit Mapping Jurnal &mdash; {item.eventCode}
      </h1>
      <MappingJurnalForm mode="edit" defaultValues={item} />
    </div>
  );
}
