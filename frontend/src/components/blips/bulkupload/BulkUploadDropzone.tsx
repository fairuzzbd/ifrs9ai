"use client";

import * as React from "react";
import { Upload, FileSpreadsheet, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

const MAX_SIZE_MB = 50;
const MAX_BYTES = MAX_SIZE_MB * 1024 * 1024;
const ACCEPTED_MIME = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet";
const REQUIRED_SHEETS = ["Deposito", "Obligasi", "Saham", "Reksadana", "Tabungan_Cash"];

export interface BulkUploadDropzoneProps {
  onFileSelected: (file: File) => void;
  disabled?: boolean;
  className?: string;
}

export function BulkUploadDropzone({ onFileSelected, disabled, className }: BulkUploadDropzoneProps) {
  const inputRef = React.useRef<HTMLInputElement>(null);
  const [selectedFile, setSelectedFile] = React.useState<File | null>(null);
  const [clientError, setClientError] = React.useState<string | null>(null);
  const [isDragging, setIsDragging] = React.useState(false);

  function validateAndSet(file: File) {
    setClientError(null);

    // Client-side hint: size check (server re-checks)
    if (file.size > MAX_BYTES) {
      setClientError(`Ukuran file ${(file.size / 1024 / 1024).toFixed(1)}MB melebihi batas ${MAX_SIZE_MB}MB.`);
      return;
    }

    // Client-side hint: extension check (server validates magic bytes)
    if (!file.name.toLowerCase().endsWith(".xlsx")) {
      setClientError("Hanya file .xlsx yang diterima. Server juga memvalidasi signature XLSX (ZIP magic bytes).");
      return;
    }

    setSelectedFile(file);
    onFileSelected(file);
  }

  function handleInputChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (file) validateAndSet(file);
  }

  function handleDrop(e: React.DragEvent<HTMLDivElement>) {
    e.preventDefault();
    setIsDragging(false);
    const file = e.dataTransfer.files?.[0];
    if (file) validateAndSet(file);
  }

  function handleDragOver(e: React.DragEvent<HTMLDivElement>) {
    e.preventDefault();
    setIsDragging(true);
  }

  function handleDragLeave() {
    setIsDragging(false);
  }

  function clearFile() {
    setSelectedFile(null);
    setClientError(null);
    if (inputRef.current) inputRef.current.value = "";
  }

  return (
    <div className={cn("space-y-3", className)}>
      <div
        role="button"
        tabIndex={disabled ? -1 : 0}
        aria-label="Area upload file XLSX instrumen bulk"
        aria-disabled={disabled}
        onDrop={handleDrop}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onClick={() => !disabled && inputRef.current?.click()}
        onKeyDown={(e) => { if (!disabled && (e.key === "Enter" || e.key === " ")) inputRef.current?.click(); }}
        className={cn(
          "border-2 border-dashed rounded-lg p-8 text-center transition-colors cursor-pointer",
          isDragging ? "border-primary bg-primary/5" : "border-muted-foreground/30 hover:border-primary/50",
          disabled && "opacity-50 cursor-not-allowed",
          selectedFile && "border-green-400 bg-green-50",
        )}
      >
        {selectedFile ? (
          <div className="flex items-center justify-center gap-3">
            <FileSpreadsheet className="h-8 w-8 text-green-600" aria-hidden="true" />
            <div className="text-left">
              <p className="font-medium text-sm">{selectedFile.name}</p>
              <p className="text-xs text-muted-foreground">
                {(selectedFile.size / 1024 / 1024).toFixed(2)}MB
              </p>
            </div>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              onClick={(e) => { e.stopPropagation(); clearFile(); }}
              aria-label="Hapus file yang dipilih"
              className="h-6 w-6"
            >
              <X className="h-4 w-4" />
            </Button>
          </div>
        ) : (
          <div className="space-y-2">
            <Upload className="h-10 w-10 mx-auto text-muted-foreground" aria-hidden="true" />
            <p className="text-sm font-medium">Seret dan lepas file XLSX di sini, atau klik untuk pilih</p>
            <p className="text-xs text-muted-foreground">Maksimal {MAX_SIZE_MB}MB · Format XLSX 5-sheet</p>
          </div>
        )}
      </div>

      <input
        ref={inputRef}
        type="file"
        accept={ACCEPTED_MIME + ",.xlsx"}
        onChange={handleInputChange}
        className="sr-only"
        aria-label="Input file XLSX"
        disabled={disabled}
      />

      {clientError && (
        <p role="alert" className="text-sm text-destructive">{clientError}</p>
      )}

      <div className="text-xs text-muted-foreground space-y-1">
        <p className="font-medium">Sheet yang dibutuhkan (5 sheet wajib):</p>
        <ul className="flex flex-wrap gap-2">
          {REQUIRED_SHEETS.map((sheet) => (
            <li key={sheet} className="bg-slate-100 rounded px-2 py-0.5 font-mono">{sheet}</li>
          ))}
        </ul>
      </div>
    </div>
  );
}
