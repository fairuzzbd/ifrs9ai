"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { CounterpartyForm } from "@/app/master/counterparty/_components/CounterpartyForm";
import { counterpartyApi } from "@/lib/api/counterparty.api";
import { notify } from "@/lib/notify";

export default function EditCounterpartyPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();

  const { data, isLoading, isError } = useQuery({
    queryKey: ["counterparty", id],
    queryFn: () => counterpartyApi.get(id),
    enabled: !!id,
  });

  const item = data?.data;

  React.useEffect(() => {
    if (!isLoading && item) {
      if (item.workflowStatus !== "DRAFT" && item.workflowStatus !== "RETURNED") {
        notify.error({
          code: "MASTER_APPROVED_NO_EDIT",
          message: `Counterparty ${item.kodeCounterparty} tidak bisa diedit karena status ${item.workflowStatus}.`,
          traceId: "",
        });
        router.push(`/master/counterparty/${id}`);
      }
    }
  }, [item, isLoading, id, router]);

  if (isLoading) {
    return (
      <div className="container mx-auto max-w-3xl py-6 space-y-4">
        <Skeleton className="h-4 w-48" />
        <Skeleton className="h-8 w-64" />
        <div className="rounded-lg border p-6 space-y-4">
          {Array.from({ length: 8 }).map((_, i) => <Skeleton key={i} className="h-10 w-full" />)}
        </div>
      </div>
    );
  }

  if (isError || !item) {
    return (
      <div className="container mx-auto max-w-3xl py-6 space-y-4">
        <p className="text-sm text-destructive">Gagal memuat data counterparty.</p>
        <Button variant="outline" asChild><Link href="/master/counterparty">Kembali ke Daftar</Link></Button>
      </div>
    );
  }

  return (
    <div className="container mx-auto max-w-3xl py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/counterparty" className="hover:underline">Counterparty</Link>
        <span className="mx-1.5">/</span>
        <Link href={`/master/counterparty/${id}`} className="hover:underline">{item.kodeCounterparty}</Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Edit</span>
      </nav>
      <h1 className="text-2xl font-semibold">Edit Counterparty &mdash; {item.kodeCounterparty}</h1>
      <CounterpartyForm mode="edit" defaultValues={item} />
    </div>
  );
}
