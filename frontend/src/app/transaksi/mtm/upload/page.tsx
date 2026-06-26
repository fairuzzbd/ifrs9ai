/**
 * /transaksi/mtm/upload — Server Component permission gate.
 * Requires: transaksi.mtm.upload
 */
import { requirePermission } from "@/lib/auth/permissions";
import MtmUploadClient from "./_page-client";

export const metadata = { title: "Upload MTM — BLIPS IFRS9" };

export default async function MtmUploadPage() {
  await requirePermission("transaksi.mtm.upload");
  return <MtmUploadClient />;
}
