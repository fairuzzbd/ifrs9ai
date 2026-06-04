import { z } from "zod";
import {
  ratingPefindoEnum,
  ratingOutlookEnum,
  actionTypeRatingEnum,
  type RatingPefindo,
  type RatingOutlook,
  type ActionTypeRating,
} from "@/lib/schemas/counterparty.schema";

// Re-export for convenience
export { ratingPefindoEnum, ratingOutlookEnum, actionTypeRatingEnum };
export type { RatingPefindo, RatingOutlook, ActionTypeRating };

// ---------------------------------------------------------------------------
// Create / update schemas
// ---------------------------------------------------------------------------

export const ratingHistoryCreateSchema = z.object({
  counterpartyId: z.string().uuid("ID counterparty tidak valid"),
  ratingPefindo: ratingPefindoEnum,
  ratingOutlook: ratingOutlookEnum,
  actionType: actionTypeRatingEnum,
  notchChange: z
    .number({ error: () => ({ message: "Notch change harus berupa angka" }) })
    .int("Notch change harus bilangan bulat")
    .min(-20, "Notch change terlalu kecil")
    .max(20, "Notch change terlalu besar"),
  tanggalBerlaku: z
    .string()
    .date("Format tanggal tidak valid (YYYY-MM-DD)"),
  tanggalBerakhir: z
    .string()
    .date("Format tanggal tidak valid (YYYY-MM-DD)")
    .optional()
    .or(z.literal("")),
  sumber: z.string().max(100).optional(),
  catatan: z.string().max(500).optional(),
});

export type RatingHistoryCreateInput = z.infer<typeof ratingHistoryCreateSchema>;

export const ratingHistoryUpdateSchema = ratingHistoryCreateSchema
  .omit({ counterpartyId: true })
  .extend({
    rowVersion: z.number().int().positive("rowVersion diperlukan untuk update"),
  });

export type RatingHistoryUpdateInput = z.infer<typeof ratingHistoryUpdateSchema>;

// ---------------------------------------------------------------------------
// API response type
// ---------------------------------------------------------------------------

export interface RatingHistoryItem {
  id: string;
  counterpartyId: string;
  counterpartyNama: string;
  counterpartyKode: string;
  ratingPefindo: RatingPefindo;
  ratingOutlook: RatingOutlook;
  actionType: ActionTypeRating;
  notchChange: number;
  sicrTriggered: boolean;      // read-only, auto-computed
  defaultTriggered: boolean;   // read-only, auto-computed
  tanggalBerlaku: string;
  tanggalBerakhir: string | null;
  sumber: string | null;
  catatan: string | null;
  rowVersion: number;
  createdAt: string;
  createdBy: string;
  updatedAt: string;
  updatedBy: string;
  deletedAt: string | null;
}

// ---------------------------------------------------------------------------
// Label maps (for display)
// ---------------------------------------------------------------------------

export const RATING_OUTLOOK_LABELS: Record<RatingOutlook, string> = {
  STABLE: "Stabil",
  POSITIVE: "Positif",
  NEGATIVE: "Negatif",
  WATCH_POSITIVE: "CreditWatch Positif",
  WATCH_NEGATIVE: "CreditWatch Negatif",
};

export const ACTION_TYPE_LABELS: Record<ActionTypeRating, string> = {
  INITIAL: "Inisial",
  UPGRADE: "Upgrade",
  DOWNGRADE: "Downgrade",
  AFFIRM: "Afirmasi",
  WATCH: "CreditWatch",
  WITHDRAW: "Ditarik",
};
