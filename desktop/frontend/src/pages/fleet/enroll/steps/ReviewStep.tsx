import { Loader2, Save } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { palette, space } from "../../../../layout/theme";

import {
    UpsertEnrollmentPresetRequest,
    createEnrollmentPreset,
    updateEnrollmentPreset,
} from "../../../../lib/api";
import { humanizeError } from "../../../../lib/humanizeError";
import { Step } from "../steps";

interface Props {
    summary: Array<{ label: string; value: string; editStep: Step }>;
    onEdit: (step: Step) => void;
    // Preset save: identity + the snapshot of fields the operator just
    // walked through. editingPresetID is non-null when the operator
    // entered the wizard via "Edit preset" on the picker — in that
    // case we PUT the existing row instead of creating a new one.
    projectID: string;
    editingPresetID: string | null;
    presetSnapshot: UpsertEnrollmentPresetRequest;
    onSaved: (presetID: string) => void;
}

export default function ReviewStep({
    summary,
    onEdit,
    projectID,
    editingPresetID,
    presetSnapshot,
    onSaved,
}: Props) {
    return (
        <div className="space-y-3" data-testid="enroll-wizard-review">
            <div style={{ fontSize: 13, color: palette.textSecondary }}>
                Review configuration before generating one-shot commands.
            </div>
            <div className="space-y-2">
                {summary.map((item) => (
                    <div
                        key={item.label}
                        className="flex items-center justify-between rounded border border-border bg-surface p-2"
                    >
                        <div style={{ fontSize: 12 }}>
                            <div style={{ color: palette.textMuted }}>{item.label}</div>
                            <div style={{ color: palette.textPrimary }}>{item.value}</div>
                        </div>
                        <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => onEdit(item.editStep)}
                        >
                            Edit
                        </Button>
                    </div>
                ))}
            </div>
            <SaveAsPreset
                projectID={projectID}
                editingPresetID={editingPresetID}
                snapshot={presetSnapshot}
                onSaved={onSaved}
            />
        </div>
    );
}

// SaveAsPreset is the inline name-input → POST/PUT control that lets
// the operator capture the current wizard state as a reusable preset.
// Folding the form into the Review step (rather than a side dialog)
// keeps the "I'm about to generate, also remember this for next time"
// flow inside one screen.
function SaveAsPreset({
    projectID,
    editingPresetID,
    snapshot,
    onSaved,
}: {
    projectID: string;
    editingPresetID: string | null;
    snapshot: UpsertEnrollmentPresetRequest;
    onSaved: (presetID: string) => void;
}) {
    const [open, setOpen] = useState(false);
    const [name, setName] = useState("");
    const [saving, setSaving] = useState(false);

    if (!open) {
        return (
            <div style={{ display: "flex", justifyContent: "flex-end" }}>
                <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => {
                        setName(snapshot.name || "");
                        setOpen(true);
                    }}
                    data-testid="enroll-wizard-save-preset"
                >
                    <Save className="size-3.5" />
                    {editingPresetID
                        ? "Update preset"
                        : "Save as preset"}
                </Button>
            </div>
        );
    }
    return (
        <div
            className="rounded border border-border bg-surface"
            style={{ padding: space[3] }}
        >
            <div
                style={{
                    fontSize: 12,
                    color: palette.textMuted,
                    marginBottom: 4,
                }}
            >
                {editingPresetID
                    ? "Update saved preset"
                    : "Save these choices as a preset"}
            </div>
            <Input
                placeholder="Preset name (e.g. linux-prod)"
                value={name}
                onChange={(e) => setName(e.target.value)}
                disabled={saving}
                data-testid="enroll-wizard-preset-name"
            />
            <div
                style={{
                    display: "flex",
                    justifyContent: "flex-end",
                    gap: space[2],
                    marginTop: space[2],
                }}
            >
                <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => setOpen(false)}
                    disabled={saving}
                >
                    Cancel
                </Button>
                <Button
                    size="sm"
                    disabled={saving || !name.trim()}
                    onClick={async () => {
                        setSaving(true);
                        try {
                            const body: UpsertEnrollmentPresetRequest = {
                                ...snapshot,
                                name: name.trim(),
                            };
                            const r = editingPresetID
                                ? await updateEnrollmentPreset(
                                      projectID,
                                      editingPresetID,
                                      body,
                                  )
                                : await createEnrollmentPreset(
                                      projectID,
                                      body,
                                  );
                            toast.success(
                                editingPresetID
                                    ? `Updated preset "${r.name}"`
                                    : `Saved preset "${r.name}"`,
                            );
                            onSaved(r.preset_id);
                            setOpen(false);
                        } catch (e) {
                            toast.error(
                                `Couldn't save preset: ${humanizeError(e)}`,
                            );
                        } finally {
                            setSaving(false);
                        }
                    }}
                    data-testid="enroll-wizard-preset-save-confirm"
                >
                    {saving && <Loader2 className="size-3.5 animate-spin" />}
                    {editingPresetID ? "Update" : "Save"}
                </Button>
            </div>
        </div>
    );
}
