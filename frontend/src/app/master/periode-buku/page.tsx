// This route has been moved to /periode-buku
// next.config.js handles the 308 redirect — this file is a no-op fallback.
import { redirect } from "next/navigation";

export default function PeriodeBukuMovedPage() {
  redirect("/periode-buku");
}
