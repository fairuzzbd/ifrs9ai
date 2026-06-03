import { ImpactPdForm } from "@/app/master/impact-pd/_components/ImpactPdForm";
import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Tambah Impact PD — BLIPS IFRS9",
};

export default function NewImpactPdPage() {
  return (
    <div className="container mx-auto max-w-2xl py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <a href="/master/impact-pd" className="hover:underline">
          Impact PD
        </a>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Tambah</span>
      </nav>
      <h1 className="text-2xl font-semibold">Tambah Impact PD</h1>
      <ImpactPdForm mode="create" />
    </div>
  );
}
