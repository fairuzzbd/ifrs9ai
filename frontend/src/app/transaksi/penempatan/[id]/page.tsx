/**
 * /transaksi/penempatan/[id] — Server Component permission gate.
 * Requires: transaksi.penempatan.read
 */
import { requirePermission } from "@/lib/auth/permissions";
import PenempatanDetailClient from "./_page-client";

export const metadata = { title: "Detail Penempatan — BLIPS IFRS9" };

interface PageProps {
  params: { id: string };
}

export default async function PenempatanDetailPage({ params }: PageProps) {
  await requirePermission("transaksi.penempatan.read");
  return <PenempatanDetailClient params={params} />;
}
