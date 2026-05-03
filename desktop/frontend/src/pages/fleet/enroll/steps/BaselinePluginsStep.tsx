import { useQuery } from "@tanstack/react-query";
import { Loader2, ShieldAlert, ShieldCheck } from "lucide-react";

import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";

import EmptyState from "../../../../components/EmptyState";
import { palette, radius, space } from "../../../../layout/theme";
import { capabilityMeta, sortCapabilities } from "../../../../lib/capabilities";
import { searchPlugins } from "../../../../lib/api/marketplace";
import { humanizeError } from "../../../../lib/humanizeError";

interface Props {
    /** Plugin IDs the operator has ticked so far. */
    selected: string[];
    onChange: (next: string[]) => void;
}

// BaselinePluginsStep is the wizard's per-enrollment plugin picker.
// Source of truth is the marketplace catalog, so an operator who
// hasn't synced the catalog will see the empty state — that's the
// honest UX, since installing without a catalog isn't possible.
//
// Contract:
//   - Defaults to nothing selected. The requirements thread asked
//     for "minimal default + operator opts in", so we never
//     pre-select anything.
//   - Selection is just a list of plugin IDs; capability authorisation
//     happens on first install at the agent (the install URL carries
//     the IDs forward, the agent applies the operator-confirmed
//     granted_capabilities at first boot).
export default function BaselinePluginsStep({ selected, onChange }: Props) {
    const plugins = useQuery({
        queryKey: ["enroll", "baseline-plugins-pool"],
        queryFn: () => searchPlugins(""),
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
                    title="Couldn't load marketplace"
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
                    title="Marketplace catalog is empty"
                    description="Sync the marketplace from /marketplace first to pick baseline plugins. The agent will boot with no plugins until you install some on the host page."
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
                Plugins to auto-install on first boot. Default empty: the
                agent connects with no host capabilities and you add them
                later from the host's Plugins tab. Pick only what this host
                needs out of the box.
            </p>
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
                        const isSel = selected.includes(p.plugin_id);
                        const caps = sortCapabilities(
                            p.capabilities.map((f) => ({ family: f })),
                        );
                        const hasHigh = caps.some(
                            (c) => capabilityMeta(c.family).risk === "high",
                        );
                        return (
                            <li
                                key={p.plugin_id}
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
                                    id={`baseline-${p.plugin_id}`}
                                    aria-label={p.name}
                                    checked={isSel}
                                    onCheckedChange={() => toggle(p.plugin_id)}
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
                                        htmlFor={`baseline-${p.plugin_id}`}
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
                                        {p.plugin_id}
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
