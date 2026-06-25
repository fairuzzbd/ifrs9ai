/**
 * /transaksi/jatuh-tempo — Server Component permission gate.
 * Requires: transaksi.jatuhtempo.read
 */
import { requirePermission } from "@/lib/auth/permissions";
import JatuhTempoListClient from "./_page-client";

export const metadata = { title: "Jatuh Tempo — BLIPS IFRS9" };

export default async function JatuhTempoListPage() {
  await requirePermission("transaksi.jatuhtempo.read");
  return <JatuhTempoListClient />;
}
