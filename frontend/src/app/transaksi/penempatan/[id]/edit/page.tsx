/**
 * /transaksi/penempatan/[id]/edit — Server Component permission gate.
 * Requires: transaksi.penempatan.update
 */
import { requirePermission } from "@/lib/auth/permissions";
import PenempatanEditClient from "./_page-client";

export const metadata = { title: "Edit Penempatan — BLIPS IFRS9" };

interface PageProps {
  params: { id: string };
}

export default async function PenempatanEditPage({ params }: PageProps) {
  await requirePermission("transaksi.penempatan.update");
  return <PenempatanEditClient params={params} />;
}
