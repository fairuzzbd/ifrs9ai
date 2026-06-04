import { CounterpartyForm } from "@/app/master/counterparty/_components/CounterpartyForm";
import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Tambah Counterparty — BLIPS IFRS9",
};

export default function NewCounterpartyPage() {
  return (
    <div className="container mx-auto max-w-3xl py-6 space-y-4">
      <nav aria-label="Breadcrumb" className="text-sm text-muted-foreground">
        <a href="/master/counterparty" className="hover:underline">Counterparty</a>
        <span className="mx-1.5">/</span>
        <span className="text-foreground font-medium">Tambah Counterparty</span>
      </nav>
      <h1 className="text-2xl font-semibold">Tambah Counterparty</h1>
      <CounterpartyForm mode="create" />
    </div>
  );
}
