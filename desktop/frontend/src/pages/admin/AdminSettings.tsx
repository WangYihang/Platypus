import { useCallback, useEffect, useMemo, useState } from "react";
import { Loader2, RotateCcw } from "lucide-react";
import { toast } from "sonner";

import Card from "../../components/Card";
import EmptyState from "../../components/EmptyState";
import PageHeader from "../../components/PageHeader";
import StatusPill from "../../components/StatusPill";
import { palette, space } from "../../layout/theme";
import {
    SettingDescriptor,
    listSettings,
    resetSetting,
    updateSetting,
} from "../../lib/api";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";

// Section order on the page. Anything outside this list appears at the
// end under "Other".
const SECTION_ORDER = ["auth", "distributor", "mesh", "audit"] as const;

const SECTION_TITLES: Record<string, string> = {
    auth: "Authentication",
    distributor: "Agent distributor",
    mesh: "Mesh overlay",
    audit: "Audit log",
};

const SOURCE_TONE: Record<SettingDescriptor["source"], "danger" | "info" | "neutral"> = {
    db: "info",
    yaml: "neutral",
    default: "neutral",
};

// formValueFor produces the string form-value for an input, given the
// effective typed value. We use strings throughout the form state; the
// submit path parses them back to the right type before sending.
// string_list values are rendered newline-separated so a <textarea>
// stays simple ("one peer per line").
function formValueFor(d: SettingDescriptor): string {
    if (d.effective === undefined || d.effective === null) return "";
    if (d.type === "string_list") {
        if (Array.isArray(d.effective)) return (d.effective as string[]).join("\n");
        return "";
    }
    return String(d.effective);
}

// parseFormValue converts a form string back to the JSON-typed value
// the server expects. Returns { ok, value, error } so validation
// errors surface inline.
function parseFormValue(
    d: SettingDescriptor,
    raw: string,
): { ok: true; value: unknown } | { ok: false; error: string } {
    switch (d.type) {
        case "bool":
            if (raw === "true") return { ok: true, value: true };
            if (raw === "false") return { ok: true, value: false };
            return { ok: false, error: "must be true or false" };
        case "duration_seconds":
        case "int": {
            const n = Number(raw);
            if (!Number.isFinite(n) || !Number.isInteger(n)) {
                return { ok: false, error: "must be an integer" };
            }
            if (d.type === "duration_seconds" && n <= 0) {
                return { ok: false, error: "must be > 0 seconds" };
            }
            return { ok: true, value: n };
        }
        case "string":
            return { ok: true, value: raw };
        case "string_list": {
            const items = raw
                .split("\n")
                .map((s) => s.trim())
                .filter((s) => s !== "");
            return { ok: true, value: items };
        }
        default:
            return { ok: false, error: `unknown type ${d.type}` };
    }
}

// AdminSettings is the /admin/settings page — admin-only runtime
// policy knobs backed by internal/settings.Registry. Each section
// card renders as an independent form; Save writes just that
// section's dirty rows via PUT /api/v1/admin/settings/:key, Reset
// clears the DB override for one row via DELETE.
export default function AdminSettings() {
    const [descs, setDescs] = useState<SettingDescriptor[] | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);
    // Per-key in-progress string values. Seeded from descs, updated
    // on each input change, diffed against descs on Save.
    const [draft, setDraft] = useState<Record<string, string>>({});
    const [saving, setSaving] = useState<string | null>(null); // section being saved

    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            const rows = await listSettings();
            setDescs(rows);
            setDraft(Object.fromEntries(rows.map((d) => [d.key, formValueFor(d)])));
            setError(null);
        } catch (e) {
            setError(String(e));
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        refresh();
    }, [refresh]);

    const sections = useMemo(() => {
        if (!descs) return [];
        const bySection = new Map<string, SettingDescriptor[]>();
        for (const d of descs) {
            const arr = bySection.get(d.section) ?? [];
            arr.push(d);
            bySection.set(d.section, arr);
        }
        const ordered = [...SECTION_ORDER].filter((s) => bySection.has(s));
        const extras = [...bySection.keys()].filter(
            (s) => !SECTION_ORDER.includes(s as (typeof SECTION_ORDER)[number]),
        );
        return [...ordered, ...extras].map((s) => ({
            id: s,
            title: SECTION_TITLES[s] ?? s,
            rows: bySection.get(s) ?? [],
        }));
    }, [descs]);

    async function handleSave(section: string, rows: SettingDescriptor[]) {
        setSaving(section);
        try {
            const errors: string[] = [];
            for (const d of rows) {
                const before = formValueFor(d);
                const after = draft[d.key] ?? before;
                if (after === before) continue;
                const parsed = parseFormValue(d, after);
                if (!parsed.ok) {
                    errors.push(`${d.label}: ${parsed.error}`);
                    continue;
                }
                try {
                    await updateSetting(d.key, parsed.value);
                } catch (e) {
                    errors.push(`${d.label}: ${String(e)}`);
                }
            }
            if (errors.length > 0) {
                toast.error(errors.join("\n"));
            } else {
                toast.success(`Saved ${SECTION_TITLES[section] ?? section}`);
            }
            await refresh();
        } finally {
            setSaving(null);
        }
    }

    async function handleReset(d: SettingDescriptor) {
        try {
            await resetSetting(d.key);
            toast.success(`Reset ${d.label}`);
            await refresh();
        } catch (e) {
            toast.error(`reset: ${String(e)}`);
        }
    }

    return (
        <div
            style={{
                display: "flex",
                flexDirection: "column",
                gap: space[4],
                padding: space[4],
                minHeight: "100%",
                background: palette.main,
            }}
        >
            <PageHeader
                title="Server settings"
                subtitle="Runtime policy knobs. Changes take effect immediately — no restart needed."
            />

            {loading && descs === null && (
                <div style={{ display: "flex", justifyContent: "center", padding: space[6] }}>
                    <Loader2 className="size-5 animate-spin text-text-muted" />
                </div>
            )}

            {error && (
                <EmptyState
                    title="Couldn't load settings"
                    description={error}
                    action={
                        <Button variant="outline" onClick={refresh}>
                            Retry
                        </Button>
                    }
                />
            )}

            {descs && sections.map(({ id, title, rows }) => (
                <Card key={id}>
                    <div
                        style={{
                            display: "flex",
                            alignItems: "center",
                            justifyContent: "space-between",
                            marginBottom: space[3],
                        }}
                    >
                        <h2
                            style={{
                                fontSize: 16,
                                fontWeight: 600,
                                color: palette.textPrimary,
                                margin: 0,
                            }}
                        >
                            {title}
                        </h2>
                        <Button
                            size="sm"
                            disabled={saving !== null}
                            onClick={() => handleSave(id, rows)}
                        >
                            {saving === id ? (
                                <>
                                    <Loader2 className="size-3 animate-spin" />
                                    Saving
                                </>
                            ) : (
                                "Save"
                            )}
                        </Button>
                    </div>

                    <div style={{ display: "flex", flexDirection: "column", gap: space[3] }}>
                        {rows.map((d) => (
                            <SettingRow
                                key={d.key}
                                d={d}
                                value={draft[d.key] ?? formValueFor(d)}
                                onChange={(v) =>
                                    setDraft((prev) => ({ ...prev, [d.key]: v }))
                                }
                                onReset={() => handleReset(d)}
                                disabled={saving !== null}
                            />
                        ))}
                    </div>
                </Card>
            ))}
        </div>
    );
}

interface SettingRowProps {
    d: SettingDescriptor;
    value: string;
    onChange: (next: string) => void;
    onReset: () => void;
    disabled: boolean;
}

function SettingRow({ d, value, onChange, onReset, disabled }: SettingRowProps) {
    return (
        <div
            style={{
                display: "grid",
                gridTemplateColumns: "minmax(0,1fr) 240px auto",
                alignItems: "center",
                gap: space[3],
                padding: `${space[2]}px 0`,
                borderBottom: `1px solid ${palette.border}`,
            }}
        >
            <div style={{ minWidth: 0 }}>
                <div
                    style={{
                        display: "flex",
                        alignItems: "center",
                        gap: space[2],
                        fontSize: 14,
                        fontWeight: 500,
                        color: palette.textPrimary,
                    }}
                >
                    <span>{d.label}</span>
                    <StatusPill tone={SOURCE_TONE[d.source]}>{d.source}</StatusPill>
                </div>
                <div
                    style={{ fontSize: 12, color: palette.textMuted, marginTop: space[1] }}
                >
                    {d.description}
                </div>
                <div
                    style={{
                        fontSize: 11,
                        color: palette.textMuted,
                        marginTop: space[1],
                        fontFamily: "var(--font-mono, monospace)",
                    }}
                >
                    {d.key} · default: {String(d.default)}
                </div>
            </div>

            <div>{renderInput(d, value, onChange, disabled)}</div>

            <Button
                variant="ghost"
                size="sm"
                disabled={disabled || d.source !== "db"}
                onClick={onReset}
                title={d.source === "db" ? "Reset to default" : "No override to reset"}
            >
                <RotateCcw className="size-3.5" />
            </Button>
        </div>
    );
}

function renderInput(
    d: SettingDescriptor,
    value: string,
    onChange: (next: string) => void,
    disabled: boolean,
) {
    if (d.type === "bool") {
        return (
            <Switch
                checked={value === "true"}
                disabled={disabled}
                onCheckedChange={(v) => onChange(v ? "true" : "false")}
            />
        );
    }
    if (d.type === "duration_seconds" || d.type === "int") {
        return (
            <Input
                type="number"
                inputMode="numeric"
                disabled={disabled}
                value={value}
                onChange={(e) => onChange(e.target.value)}
            />
        );
    }
    if (d.type === "string_list") {
        return (
            <textarea
                disabled={disabled}
                value={value}
                onChange={(e) => onChange(e.target.value)}
                rows={Math.max(2, (value.match(/\n/g)?.length ?? 0) + 1)}
                placeholder={"one per line\nhost:9443"}
                style={{
                    width: "100%",
                    minHeight: 60,
                    padding: "6px 8px",
                    fontFamily: "var(--font-mono, monospace)",
                    fontSize: 12,
                    border: `1px solid ${palette.border}`,
                    borderRadius: 4,
                    background: palette.surface,
                    color: palette.textPrimary,
                    resize: "vertical",
                }}
            />
        );
    }
    return (
        <Input
            type="text"
            disabled={disabled}
            value={value}
            onChange={(e) => onChange(e.target.value)}
        />
    );
}
