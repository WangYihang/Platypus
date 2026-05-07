import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
    ChevronDown,
    ChevronRight,
    Loader2,
    Settings2,
    ShieldAlert,
    ShieldCheck,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";

import EmptyState from "../../../../components/EmptyState";
import { palette, radius, space } from "../../../../layout/theme";
import { capabilityMeta, sortCapabilities } from "../../../../lib/capabilities";
import { useCurrentProject } from "../../../../layout/ProjectShell";
import {
    SystemPlugin,
    listSystemPlugins,
} from "../../../../lib/api/system_plugins";
import {
    createProjectSecret,
    listProjectSecrets,
} from "../../../../lib/api/project_secrets";
import {
    BASELINE_PRESETS,
    BaselinePreset,
    matchingPreset,
} from "../../../../lib/baselinePresets";
import { humanizeError } from "../../../../lib/humanizeError";
import PluginSpecEditor, {
    PluginSpecDraft,
} from "../../../../components/PluginSpecEditor";
import { MarketplacePlugin } from "../../../../lib/api";

interface Props {
    /** PluginSpec drafts the operator has selected so far. The
     *  step renders the picker UI plus an inline expand row per
     *  selected plugin that surfaces capability checkboxes and
     *  (when the plugin declares a config schema) the schema-driven
     *  form. */
    value: PluginSpecDraft[];
    onChange: (next: PluginSpecDraft[]) => void;
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
export default function BaselinePluginsStep({ value, onChange }: Props) {
    const project = useCurrentProject();
    const plugins = useQuery({
        queryKey: ["enroll", "system-plugins-pool"],
        queryFn: () => listSystemPlugins(),
        refetchOnWindowFocus: false,
    });
    // Project-scoped secrets feed the inline PluginSpecEditor's
    // SecretPicker. Fetched lazily — most plugins won't reach for
    // it, but having the list ready means there's no UI churn the
    // moment an operator opens a Configure panel.
    const secrets = useQuery({
        queryKey: ["enroll", "project-secrets", project.id],
        queryFn: () => listProjectSecrets(project.id),
        refetchOnWindowFocus: false,
    });
    // Inline editor expansion state — keyed by plugin_id. Stored
    // as a Set so the per-row toggle stays O(1) and the operator
    // can have multiple panels open at once when fine-tuning a
    // multi-plugin baseline.
    const [expanded, setExpanded] = useState<Set<string>>(() => new Set());

    // Project value → flat ids for rendering (the picker UI stays
    // checkbox-per-plugin) and back to PluginSpec rows on each
    // edit. Per-plugin rich config is added by the parent's future
    // "Configure" expand row; for now the spec carries only its id.
    const selected = value.map((s) => s.plugin_id);

    function toggle(id: string) {
        const has = value.some((s) => s.plugin_id === id);
        if (has) {
            onChange(value.filter((s) => s.plugin_id !== id));
        } else {
            onChange([...value, { plugin_id: id }]);
        }
    }

    function setSelectedIDs(ids: string[]) {
        // Preserve any rich PluginSpec entries the operator has
        // already authored; new ids get a minimal {plugin_id} spec.
        const byID = new Map(value.map((s) => [s.plugin_id, s] as const));
        onChange(ids.map((id) => byID.get(id) ?? { plugin_id: id }));
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
                    setSelectedIDs(
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
                        const spec = value.find((s) => s.plugin_id === p.id);
                        const isSel = !!spec;
                        const isExpanded = expanded.has(p.id);
                        return (
                            <PluginRow
                                key={p.id}
                                plugin={p}
                                spec={spec}
                                selected={isSel}
                                expanded={isExpanded}
                                projectID={project.id}
                                secrets={secrets.data ?? []}
                                onToggleSelected={() => toggle(p.id)}
                                onToggleExpanded={() =>
                                    setExpanded((prev) => {
                                        const next = new Set(prev);
                                        if (next.has(p.id)) next.delete(p.id);
                                        else next.add(p.id);
                                        return next;
                                    })
                                }
                                onSpecChange={(updated) => {
                                    const next = value.map((s) =>
                                        s.plugin_id === p.id ? updated : s,
                                    );
                                    onChange(next);
                                }}
                                onCreateSecret={async (req) => {
                                    const r = await createProjectSecret(
                                        project.id,
                                        req,
                                    );
                                    await secrets.refetch();
                                    return r;
                                }}
                            />
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

// PluginRow renders one plugin in the baseline picker plus, when
// the operator both selects and expands the row, the
// PluginSpecEditor for per-plugin capabilities + config. The
// editor is the user-facing payoff of the PluginSpec atom that
// PR 1-4 plumbed end-to-end: every spec the operator authors here
// flows through the wire as-is to the agent's plugin runtime.
//
// SystemPlugin → MarketplacePlugin coercion is necessary because
// the editor is shape-agnostic but typed against MarketplacePlugin
// (the marketplace + system flows are gradually converging on a
// shared "PluginManifestInfo" shape; until then the inline
// coercion keeps the editor's surface narrow).
function PluginRow({
    plugin,
    spec,
    selected,
    expanded,
    projectID,
    secrets,
    onToggleSelected,
    onToggleExpanded,
    onSpecChange,
    onCreateSecret,
}: {
    plugin: SystemPlugin;
    spec: PluginSpecDraft | undefined;
    selected: boolean;
    expanded: boolean;
    projectID: string;
    secrets: import("../../../../lib/api/project_secrets").ProjectSecretRedacted[];
    onToggleSelected: () => void;
    onToggleExpanded: () => void;
    onSpecChange: (next: PluginSpecDraft) => void;
    onCreateSecret: (
        req: import("../../../../lib/api/project_secrets").CreateProjectSecretRequest,
    ) => Promise<
        import("../../../../lib/api/project_secrets").ProjectSecretRedacted
    >;
}) {
    const caps = sortCapabilities(
        plugin.capabilities.map((f) => ({ family: f })),
    );
    const hasHigh = caps.some(
        (c) => capabilityMeta(c.family).risk === "high",
    );
    const editorPlugin: MarketplacePlugin = {
        plugin_id: plugin.id,
        version: plugin.version,
        name: plugin.name,
        author: plugin.author ?? "",
        license: plugin.license ?? "",
        homepage: "",
        description: plugin.description ?? "",
        latest_version: plugin.version,
        publisher_key_id: "",
        wasm_url: "",
        signature_url: "",
        wasm_sha256_hex: "",
        capabilities: plugin.capabilities,
        fetched_at_unix: 0,
        // System plugins don't yet surface config_schema /
        // secret_fields / schema_version; the editor handles
        // their absence by rendering only the capability section.
        // Once the system-plugins endpoint is upgraded to expose
        // config metadata, this row picks up schema-driven config
        // automatically.
    };
    return (
        <li
            style={{
                display: "flex",
                flexDirection: "column",
                gap: space[2],
                border: `1px solid ${palette.border}`,
                borderRadius: radius.md,
                padding: space[3],
                background: palette.surface,
            }}
        >
            <div
                style={{
                    display: "flex",
                    gap: space[2],
                    alignItems: "flex-start",
                }}
            >
                <Checkbox
                    id={`baseline-${plugin.id}`}
                    aria-label={plugin.name}
                    checked={selected}
                    onCheckedChange={onToggleSelected}
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
                        htmlFor={`baseline-${plugin.id}`}
                        className="text-sm font-medium cursor-pointer"
                    >
                        <span
                            style={{
                                display: "flex",
                                gap: space[2],
                                alignItems: "center",
                            }}
                        >
                            {plugin.name}
                            <span
                                style={{
                                    fontSize: 11,
                                    color: palette.textMuted,
                                }}
                            >
                                v{plugin.version}
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
                        {plugin.id}
                    </span>
                    {plugin.description && (
                        <span
                            style={{
                                fontSize: 12,
                                color: palette.textSecondary,
                            }}
                        >
                            {plugin.description}
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
                {selected && (
                    <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={onToggleExpanded}
                        data-testid={`baseline-plugins-configure-${plugin.id}`}
                    >
                        {expanded ? (
                            <ChevronDown className="size-3.5" />
                        ) : (
                            <ChevronRight className="size-3.5" />
                        )}
                        <Settings2 className="size-3.5" />
                        Configure
                    </Button>
                )}
            </div>
            {selected && expanded && spec && (
                <div
                    style={{
                        borderTop: `1px solid ${palette.border}`,
                        paddingTop: space[2],
                    }}
                >
                    <PluginSpecEditor
                        projectID={projectID}
                        plugin={editorPlugin}
                        secrets={secrets}
                        value={spec}
                        onChange={onSpecChange}
                        onCreateSecret={onCreateSecret}
                    />
                </div>
            )}
        </li>
    );
}
