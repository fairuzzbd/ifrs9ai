import { ImpactMevPdForm } from "@/app/master/impact-mev-pd/_components/ImpactMevPdForm";
import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Tambah Impact MEV-PD — BLIPS IFRS9",
};

export default function NewImpactMevPdPage() {
  return (
    <div className="container mx-auto max-w-3xl py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <a href="/master/impact-mev-pd" className="hover:underline">
          Impact MEV-PD
        </a>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Tambah</span>
      </nav>
      <h1 className="text-2xl font-semibold">Tambah Impact MEV-PD</h1>
      <ImpactMevPdForm mode="create" />
    </div>
  );
}
