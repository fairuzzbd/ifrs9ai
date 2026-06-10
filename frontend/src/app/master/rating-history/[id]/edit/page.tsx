"use client";

import * as React from "react";
import { Suspense } from "react";
import { useQuery } from "@tanstack/react-query";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { RatingHistoryForm } from "@/app/master/rating-history/_components/RatingHistoryForm";
import { ratingHistoryApi } from "@/lib/api/rating-history.api";

function EditRatingHistoryContent() {
  const { id } = useParams<{ id: string }>();

  const { data, isLoading, isError } = useQuery({
    queryKey: ["rating-history", id],
    queryFn: () => ratingHistoryApi.get(id),
    enabled: !!id,
  });

  const item = data?.data;

  if (isLoading) {
    return (
      <div className="container mx-auto max-w-3xl py-6 space-y-4">
        <Skeleton className="h-4 w-48" />
        <Skeleton className="h-8 w-64" />
        <div className="rounded-lg border p-6 space-y-4">
          {Array.from({ length: 6 }).map((_, i) => <Skeleton key={i} className="h-10 w-full" />)}
        </div>
      </div>
    );
  }

  if (isError || !item) {
    return (
      <div className="container mx-auto max-w-3xl py-6 space-y-4">
        <p className="text-sm text-destructive">Gagal memuat data rating history.</p>
        <Button variant="outline" asChild><Link href="/master/rating-history">Kembali ke Daftar</Link></Button>
      </div>
    );
  }

  return (
    <div className="container mx-auto max-w-3xl py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/rating-history" className="hover:underline">Rating History</Link>
        <span className="mx-1.5">/</span>
        <Link href={`/master/rating-history/${id}`} className="hover:underline">
          {item.ratingPefindo} — {item.tanggalBerlaku}
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Edit</span>
      </nav>
      <h1 className="text-2xl font-semibold">
        Edit Rating History &mdash; {item.ratingPefindo}
      </h1>
      <RatingHistoryForm mode="edit" defaultValues={item} />
    </div>
  );
}

export default function EditRatingHistoryPage() {
  return (
    <Suspense>
      <EditRatingHistoryContent />
    </Suspense>
  );
}
