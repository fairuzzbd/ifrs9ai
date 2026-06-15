"use client";

import * as React from "react";
import { Check, ChevronDown, Search, AlertTriangle } from "lucide-react";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";
import { WorkflowPathBadge } from "./WorkflowPathBadge";
import {
  EVENT_CODE_GROUPS,
  type EventCodeMeta,
  type KategoriEvent,
  type TriggerSource,
} from "@/lib/schemas/jurnal.schema";

export interface EventCodePickerProps {
  value: string | null;
  onChange: (code: string, meta: EventCodeMeta) => void;
  allowCustom?: boolean;
  disabled?: boolean;
  excludeApproved?: boolean;
  approvedCodes?: string[];
  placeholder?: string;
}

export function EventCodePicker({
  value,
  onChange,
  allowCustom = false,
  disabled = false,
  excludeApproved = false,
  approvedCodes = [],
  placeholder = "Pilih kode event...",
}: EventCodePickerProps) {
  const [open, setOpen] = React.useState(false);
  const [search, setSearch] = React.useState("");
  const searchRef = React.useRef<HTMLInputElement>(null);

  // Find current selection metadata
  const allMeta = EVENT_CODE_GROUPS.flatMap((g) => g.codes);
  const currentMeta = allMeta.find((m) => m.eventCode === value);

  // Filter groups by search
  const filteredGroups = EVENT_CODE_GROUPS.map((g) => ({
    ...g,
    codes: g.codes.filter(
      (c) =>
        c.eventCode.toLowerCase().includes(search.toLowerCase()) ||
        c.namaEvent.toLowerCase().includes(search.toLowerCase()),
    ),
  })).filter((g) => g.codes.length > 0);

  const handleSelect = (meta: EventCodeMeta) => {
    onChange(meta.eventCode, meta);
    setOpen(false);
    setSearch("");
  };

  const handleOpenChange = (v: boolean) => {
    setOpen(v);
    if (v) {
      setTimeout(() => searchRef.current?.focus(), 50);
    }
  };

  const isApproved = (code: string) => approvedCodes.includes(code);

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          aria-label="Pilih kode event jurnal"
          disabled={disabled}
          className="w-full justify-between font-normal"
        >
          {currentMeta ? (
            <span className="flex items-center gap-2">
              <span className="font-mono text-xs font-medium">
                {currentMeta.eventCode}
              </span>
              <span className="text-muted-foreground text-xs truncate">
                {currentMeta.namaEvent}
              </span>
              <WorkflowPathBadge path={currentMeta.workflowPath} size="sm" showTooltip={false} />
            </span>
          ) : (
            <span className="text-muted-foreground">{placeholder}</span>
          )}
          <ChevronDown className="ml-2 h-4 w-4 shrink-0 opacity-50" aria-hidden="true" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        className="w-[480px] p-0"
        align="start"
        onKeyDown={(e) => {
          if (e.key === "Escape") setOpen(false);
        }}
      >
        {/* Search input */}
        <div className="flex items-center border-b px-3 py-2">
          <Search className="mr-2 h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
          <Input
            ref={searchRef}
            placeholder="Cari kode atau nama event..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="border-0 p-0 shadow-none focus-visible:ring-0 text-sm"
            aria-label="Cari kode event"
          />
        </div>

        {/* List */}
        <div className="max-h-80 overflow-y-auto py-1" role="listbox" aria-label="Daftar kode event">
          {filteredGroups.length === 0 && (
            <p className="py-6 text-center text-sm text-muted-foreground">
              Tidak ada hasil
            </p>
          )}
          {filteredGroups.map((group) => (
            <div key={group.label}>
              <div className="px-3 py-1.5 text-xs font-semibold text-muted-foreground uppercase tracking-wide">
                {group.label}
              </div>
              {group.codes.map((meta) => {
                const selected = value === meta.eventCode;
                const alreadyApproved = isApproved(meta.eventCode);
                const excluded = excludeApproved && alreadyApproved;

                return (
                  <button
                    key={meta.eventCode}
                    role="option"
                    aria-selected={selected}
                    disabled={excluded}
                    className={cn(
                      "flex w-full items-center gap-2 px-3 py-2 text-left text-sm hover:bg-accent focus:bg-accent focus:outline-none",
                      selected && "bg-accent",
                      excluded && "opacity-40 cursor-not-allowed",
                    )}
                    onClick={() => !excluded && handleSelect(meta)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" || e.key === " ") {
                        e.preventDefault();
                        if (!excluded) handleSelect(meta);
                      }
                    }}
                  >
                    <Check
                      className={cn(
                        "h-4 w-4 shrink-0",
                        selected ? "opacity-100" : "opacity-0",
                      )}
                      aria-hidden="true"
                    />
                    <span className="flex flex-col min-w-0 flex-1">
                      <span className="font-mono text-xs font-semibold">
                        {meta.eventCode}
                      </span>
                      <span className="text-xs text-muted-foreground truncate">
                        {meta.namaEvent}
                      </span>
                    </span>
                    <WorkflowPathBadge path={meta.workflowPath} size="sm" showTooltip={false} />
                  </button>
                );
              })}
              {/* Warning for already-approved codes */}
              {!excludeApproved && group.codes.some((c) => isApproved(c.eventCode) && value === c.eventCode) && (
                <div className="mx-3 mb-2 flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 p-2 text-xs text-amber-800">
                  <AlertTriangle className="h-3.5 w-3.5 mt-0.5 shrink-0" aria-hidden="true" />
                  <span>
                    Kode ini sudah ter-approve. Membuat mapping baru akan membuat versi baru
                    dan mematikan versi lama setelah approval.
                  </span>
                </div>
              )}
            </div>
          ))}

          {allowCustom && (
            <div className="border-t px-3 py-2">
              <button
                className="flex w-full items-center gap-2 py-1.5 text-sm text-blue-600 hover:text-blue-700"
                onClick={() => {
                  const customMeta: EventCodeMeta = {
                    eventCode: search.toUpperCase().replace(/\s+/g, "_"),
                    namaEvent: search,
                    kategoriEvent: "KOREKSI" as KategoriEvent,
                    triggerSource: "USER_INPUT" as TriggerSource,
                    workflowPath: "4-eyes",
                    klasifikasiAllowed: [],
                    isRegulated: false,
                  };
                  onChange(customMeta.eventCode, customMeta);
                  setOpen(false);
                  setSearch("");
                }}
              >
                <span>+ Buat kode baru: &ldquo;{search || "..."}&rdquo;</span>
              </button>
            </div>
          )}
        </div>
      </PopoverContent>
    </Popover>
  );
}
