import { MataUangForm } from "@/app/master/mata-uang/_components/MataUangForm";
import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Tambah Mata Uang — BLIPS IFRS9",
};

export default function NewMataUangPage() {
  return (
    <div className="container mx-auto max-w-2xl py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <a href="/master/mata-uang" className="hover:underline">
          Mata Uang
        </a>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Tambah Mata Uang</span>
      </nav>
      <h1 className="text-2xl font-semibold">Tambah Mata Uang</h1>
      <MataUangForm mode="create" />
    </div>
  );
}
