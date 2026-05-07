import { useMemo } from "react";

import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { palette, space } from "../layout/theme";

import {
    CreateProjectSecretRequest,
    MarketplacePlugin,
    ProjectSecretRedacted,
} from "../lib/api";
import SchemaForm, { JSONSchema, SchemaValue, SecretWidgetProps } from "./SchemaForm";
import SecretPicker from "./SecretPicker";

/**
 * PluginSpecDraft is the editor's controlled value. Exact fields
 * mirror the wire-side PluginSpec — the parent passes it straight
 * to the wizard's plugin_specs[] array on save.
 */
export interface PluginSpecDraft {
    plugin_id: string;
    version?: string;
    granted_capabilities?: string[];
    config_overrides?: Record<string, SchemaValue>;
    schema_version?: number;
}

interface Props {
    projectID: string;
    plugin: MarketplacePlugin;
    secrets: ProjectSecretRedacted[];
    value: PluginSpecDraft;
    onChange: (next: PluginSpecDraft) => void;
    onCreateSecret: (
        req: CreateProjectSecretRequest,
    ) => Promise<ProjectSecretRedacted>;
}

// PluginSpecEditor is the central form for one plugin deployment.
// Three concerns rendered together:
//   1. Capability grants — the operator-authoritative subset of the
//      manifest's declared families. Defaults to none (the
//      "operator-authority-wins" rule from chooseCaps in the
//      reconciler).
//   2. Config form — schema-driven fields drawn from the manifest's
//      config.schema. Plugins that don't declare a schema show
//      only the capability section.
//   3. Secret-marked fields — fields whose path is in
//      config.secret_fields render the SecretPicker instead of a
//      plaintext input. The picker writes a {"$secret":"sec_..."}
//      reference into config_overrides; the server resolves it at
//      install time.
//
// The whole editor is controlled (value + onChange) so the parent
// (BaselinePluginsStep, the future per-host install dialog) stays
// the single source of truth for the spec it's about to save.
export default function PluginSpecEditor({
    projectID,
    plugin,
    secrets,
    value,
    onChange,
    onCreateSecret,
}: Props) {
    const schema = plugin.config_schema as JSONSchema | undefined;
    const secretFields = plugin.config_secret_fields ?? [];
    const grants = value.granted_capabilities ?? [];

    const overrides = useMemo<Record<string, SchemaValue>>(
        () => value.config_overrides ?? {},
        [value.config_overrides],
    );

    const renderSecretWidget = (props: SecretWidgetProps) => {
        // The SecretRef is `{$secret: "sec_..."}`. Plaintext typed
        // in the picker's inline-create flow round-trips through
        // onCreateSecret → onSelect(id) → SecretRef.
        const current =
            typeof props.value === "object" &&
            props.value !== null &&
            "$secret" in (props.value as Record<string, unknown>)
                ? String((props.value as { $secret: string }).$secret)
                : "";
        return (
            <SecretPicker
                projectID={projectID}
                secrets={secrets}
                value={current || undefined}
                onSelect={(id) => {
                    if (!id) {
                        // Clearing yields an undefined override at
                        // this path — matches "operator removed
                        // the value, agent gets schema default".
                        props.onChange(undefined);
                        return;
                    }
                    props.onChange({ $secret: id });
                }}
                onCreate={onCreateSecret}
            />
        );
    };

    return (
        <div className="space-y-4">
            <CapabilitiesSection
                declared={plugin.capabilities ?? []}
                granted={grants}
                onChange={(next) =>
                    onChange({ ...value, granted_capabilities: next })
                }
            />
            {schema && (
                <ConfigSection
                    schema={schema}
                    schemaVersion={plugin.config_schema_version}
                    secretFields={secretFields}
                    value={overrides}
                    onChange={(next) =>
                        onChange({
                            ...value,
                            config_overrides: next,
                            // Pin the schema version every time the
                            // operator edits the config so the
                            // saved spec carries the same version
                            // the schema-form rendered against.
                            // The server-side validator refuses
                            // stale-version specs.
                            schema_version: plugin.config_schema_version,
                        })
                    }
                    renderSecretWidget={renderSecretWidget}
                />
            )}
        </div>
    );
}

function CapabilitiesSection({
    declared,
    granted,
    onChange,
}: {
    declared: string[];
    granted: string[];
    onChange: (next: string[]) => void;
}) {
    if (declared.length === 0) {
        return null;
    }
    const grantedSet = new Set(granted);
    return (
        <div className="space-y-2">
            <SectionHeader title="Capabilities" />
            <div
                style={{
                    fontSize: 11,
                    color: palette.textMuted,
                }}
            >
                Operator-authoritative subset of the manifest's declared
                families. The agent enforces this set on every host-function
                call.
            </div>
            <div className="space-y-1">
                {declared.map((cap) => {
                    const id = `pse-cap-${cap}`;
                    return (
                        <div
                            key={cap}
                            style={{
                                display: "flex",
                                alignItems: "center",
                                gap: space[2],
                            }}
                        >
                            <Checkbox
                                id={id}
                                checked={grantedSet.has(cap)}
                                onCheckedChange={(c) => {
                                    const next = new Set(grantedSet);
                                    if (c) next.add(cap);
                                    else next.delete(cap);
                                    onChange(
                                        declared.filter((d) => next.has(d)),
                                    );
                                }}
                            />
                            <Label
                                htmlFor={id}
                                style={{
                                    fontSize: 12,
                                    fontFamily:
                                        "ui-monospace, SFMono-Regular, monospace",
                                    color: palette.textPrimary,
                                }}
                            >
                                {cap}
                            </Label>
                        </div>
                    );
                })}
            </div>
        </div>
    );
}

function ConfigSection({
    schema,
    schemaVersion,
    secretFields,
    value,
    onChange,
    renderSecretWidget,
}: {
    schema: JSONSchema;
    schemaVersion: number | undefined;
    secretFields: string[];
    value: Record<string, SchemaValue>;
    onChange: (next: Record<string, SchemaValue>) => void;
    renderSecretWidget: (p: SecretWidgetProps) => React.ReactNode;
}) {
    return (
        <div className="space-y-2">
            <SectionHeader title="Configuration" />
            <SchemaForm
                schema={schema}
                value={value}
                onChange={onChange}
                secretFields={secretFields}
                renderSecretWidget={renderSecretWidget}
            />
            {schemaVersion !== undefined && (
                <div
                    style={{
                        fontSize: 11,
                        color: palette.textMuted,
                    }}
                >
                    Pinned to schema version {schemaVersion}. A plugin update
                    that bumps schema_version will refuse this saved config
                    until you re-author it.
                </div>
            )}
        </div>
    );
}

function SectionHeader({ title }: { title: string }) {
    return (
        <div
            style={{
                fontSize: 11,
                color: palette.textMuted,
                textTransform: "uppercase",
                letterSpacing: 0.4,
            }}
        >
            {title}
        </div>
    );
}
