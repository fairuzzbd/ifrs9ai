"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { MataUangForm } from "@/app/master/mata-uang/_components/MataUangForm";
import { mataUangApi } from "@/lib/api/mata-uang.api";
import { notify } from "@/lib/notify";

export default function EditMataUangPage() {
  const { kode } = useParams<{ kode: string }>();
  const router = useRouter();

  const { data, isLoading, isError } = useQuery({
    queryKey: ["mata-uang", kode],
    queryFn: () => mataUangApi.get(kode),
    enabled: !!kode,
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
          message: `Mata uang ${kode} tidak bisa diedit karena status ${item.workflowStatus}.`,
          traceId: "",
        });
        router.push(`/master/mata-uang/${kode}`);
      }
    }
  }, [item, isLoading, kode, router]);

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
          Gagal memuat data mata uang {kode}.
        </p>
        <Button variant="outline" asChild>
          <Link href="/master/mata-uang">Kembali ke Daftar</Link>
        </Button>
      </div>
    );
  }

  return (
    <div className="container mx-auto max-w-2xl py-6 space-y-4">
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
        <span className="text-foreground font-medium">Edit</span>
      </nav>
      <h1 className="text-2xl font-semibold">
        Edit Mata Uang &mdash; {kode}
      </h1>
      <MataUangForm mode="edit" defaultValues={item} />
    </div>
  );
}
