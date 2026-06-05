"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { PeriodeBukuForm } from "@/app/master/periode-buku/_components/PeriodeBukuForm";
import { periodeBukuApi } from "@/lib/api/periode-buku.api";
import { notify } from "@/lib/notify";

export default function EditPeriodeBukuPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();

  const { data, isLoading, isError } = useQuery({
    queryKey: ["periode-buku", id],
    queryFn: () => periodeBukuApi.get(id),
    enabled: !!id,
  });

  const item = data?.data;

  React.useEffect(() => {
    if (!isLoading && item) {
      // Guard: only DRAFT or RETURNED can be edited
      if (
        item.workflowStatus !== "DRAFT" &&
        item.workflowStatus !== "RETURNED"
      ) {
        notify.error({
          code: "MASTER_APPROVED_NO_EDIT",
          message: `Periode ${item.periodeIdKode} tidak bisa diedit karena status workflow ${item.workflowStatus}.`,
          traceId: "",
        });
        router.push(`/master/periode-buku/${id}`);
      }
    }
  }, [item, isLoading, id, router]);

  if (isLoading) {
    return (
      <div className="container mx-auto max-w-2xl py-6 space-y-4">
        <Skeleton className="h-4 w-48" />
        <Skeleton className="h-8 w-64" />
        <div className="rounded-lg border p-6 space-y-4">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-10 w-full" />
          ))}
        </div>
      </div>
    );
  }

  if (isError || !item) {
    return (
      <div className="container mx-auto max-w-2xl py-6 space-y-4">
        <p className="text-sm text-destructive">
          Gagal memuat data periode buku.
        </p>
        <Button variant="outline" asChild>
          <Link href="/master/periode-buku">Kembali ke Daftar</Link>
        </Button>
      </div>
    );
  }

  return (
    <div className="container mx-auto max-w-2xl py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/periode-buku" className="hover:underline">
          Periode Buku
        </Link>
        <span className="mx-1.5">/</span>
        <Link
          href={`/master/periode-buku/${id}`}
          className="hover:underline"
        >
          {item.periodeIdKode}
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Edit</span>
      </nav>
      <h1 className="text-2xl font-semibold">
        Edit Periode &mdash; {item.periodeIdKode}
      </h1>
      <PeriodeBukuForm mode="edit" defaultValues={item} />
    </div>
  );
}
