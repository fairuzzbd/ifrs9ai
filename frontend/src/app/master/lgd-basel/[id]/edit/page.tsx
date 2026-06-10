"use client";

import * as React from "react";
import { useQuery } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { LGDBaselForm } from "@/app/master/lgd-basel/_components/LGDBaselForm";
import { lgdBaselApi } from "@/lib/api/lgd-basel.api";
import { notify } from "@/lib/notify";
import { TIPE_EKSPOSUR_LABELS } from "@/lib/schemas/lgd-basel.schema";

export default function EditLGDBaselPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();

  const { data, isLoading, isError } = useQuery({
    queryKey: ["lgd-basel", id],
    queryFn: () => lgdBaselApi.get(id),
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
          message: `LGD pool ${TIPE_EKSPOSUR_LABELS[item.tipeEksposur] ?? item.tipeEksposur} tidak bisa diedit karena status ${item.workflowStatus}.`,
          traceId: "",
        });
        router.push(`/master/lgd-basel/${id}`);
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
          Gagal memuat data LGD pool {id}.
        </p>
        <Button variant="outline" asChild>
          <Link href="/master/lgd-basel">Kembali ke Daftar</Link>
        </Button>
      </div>
    );
  }

  const tipeLabel = TIPE_EKSPOSUR_LABELS[item.tipeEksposur] ?? item.tipeEksposur;

  return (
    <div className="container mx-auto max-w-2xl py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/lgd-basel" className="hover:underline">
          LGD Basel Pool
        </Link>
        <span className="mx-1.5">/</span>
        <Link href={`/master/lgd-basel/${id}`} className="hover:underline">
          {tipeLabel}
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Edit</span>
      </nav>
      <h1 className="text-2xl font-semibold">
        Edit LGD Pool &mdash; {tipeLabel}
      </h1>
      <LGDBaselForm mode="edit" defaultValues={item} />
    </div>
  );
}
