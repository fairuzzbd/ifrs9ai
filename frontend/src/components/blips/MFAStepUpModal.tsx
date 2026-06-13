"use client";

import * as React from "react";
import { Loader2, XCircle } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { apiPost } from "@/lib/api";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface MFAStepUpModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  onVerified: (stepUpToken: string) => void;
  onCancel?: () => void;
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function MFAStepUpModal({
  open,
  onOpenChange,
  title,
  description,
  onVerified,
  onCancel,
}: MFAStepUpModalProps) {
  const [digits, setDigits] = React.useState<string[]>(Array(6).fill(""));
  const [error, setError] = React.useState<string | null>(null);
  const [attemptsLeft, setAttemptsLeft] = React.useState<number>(3);
  const [loading, setLoading] = React.useState(false);

  const inputRefs = React.useRef<Array<HTMLInputElement | null>>([]);

  // Auto-focus first digit when modal opens
  React.useEffect(() => {
    if (open) {
      setDigits(Array(6).fill(""));
      setError(null);
      setAttemptsLeft(3);
      setTimeout(() => inputRefs.current[0]?.focus(), 50);
    }
  }, [open]);

  const handleDigitChange = (index: number, value: string) => {
    // Accept only single digit
    const digit = value.replace(/\D/g, "").slice(-1);
    const next = [...digits];
    next[index] = digit;
    setDigits(next);

    // Auto-advance
    if (digit && index < 5) {
      inputRefs.current[index + 1]?.focus();
    }

    // Auto-submit when all 6 filled
    if (digit && index === 5) {
      const code = [...next.slice(0, 5), digit].join("");
      if (code.length === 6) {
        void submitMFA(code);
      }
    }
  };

  const handleKeyDown = (index: number, e: React.KeyboardEvent) => {
    if (e.key === "Backspace") {
      if (!digits[index] && index > 0) {
        inputRefs.current[index - 1]?.focus();
      }
      const next = [...digits];
      next[index] = "";
      setDigits(next);
    }
  };

  const handlePaste = (e: React.ClipboardEvent) => {
    e.preventDefault();
    const text = e.clipboardData.getData("text").replace(/\D/g, "").slice(0, 6);
    if (text.length === 6) {
      setDigits(text.split(""));
      void submitMFA(text);
    }
  };

  const submitMFA = async (code: string) => {
    setLoading(true);
    setError(null);
    try {
      const res = await apiPost<{ data: { stepUpToken: string } }>(
        "/api/v1/auth/step-up",
        { code, method: "TOTP" },
      );
      onVerified(res.data.stepUpToken);
      onOpenChange(false);
    } catch {
      const left = attemptsLeft - 1;
      setAttemptsLeft(left);
      setError(
        left > 0
          ? `Kode salah. Sisa percobaan: ${left}.`
          : "Kode salah. Anda telah melebihi batas percobaan. Silakan minta kode baru.",
      );
      setDigits(Array(6).fill(""));
      setTimeout(() => inputRefs.current[0]?.focus(), 50);
    } finally {
      setLoading(false);
    }
  };

  const handleSubmit = () => {
    const code = digits.join("");
    if (code.length !== 6) return;
    void submitMFA(code);
  };

  const handleCancel = () => {
    onOpenChange(false);
    onCancel?.();
  };

  const errorId = "mfa-error";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-sm" aria-describedby={description ? "mfa-desc" : undefined}>
        <DialogHeader>
          <DialogTitle>Verifikasi MFA Step-Up</DialogTitle>
          {description && (
            <DialogDescription id="mfa-desc">{description}</DialogDescription>
          )}
        </DialogHeader>

        <div className="space-y-4 py-2">
          <p className="text-sm font-medium">{title}</p>

          <div>
            <label className="text-sm text-muted-foreground mb-2 block">
              Kode TOTP (6 digit)
            </label>
            {/* OTP Input row */}
            <div
              className="flex gap-2 justify-center"
              onPaste={handlePaste}
              role="group"
              aria-label="Input kode OTP"
            >
              {digits.map((d, i) => (
                <Input
                  key={i}
                  ref={(el) => {
                    inputRefs.current[i] = el;
                  }}
                  value={d}
                  onChange={(e) => handleDigitChange(i, e.target.value)}
                  onKeyDown={(e) => handleKeyDown(i, e)}
                  maxLength={1}
                  inputMode="numeric"
                  autoComplete={i === 0 ? "one-time-code" : "off"}
                  pattern="[0-9]*"
                  aria-label={`Kode OTP digit ${i + 1}`}
                  aria-describedby={error ? errorId : undefined}
                  aria-invalid={!!error}
                  className="w-10 h-12 text-center text-lg font-mono"
                  disabled={loading}
                />
              ))}
            </div>

            {error && (
              <p
                id={errorId}
                role="alert"
                aria-live="polite"
                className="text-sm text-destructive mt-2 flex items-center gap-1"
              >
                <XCircle className="h-4 w-4 flex-shrink-0" aria-hidden="true" />
                {error}
              </p>
            )}
          </div>
        </div>

        <div className="flex justify-end gap-2">
          <Button
            variant="outline"
            onClick={handleCancel}
            disabled={loading}
          >
            Batal
          </Button>
          <Button
            onClick={handleSubmit}
            disabled={loading || digits.join("").length !== 6}
          >
            {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />}
            Verifikasi
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

// ---------------------------------------------------------------------------
// Hook: step-up token management (freshness check per DEC-027, 5-min TTL)
// ---------------------------------------------------------------------------

interface StepUpState {
  token: string | null;
  expiresAt: number | null; // unix ms
}

let _stepUpState: StepUpState = { token: null, expiresAt: null };

export function getStepUpToken(): string | null {
  if (!_stepUpState.token || !_stepUpState.expiresAt) return null;
  if (Date.now() > _stepUpState.expiresAt) {
    _stepUpState = { token: null, expiresAt: null };
    return null;
  }
  return _stepUpState.token;
}

export function setStepUpToken(token: string, ttlSeconds = 300) {
  _stepUpState = { token, expiresAt: Date.now() + ttlSeconds * 1000 };
}

export function clearStepUpToken() {
  _stepUpState = { token: null, expiresAt: null };
}

export function isMFAFresh(): boolean {
  return getStepUpToken() !== null;
}
