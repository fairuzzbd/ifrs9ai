"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { PDPefindoForm } from "@/app/master/pd-pefindo/_components/PDPefindoForm";
import { pdPefindoApi } from "@/lib/api/pd-pefindo.api";
import { notify } from "@/lib/notify";

export default function EditPDPefindoPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();

  const { data, isLoading, isError } = useQuery({
    queryKey: ["pd-pefindo", id],
    queryFn: () => pdPefindoApi.get(id),
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
          message: `PD Pefindo ${item.rating} tidak bisa diedit karena status ${item.workflowStatus}.`,
          traceId: "",
        });
        router.push(`/master/pd-pefindo/${id}`);
      }
    }
  }, [item, isLoading, id, router]);

  if (isLoading) {
    return (
      <div className="container mx-auto max-w-3xl py-6 space-y-4">
        <Skeleton className="h-4 w-48" />
        <Skeleton className="h-8 w-64" />
        <div className="rounded-lg border p-6 space-y-4">
          {Array.from({ length: 7 }).map((_, i) => (
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
          Gagal memuat data PD Pefindo {id}.
        </p>
        <Button variant="outline" asChild>
          <Link href="/master/pd-pefindo">Kembali ke Daftar</Link>
        </Button>
      </div>
    );
  }

  return (
    <div className="container mx-auto max-w-3xl py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/pd-pefindo" className="hover:underline">
          PD Pefindo
        </Link>
        <span className="mx-1.5">/</span>
        <Link href={`/master/pd-pefindo/${id}`} className="hover:underline">
          {item.rating}
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Edit</span>
      </nav>
      <h1 className="text-2xl font-semibold">
        Edit PD Pefindo &mdash;{" "}
        <code className="font-mono">{item.rating}</code>
      </h1>
      <PDPefindoForm
        mode="edit"
        defaultValues={{
          ...item,
          rowVersion: item.rowVersion,
        }}
      />
    </div>
  );
}
