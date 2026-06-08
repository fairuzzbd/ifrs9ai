"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { BobotSkenarioForm } from "@/app/master/bobot-skenario/_components/BobotSkenarioForm";
import { bobotSkenarioApi } from "@/lib/api/bobot-skenario.api";
import { notify } from "@/lib/notify";
import { SKENARIO_ECL_LABELS } from "@/lib/schemas/bobot-skenario.schema";

export default function EditBobotSkenarioPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();

  const { data, isLoading, isError } = useQuery({
    queryKey: ["bobot-skenario", id],
    queryFn: () => bobotSkenarioApi.get(id),
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
          message: `Bobot skenario ${SKENARIO_ECL_LABELS[item.skenario] ?? item.skenario} tidak bisa diedit karena status ${item.workflowStatus}.`,
          traceId: "",
        });
        router.push(`/master/bobot-skenario/${id}`);
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
          Gagal memuat data bobot skenario {id}.
        </p>
        <Button variant="outline" asChild>
          <Link href="/master/bobot-skenario">Kembali ke Daftar</Link>
        </Button>
      </div>
    );
  }

  const skenarioLabel =
    SKENARIO_ECL_LABELS[item.skenario] ?? item.skenario;

  return (
    <div className="container mx-auto max-w-2xl py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/bobot-skenario" className="hover:underline">
          Bobot Skenario ECL
        </Link>
        <span className="mx-1.5">/</span>
        <Link
          href={`/master/bobot-skenario/${id}`}
          className="hover:underline"
        >
          {skenarioLabel}
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Edit</span>
      </nav>
      <h1 className="text-2xl font-semibold">
        Edit Bobot Skenario &mdash; {skenarioLabel}
      </h1>
      <BobotSkenarioForm mode="edit" defaultValues={item} />
    </div>
  );
}
