"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { ImpactPdForm } from "@/app/master/impact-pd/_components/ImpactPdForm";
import { impactPdApi } from "@/lib/api/impact-pd.api";
import { notify } from "@/lib/notify";

export default function EditImpactPdPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();

  const { data, isLoading, isError } = useQuery({
    queryKey: ["impact-pd", id],
    queryFn: () => impactPdApi.get(id),
    enabled: !!id,
  });

  const item = data?.data;

  React.useEffect(() => {
    if (!isLoading && item) {
      const isEditable = item.workflowStatus === "DRAFT" || item.workflowStatus === "REJECTED";
      if (!isEditable) {
        notify.error({
          code: "MASTER_APPROVED_NO_EDIT",
          message: `Impact PD tidak bisa diedit karena status ${item.workflowStatus as string}.`,
          traceId: "",
        });
        router.push(`/master/impact-pd/${id}`);
      }
    }
  }, [item, isLoading, id, router]);

  if (isLoading) {
    return (
      <div className="container mx-auto max-w-2xl py-6 space-y-4">
        <Skeleton className="h-4 w-48" />
        <Skeleton className="h-8 w-64" />
        <div className="rounded-lg border p-6 space-y-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-10 w-full" />
          ))}
        </div>
      </div>
    );
  }

  if (isError || !item) {
    return (
      <div className="container mx-auto max-w-2xl py-6 space-y-4">
        <p className="text-sm text-destructive">Gagal memuat data Impact PD.</p>
        <Button variant="outline" asChild>
          <Link href="/master/impact-pd">Kembali ke Daftar</Link>
        </Button>
      </div>
    );
  }

  return (
    <div className="container mx-auto max-w-2xl py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/impact-pd" className="hover:underline">
          Impact PD
        </Link>
        <span className="mx-1.5">/</span>
        <Link href={`/master/impact-pd/${id}`} className="hover:underline">
          {id.slice(0, 8)}&hellip;
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Edit</span>
      </nav>
      <h1 className="text-2xl font-semibold">Edit Impact PD</h1>
      <ImpactPdForm mode="edit" defaultValues={item} />
    </div>
  );
}
