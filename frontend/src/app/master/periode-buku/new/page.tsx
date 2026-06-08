import { PeriodeBukuForm } from "@/app/master/periode-buku/_components/PeriodeBukuForm";
import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = {
  title: "Buat Periode Buku — BLIPS IFRS9",
};

export default function NewPeriodeBukuPage() {
  return (
    <div className="container mx-auto max-w-2xl py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/periode-buku" className="hover:underline">
          Periode Buku
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Buat Periode</span>
      </nav>
      <h1 className="text-2xl font-semibold">Buat Periode Buku</h1>
      <PeriodeBukuForm mode="create" />
    </div>
  );
}
