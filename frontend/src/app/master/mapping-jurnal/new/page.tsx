import { MappingJurnalForm } from "@/app/master/mapping-jurnal/_components/MappingJurnalForm";
import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Tambah Mapping Jurnal — BLIPS IFRS9",
};

export default function NewMappingJurnalPage() {
  return (
    <div className="container mx-auto max-w-4xl py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <a href="/master/mapping-jurnal" className="hover:underline">
          Mapping Jurnal
        </a>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Tambah Mapping Jurnal</span>
      </nav>
      <h1 className="text-2xl font-semibold">Tambah Mapping Jurnal</h1>
      <MappingJurnalForm mode="create" />
    </div>
  );
}
