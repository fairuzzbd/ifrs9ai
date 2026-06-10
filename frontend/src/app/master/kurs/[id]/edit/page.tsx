"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { KursForm } from "@/app/master/kurs/_components/KursForm";
import { kursApi } from "@/lib/api/kurs.api";
import { notify } from "@/lib/notify";

export default function EditKursPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();

  const { data, isLoading, isError } = useQuery({
    queryKey: ["kurs", id],
    queryFn: () => kursApi.get(id),
    enabled: !!id,
  });

  const item = data?.data;

  React.useEffect(() => {
    if (!isLoading && item) {
      // Guard: only DRAFT or RETURNED can be edited
      if (item.workflowStatus !== "DRAFT" && item.workflowStatus !== "RETURNED") {
        notify.error({
          code: "MASTER_APPROVED_NO_EDIT",
          message: `Kurs ${item.fxRateIdKode} tidak bisa diedit karena status ${item.workflowStatus}.`,
          traceId: "",
        });
        router.push(`/master/kurs/${id}`);
        return;
      }
      // Guard: locked_flag = true → redirect
      if (item.lockedFlag) {
        notify.warning("Kurs ini terkunci karena periode buku sudah di-hard-close. Data tidak bisa diedit.");
        router.push(`/master/kurs/${id}`);
      }
    }
  }, [item, isLoading, id, router]);

  if (isLoading) {
    return (
      <div className="container mx-auto max-w-3xl py-6 space-y-4">
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
      <div className="container mx-auto max-w-3xl py-6 space-y-4">
        <p className="text-sm text-destructive">Gagal memuat data kurs {id}.</p>
        <Button variant="outline" asChild>
          <Link href="/master/kurs">Kembali ke Daftar</Link>
        </Button>
      </div>
    );
  }

  return (
    <div className="container mx-auto max-w-3xl py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/kurs" className="hover:underline">
          Kurs
        </Link>
        <span className="mx-1.5">/</span>
        <Link href={`/master/kurs/${id}`} className="hover:underline">
          {item.fxRateIdKode}
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Edit</span>
      </nav>
      <h1 className="text-2xl font-semibold">
        Edit Kurs &mdash; {item.fxRateIdKode}
      </h1>
      <KursForm mode="edit" defaultValues={item} />
    </div>
  );
}
