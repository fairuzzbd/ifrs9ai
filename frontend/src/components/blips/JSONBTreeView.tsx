"use client";

import * as React from "react";
import { ChevronRight, ChevronDown } from "lucide-react";
import { cn } from "@/lib/utils";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface JSONBTreeViewProps {
  data: Record<string, unknown>;
  maxDepth?: number;
  initiallyExpanded?: boolean;
  className?: string;
}

// ---------------------------------------------------------------------------
// Node
// ---------------------------------------------------------------------------

interface NodeProps {
  nodeKey: string;
  value: unknown;
  depth: number;
  maxDepth: number;
}

function JSONNode({ nodeKey, value, depth, maxDepth }: NodeProps) {
  const [expanded, setExpanded] = React.useState(depth < 1);

  const isObject =
    value !== null && typeof value === "object" && !Array.isArray(value);
  const isArray = Array.isArray(value);
  const isComplex = isObject || isArray;

  const toggleExpand = () => setExpanded((e) => !e);

  const renderPrimitive = () => {
    if (value === null) return <span className="text-gray-400">null</span>;
    if (typeof value === "boolean")
      return (
        <span className={value ? "text-green-700" : "text-red-600"}>
          {String(value)}
        </span>
      );
    if (typeof value === "number")
      return <span className="text-blue-700">{value}</span>;
    return <span className="text-amber-700">&quot;{String(value)}&quot;</span>;
  };

  if (depth >= maxDepth && isComplex) {
    return (
      <div className="flex gap-1 text-xs">
        <span className="text-muted-foreground">{nodeKey}:</span>
        <span className="text-gray-400 italic">[Object]</span>
      </div>
    );
  }

  return (
    <div className={cn("text-xs", depth > 0 && "ml-4")}>
      {isComplex ? (
        <div>
          <button
            type="button"
            onClick={toggleExpand}
            className="inline-flex items-center gap-0.5 hover:bg-muted rounded px-0.5 focus-visible:ring-1 focus-visible:ring-ring"
            aria-expanded={expanded}
            aria-label={`${expanded ? "Tutup" : "Buka"} ${nodeKey}`}
          >
            {expanded ? (
              <ChevronDown className="h-3 w-3 text-muted-foreground" aria-hidden="true" />
            ) : (
              <ChevronRight className="h-3 w-3 text-muted-foreground" aria-hidden="true" />
            )}
            <span className="text-muted-foreground">{nodeKey}</span>
            <span className="text-gray-400 ml-0.5">
              {isArray ? `[${(value as unknown[]).length}]` : "{…}"}
            </span>
          </button>
          {expanded && (
            <div>
              {isArray
                ? (value as unknown[]).map((item, i) => (
                    <JSONNode
                      key={i}
                      nodeKey={String(i)}
                      value={item}
                      depth={depth + 1}
                      maxDepth={maxDepth}
                    />
                  ))
                : Object.entries(value as Record<string, unknown>).map(
                    ([k, v]) => (
                      <JSONNode
                        key={k}
                        nodeKey={k}
                        value={v}
                        depth={depth + 1}
                        maxDepth={maxDepth}
                      />
                    ),
                  )}
            </div>
          )}
        </div>
      ) : (
        <div className="flex gap-1">
          <span className="text-muted-foreground">{nodeKey}:</span>
          {renderPrimitive()}
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function JSONBTreeView({
  data,
  maxDepth = 3,
  initiallyExpanded = false,
  className,
}: JSONBTreeViewProps) {
  const [rootExpanded, setRootExpanded] = React.useState(initiallyExpanded);

  const entries = Object.entries(data);

  if (entries.length === 0) {
    return (
      <span className="text-xs text-muted-foreground italic">
        (tidak ada evidence)
      </span>
    );
  }

  return (
    <div
      className={cn(
        "font-mono border rounded p-2 bg-muted/20 text-xs max-w-sm",
        className,
      )}
    >
      <button
        type="button"
        onClick={() => setRootExpanded((e) => !e)}
        className="inline-flex items-center gap-0.5 text-xs text-muted-foreground hover:bg-muted rounded px-0.5 focus-visible:ring-1 focus-visible:ring-ring"
        aria-expanded={rootExpanded}
        aria-label={`${rootExpanded ? "Tutup" : "Buka"} tree view`}
      >
        {rootExpanded ? (
          <ChevronDown className="h-3 w-3" aria-hidden="true" />
        ) : (
          <ChevronRight className="h-3 w-3" aria-hidden="true" />
        )}
        <span>{`{${entries.length} key${entries.length !== 1 ? "s" : ""}}`}</span>
      </button>

      {rootExpanded && (
        <div className="mt-1 ml-2">
          {entries.map(([k, v]) => (
            <JSONNode
              key={k}
              nodeKey={k}
              value={v}
              depth={0}
              maxDepth={maxDepth}
            />
          ))}
        </div>
      )}
    </div>
  );
}
