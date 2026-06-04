import { Suspense } from "react";
import { RatingHistoryForm } from "@/app/master/rating-history/_components/RatingHistoryForm";
import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Tambah Rating History — BLIPS IFRS9",
};

function NewRatingHistoryContent() {
  return (
    <div className="container mx-auto max-w-3xl py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <a href="/master/rating-history" className="hover:underline">Rating History</a>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Tambah Rating</span>
      </nav>
      <h1 className="text-2xl font-semibold">Tambah Rating History</h1>
      <RatingHistoryForm mode="create" />
    </div>
  );
}

export default function NewRatingHistoryPage() {
  return (
    <Suspense>
      <NewRatingHistoryContent />
    </Suspense>
  );
}
