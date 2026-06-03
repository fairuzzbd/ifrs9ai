"use client";

import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Toaster } from "sonner";
import { NuqsAdapter } from "nuqs/adapters/next/app";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: (failureCount, error) => {
        // Do not retry on 4xx errors
        if (
          error &&
          typeof error === "object" &&
          "status" in error &&
          typeof (error as { status: number }).status === "number"
        ) {
          const status = (error as { status: number }).status;
          if (status >= 400 && status < 500) return false;
        }
        return failureCount < 2;
      },
    },
  },
});

export function Providers({ children }: { children: React.ReactNode }) {
  return (
    <NuqsAdapter>
      <QueryClientProvider client={queryClient}>
        {children}
        <Toaster
          position="top-right"
          richColors
          closeButton
          toastOptions={{
            classNames: {
              error: "text-sm",
              success: "text-sm",
            },
          }}
        />
      </QueryClientProvider>
    </NuqsAdapter>
  );
}
