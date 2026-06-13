"use client";

import * as React from "react";
import { CheckCircle2, XCircle } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";
import type { SolverMetadata } from "@/lib/schemas/eir.schema";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface NewtonRaphsonSolverPanelProps {
  solverMetadata: SolverMetadata | null | undefined;
}

// ---------------------------------------------------------------------------
// Convergence mini-chart (pure CSS/SVG — no recharts needed)
// ---------------------------------------------------------------------------

function ConvergenceChart({
  path,
}: {
  path: number[];
}) {
  if (!path || path.length < 2) return null;

  const max = Math.max(...path);
  const min = Math.min(...path);
  const range = max - min || 1;

  const W = 200;
  const H = 60;
  const pts = path.map((v, i) => {
    const x = (i / (path.length - 1)) * W;
    // Invert y: lower residual = higher on chart looks better
    const normalized = (v - min) / range;
    const y = H - normalized * H;
    return `${x},${y}`;
  });

  return (
    <div className="mt-2">
      <p className="text-xs text-muted-foreground mb-1">Kurva Konvergensi (Residual)</p>
      <svg
        viewBox={`0 0 ${W} ${H}`}
        className="w-full h-16 border rounded bg-muted/20"
        aria-label="Grafik konvergensi solver EIR"
        role="img"
      >
        <polyline
          points={pts.join(" ")}
          fill="none"
          stroke="hsl(var(--primary))"
          strokeWidth="1.5"
        />
      </svg>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Panel
// ---------------------------------------------------------------------------

export function NewtonRaphsonSolverPanel({
  solverMetadata,
}: NewtonRaphsonSolverPanelProps) {
  if (!solverMetadata) {
    return (
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">Solver EIR</CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-xs text-muted-foreground italic">
            Metadata solver tidak tersedia (compute lama).
          </p>
        </CardContent>
      </Card>
    );
  }

  const { iterations, maxIterations, finalResidual, converged, precision, convergencePath } =
    solverMetadata;

  return (
    <Card
      className={cn(
        "border-l-4",
        converged ? "border-l-green-500" : "border-l-destructive",
      )}
    >
      <CardHeader className="pb-2">
        <CardTitle className="text-sm flex items-center gap-2">
          {converged ? (
            <CheckCircle2 className="h-4 w-4 text-green-600" aria-hidden="true" />
          ) : (
            <XCircle className="h-4 w-4 text-destructive" aria-hidden="true" />
          )}
          Solver EIR (Newton-Raphson)
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-2 text-sm">
        <div className="flex justify-between">
          <span className="text-muted-foreground">Status</span>
          <span
            className={cn(
              "font-medium",
              converged ? "text-green-700" : "text-destructive",
            )}
          >
            {converged ? "Konvergen" : "Tidak Konvergen"}
          </span>
        </div>
        <div className="flex justify-between">
          <span className="text-muted-foreground">Iterasi</span>
          <span>
            {iterations} dari maks {maxIterations}
          </span>
        </div>
        <div className="flex justify-between">
          <span className="text-muted-foreground">Residual Akhir</span>
          <span className="font-mono text-xs">{finalResidual}</span>
        </div>
        <div className="flex justify-between">
          <span className="text-muted-foreground">Presisi</span>
          <span className="text-xs">{precision}</span>
        </div>

        {convergencePath && convergencePath.length > 0 && (
          <ConvergenceChart path={convergencePath} />
        )}
      </CardContent>
    </Card>
  );
}
