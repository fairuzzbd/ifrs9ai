"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { InstrumenForm } from "@/app/master/instrumen/_components/InstrumenForm";
import { instrumenApi } from "@/lib/api/instrumen.api";
import { notify } from "@/lib/notify";

export default function EditInstrumenPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();

  const { data, isLoading, isError } = useQuery({
    queryKey: ["instrumen", id],
    queryFn: () => instrumenApi.get(id),
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
          message: `Instrumen ${item.kodeInstrumen} tidak bisa diedit karena status ${item.workflowStatus}.`,
          traceId: "",
        });
        router.push(`/master/instrumen/${id}`);
      }
    }
  }, [item, isLoading, id, router]);

  if (isLoading) {
    return (
      <div className="container mx-auto max-w-3xl py-6 space-y-4">
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
      <div className="container mx-auto max-w-3xl py-6 space-y-4">
        <p className="text-sm text-destructive">
          Gagal memuat data instrumen {id}.
        </p>
        <Button variant="outline" asChild>
          <Link href="/master/instrumen">Kembali ke Daftar</Link>
        </Button>
      </div>
    );
  }

  return (
    <div className="container mx-auto max-w-3xl py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/instrumen" className="hover:underline">
          Instrumen
        </Link>
        <span className="mx-1.5">/</span>
        <Link href={`/master/instrumen/${id}`} className="hover:underline">
          {item.kodeInstrumen}
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Edit</span>
      </nav>
      <h1 className="text-2xl font-semibold">
        Edit Instrumen &mdash; {item.kodeInstrumen}
      </h1>
      <InstrumenForm mode="edit" defaultValues={item} />
    </div>
  );
}
