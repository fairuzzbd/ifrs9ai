import { KursForm } from "@/app/master/kurs/_components/KursForm";
import type { Metadata } from "next";
import Link from "next/link";

export const metadata: Metadata = {
  title: "Tambah Kurs — BLIPS IFRS9",
};

export default function NewKursPage() {
  return (
    <div className="container mx-auto max-w-3xl py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <Link href="/master/kurs" className="hover:underline">
          Kurs
        </Link>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Tambah Kurs</span>
      </nav>
      <h1 className="text-2xl font-semibold">Tambah Kurs</h1>
      <KursForm mode="create" />
    </div>
  );
}
