"use client";

/**
 * TransaksiTabs — client island for active-state highlight on the transaksi layout.
 * Renders a tab nav that highlights the currently active sub-route.
 * Each tab is a <Link prefetch> (server-routed, not a <button>).
 */

import * as React from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { cn } from "@/lib/utils";

export interface TransaksiTab {
  /** Display label (Bahasa Indonesia) */
  label: string;
  /** Route href, e.g. /transaksi/penempatan */
  href: string;
  /** Permission string required to see this tab */
  permission: string;
}

interface TransaksiTabsProps {
  /** Tabs whose permissions were satisfied (server filters before passing) */
  tabs: TransaksiTab[];
}

export function TransaksiTabs({ tabs }: TransaksiTabsProps) {
  const pathname = usePathname();

  return (
    <nav
      aria-label="Navigasi Transaksi"
      className="border-b bg-background"
    >
      <ul
        role="tablist"
        className="flex overflow-x-auto scrollbar-none px-6"
      >
        {tabs.map((tab) => {
          // Mark active if pathname starts with tab href
          const isActive =
            pathname === tab.href || pathname.startsWith(`${tab.href}/`);

          return (
            <li key={tab.href} role="presentation">
              <Link
                href={tab.href}
                prefetch
                role="tab"
                aria-selected={isActive}
                aria-current={isActive ? "page" : undefined}
                className={cn(
                  "inline-flex items-center whitespace-nowrap px-4 py-3 text-sm font-medium border-b-2 transition-colors focus:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2",
                  isActive
                    ? "border-primary text-primary"
                    : "border-transparent text-muted-foreground hover:text-foreground hover:border-muted-foreground",
                )}
              >
                {tab.label}
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
