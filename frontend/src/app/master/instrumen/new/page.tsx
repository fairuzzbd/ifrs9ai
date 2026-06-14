import { InstrumenForm } from "@/app/master/instrumen/_components/InstrumenForm";
import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Tambah Instrumen — BLIPS IFRS9",
};

export default function NewInstrumenPage() {
  return (
    <div className="container mx-auto max-w-3xl py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <a href="/master/instrumen" className="hover:underline">
          Instrumen
        </a>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Tambah Instrumen</span>
      </nav>
      <h1 className="text-2xl font-semibold">Tambah Instrumen</h1>
      <InstrumenForm mode="create" />
    </div>
  );
}
