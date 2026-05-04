import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { Loader2, ShieldAlert, ShieldCheck } from "lucide-react";

import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";

import EmptyState from "../../../../components/EmptyState";
import { palette, radius, space } from "../../../../layout/theme";
import { capabilityMeta, sortCapabilities } from "../../../../lib/capabilities";
import { listSystemPlugins } from "../../../../lib/api/system_plugins";
import {
    BASELINE_PRESETS,
    BaselinePreset,
    matchingPreset,
} from "../../../../lib/baselinePresets";
import { humanizeError } from "../../../../lib/humanizeError";

interface Props {
    /** Plugin IDs the operator has ticked so far. */
    selected: string[];
    onChange: (next: string[]) => void;
}

// BaselinePluginsStep is the wizard's per-enrollment plugin picker.
// Source of truth is the server's system-plugin catalog
// (<data-dir>/system-plugins/), populated by the agent-publisher in
// dev mode and seeded manually in production. We deliberately don't
// fall back to the marketplace catalog: marketplace plugins are
// signed by a different publisher key and run in the post-enroll
// flow, never the install bundle.
//
// Contract:
//   - Defaults to nothing selected. The requirements thread asked
//     for "minimal default + operator opts in", so we never
//     pre-select anything (the agent's mandatory-core merge in
//     cmd/platypus-agent picks up sys-info regardless).
//   - Selection is just a list of plugin IDs; the install bundle
//     carries them forward and the agent's allowlist filter applies
//     at first boot.
export default function BaselinePluginsStep({ selected, onChange }: Props) {
    const plugins = useQuery({
        queryKey: ["enroll", "system-plugins-pool"],
        queryFn: () => listSystemPlugins(),
        refetchOnWindowFocus: false,
    });

    function toggle(id: string) {
        const set = new Set(selected);
        if (set.has(id)) set.delete(id);
        else set.add(id);
        onChange([...set]);
    }

    if (plugins.isLoading) {
        return (
            <div
                data-testid="enroll-wizard-baseline-plugins"
                style={{ display: "flex", justifyContent: "center", padding: space[4] }}
            >
                <Loader2 className="size-5 animate-spin" />
            </div>
        );
    }

    if (plugins.error) {
        return (
            <div data-testid="enroll-wizard-baseline-plugins">
                <EmptyState
                    title="Couldn't load system plugins"
                    description={humanizeError(plugins.error)}
                />
            </div>
        );
    }

    const list = plugins.data ?? [];
    if (list.length === 0) {
        return (
            <div data-testid="enroll-wizard-baseline-plugins">
                <EmptyState
                    title="No system plugins available"
                    description="The server hasn't been seeded with system plugins yet. In dev, the agent-publisher sidecar populates this on the first `docker compose up`. In production, your operator needs to stage signed bundles under <data-dir>/system-plugins/."
                />
            </div>
        );
    }

    return (
        <div
            data-testid="enroll-wizard-baseline-plugins"
            style={{
                display: "flex",
                flexDirection: "column",
                gap: space[2],
            }}
        >
            <p
                style={{
                    margin: 0,
                    fontSize: 12,
                    color: palette.textMuted,
                }}
            >
                Plugins to auto-install on first boot. Pick a preset that
                matches the host's role, or fine-tune the individual plugins
                below. Anything you skip can still be added later from the
                host's Plugins tab.
            </p>
            <PresetGrid
                catalogIDs={new Set(list.map((p) => p.id))}
                selected={selected}
                onPick={(p) =>
                    onChange(
                        p.pluginIDs.filter((id) =>
                            list.some((catalog) => catalog.id === id),
                        ),
                    )
                }
            />
            <ScrollArea className="max-h-[40vh] pr-1">
                <ul
                    style={{
                        listStyle: "none",
                        padding: 0,
                        margin: 0,
                        display: "flex",
                        flexDirection: "column",
                        gap: space[1],
                    }}
                >
                    {list.map((p) => {
                        const isSel = selected.includes(p.id);
                        const caps = sortCapabilities(
                            p.capabilities.map((f) => ({ family: f })),
                        );
                        const hasHigh = caps.some(
                            (c) => capabilityMeta(c.family).risk === "high",
                        );
                        return (
                            <li
                                key={p.id}
                                style={{
                                    display: "flex",
                                    gap: space[2],
                                    border: `1px solid ${palette.border}`,
                                    borderRadius: radius.md,
                                    padding: space[3],
                                    background: palette.surface,
                                    alignItems: "flex-start",
                                }}
                            >
                                <Checkbox
                                    id={`baseline-${p.id}`}
                                    aria-label={p.name}
                                    checked={isSel}
                                    onCheckedChange={() => toggle(p.id)}
                                    className="mt-1"
                                />
                                <div
                                    style={{
                                        display: "flex",
                                        flexDirection: "column",
                                        gap: 2,
                                        flex: 1,
                                        minWidth: 0,
                                    }}
                                >
                                    <Label
                                        htmlFor={`baseline-${p.id}`}
                                        className="text-sm font-medium cursor-pointer"
                                    >
                                        <span style={{ display: "flex", gap: space[2], alignItems: "center" }}>
                                            {p.name}
                                            <span
                                                style={{
                                                    fontSize: 11,
                                                    color: palette.textMuted,
                                                }}
                                            >
                                                v{p.version}
                                            </span>
                                            {hasHigh ? (
                                                <ShieldAlert className="size-3 text-red-600" />
                                            ) : (
                                                <ShieldCheck className="size-3 text-emerald-600" />
                                            )}
                                        </span>
                                    </Label>
                                    <span
                                        style={{
                                            fontSize: 11,
                                            color: palette.textMuted,
                                            fontFamily: "monospace",
                                        }}
                                    >
                                        {p.id}
                                    </span>
                                    {p.description && (
                                        <span
                                            style={{
                                                fontSize: 12,
                                                color: palette.textSecondary,
                                            }}
                                        >
                                            {p.description}
                                        </span>
                                    )}
                                    {caps.length > 0 && (
                                        <div
                                            style={{
                                                display: "flex",
                                                flexWrap: "wrap",
                                                gap: 4,
                                                marginTop: 2,
                                            }}
                                        >
                                            {caps.map(({ family }) => (
                                                <span
                                                    key={family}
                                                    style={{
                                                        fontSize: 10,
                                                        background: palette.surfaceHover,
                                                        color: palette.textPrimary,
                                                        padding: "2px 6px",
                                                        borderRadius: 4,
                                                        fontFamily: "monospace",
                                                    }}
                                                >
                                                    {family}
                                                </span>
                                            ))}
                                        </div>
                                    )}
                                </div>
                            </li>
                        );
                    })}
                </ul>
            </ScrollArea>
            <p style={{ fontSize: 11, color: palette.textMuted, margin: 0 }}>
                {selected.length === 0
                    ? "No baseline plugins selected — agent will boot empty."
                    : `${selected.length} plugin${selected.length === 1 ? "" : "s"} selected.`}
            </p>
        </div>
    );
}

// PresetGrid is the row of "preset" cards above the per-plugin
// list. Each card shows what the preset bundles in plain English;
// clicking it sets the operator's selection to the preset's
// plugin IDs (filtered to what the catalog actually offers, so a
// preset referencing a plugin the server isn't shipping yet just
// drops it instead of leaving the operator with a stuck "Install"
// step downstream).
function PresetGrid({
    catalogIDs,
    selected,
    onPick,
}: {
    catalogIDs: Set<string>;
    selected: string[];
    onPick: (p: BaselinePreset) => void;
}) {
    const active = useMemo(
        () => matchingPreset(selected, catalogIDs),
        [selected, catalogIDs],
    );
    return (
        <div
            role="group"
            aria-label="baseline plugin presets"
            style={{
                display: "grid",
                gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))",
                gap: space[2],
                margin: `${space[2]}px 0 ${space[3]}px`,
            }}
        >
            {BASELINE_PRESETS.map((p) => {
                const isActive = active?.id === p.id;
                const declaredCount = p.pluginIDs.filter((id) => catalogIDs.has(id)).length;
                const totalCount = p.pluginIDs.length;
                const missing = totalCount - declaredCount;
                return (
                    <button
                        key={p.id}
                        type="button"
                        aria-pressed={isActive}
                        onClick={() => onPick(p)}
                        style={{
                            textAlign: "left",
                            padding: space[3],
                            borderRadius: radius.md,
                            border: `1px solid ${isActive ? palette.accent : palette.border}`,
                            background: isActive ? palette.surfaceHover : palette.surface,
                            color: palette.textPrimary,
                            cursor: "pointer",
                            display: "flex",
                            flexDirection: "column",
                            gap: 4,
                        }}
                    >
                        <span style={{ fontSize: 13, fontWeight: 600 }}>{p.label}</span>
                        <p
                            style={{
                                margin: 0,
                                fontSize: 11,
                                color: palette.textSecondary,
                                lineHeight: 1.4,
                            }}
                        >
                            {p.summary}
                        </p>
                        <span
                            style={{
                                marginTop: 4,
                                fontSize: 10,
                                color: palette.textMuted,
                            }}
                        >
                            {totalCount === 0
                                ? "Empty selection"
                                : missing > 0
                                ? `${declaredCount} of ${totalCount} plugins available`
                                : `${declaredCount} plugin${declaredCount === 1 ? "" : "s"}`}
                        </span>
                    </button>
                );
            })}
        </div>
    );
}
