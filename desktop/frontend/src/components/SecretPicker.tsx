import { useState } from "react";
import { ChevronDown, KeyRound, Plus, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { palette, space } from "../layout/theme";

import {
    CreateProjectSecretRequest,
    ProjectSecretRedacted,
} from "../lib/api";

interface Props {
    projectID: string;
    secrets: ProjectSecretRedacted[];
    /** Currently-selected secret id, or undefined for "no selection". */
    value: string | undefined;
    /** Called with the new secret_id when the operator picks one,
     *  or with "" when they clear the selection. */
    onSelect: (secretID: string) => void;
    /** Inline-create handler. Receives the form values and is
     *  expected to POST + return the redacted row. The picker
     *  immediately calls onSelect with the resulting id. */
    onCreate: (req: CreateProjectSecretRequest) => Promise<ProjectSecretRedacted>;
}

// SecretPicker is the form widget rendered for plugin config fields
// the manifest marked secret. The operator either picks an
// already-saved secret from the project or — discoverable inline —
// creates a fresh one without leaving the wizard. Either way the
// picker yields a `sec_<id>` to its caller; the form layer wraps
// it as {"$secret":"sec_<id>"} before sending to the server.
//
// Revoked secrets are filtered out: a config that references a
// revoked secret produces an install-time failure, so surfacing
// them in the picker would lead operators into a broken state.
export default function SecretPicker({
    projectID,
    secrets,
    value,
    onSelect,
    onCreate,
}: Props) {
    const [createOpen, setCreateOpen] = useState(false);

    const active = secrets.filter((s) => !s.revoked);
    const selected = active.find((s) => s.secret_id === value);
    const triggerLabel = selected ? selected.name : "Select a secret";

    return (
        <div
            style={{
                display: "flex",
                alignItems: "center",
                gap: space[1],
            }}
        >
            <DropdownMenu>
                <DropdownMenuTrigger asChild>
                    <Button
                        variant="outline"
                        size="sm"
                        data-testid="secret-picker"
                        style={{
                            justifyContent: "space-between",
                            minWidth: 220,
                            color: selected
                                ? palette.textPrimary
                                : palette.textMuted,
                        }}
                    >
                        <span
                            style={{
                                display: "inline-flex",
                                alignItems: "center",
                                gap: 6,
                                overflow: "hidden",
                                textOverflow: "ellipsis",
                                whiteSpace: "nowrap",
                            }}
                        >
                            <KeyRound className="size-3.5" />
                            {triggerLabel}
                        </span>
                        <ChevronDown className="size-3.5" />
                    </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start" className="min-w-[220px]">
                    {active.length === 0 ? (
                        <div
                            style={{
                                padding: space[2],
                                fontSize: 12,
                                color: palette.textMuted,
                            }}
                        >
                            No saved secrets in this project yet.
                        </div>
                    ) : (
                        active.map((s) => (
                            <DropdownMenuItem
                                key={s.secret_id}
                                onSelect={() => onSelect(s.secret_id)}
                                data-testid={`secret-picker-option-${s.secret_id}`}
                            >
                                <div
                                    style={{
                                        display: "flex",
                                        flexDirection: "column",
                                        fontSize: 12,
                                    }}
                                >
                                    <span style={{ color: palette.textPrimary }}>
                                        {s.name}
                                    </span>
                                    {s.description && (
                                        <span
                                            style={{
                                                color: palette.textMuted,
                                                fontSize: 11,
                                            }}
                                        >
                                            {s.description}
                                        </span>
                                    )}
                                </div>
                            </DropdownMenuItem>
                        ))
                    )}
                    <DropdownMenuSeparator />
                    <DropdownMenuItem
                        onSelect={() => setCreateOpen(true)}
                        data-testid="secret-picker-create"
                    >
                        <Plus className="size-3.5" />
                        Create new secret…
                    </DropdownMenuItem>
                </DropdownMenuContent>
            </DropdownMenu>
            {selected && (
                <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => onSelect("")}
                    title="Clear selection"
                    data-testid="secret-picker-clear"
                >
                    <X className="size-3.5" />
                </Button>
            )}
            {createOpen && (
                <InlineCreate
                    projectID={projectID}
                    onCreate={onCreate}
                    onCreated={(id) => {
                        setCreateOpen(false);
                        onSelect(id);
                    }}
                    onCancel={() => setCreateOpen(false)}
                />
            )}
        </div>
    );
}

// InlineCreate is the small two-input form that lets operators
// create a fresh secret without leaving the picker. Lives as a
// sibling of the dropdown trigger so the layout stays compact;
// the form's busy state is local since it's transient.
function InlineCreate({
    onCreate,
    onCreated,
    onCancel,
}: {
    projectID: string;
    onCreate: (req: CreateProjectSecretRequest) => Promise<ProjectSecretRedacted>;
    onCreated: (secretID: string) => void;
    onCancel: () => void;
}) {
    const [name, setName] = useState("");
    const [value, setValue] = useState("");
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState("");

    return (
        <div
            style={{
                display: "flex",
                gap: space[1],
                alignItems: "center",
            }}
        >
            <Input
                placeholder="name"
                size={12}
                value={name}
                onChange={(e) => setName(e.target.value)}
                disabled={busy}
                data-testid="secret-picker-new-name"
                style={{ width: 130 }}
            />
            <Input
                type="password"
                placeholder="value"
                value={value}
                onChange={(e) => setValue(e.target.value)}
                disabled={busy}
                data-testid="secret-picker-new-value"
                style={{ width: 160 }}
            />
            <Button
                size="sm"
                disabled={busy || !name.trim() || !value}
                onClick={async () => {
                    setBusy(true);
                    setError("");
                    try {
                        const r = await onCreate({
                            name: name.trim(),
                            value,
                        });
                        // Wipe the local copy of the plaintext as
                        // soon as the request resolves. Go's
                        // garbage collector will reap it eventually
                        // — clearing the state slot is best-effort.
                        setValue("");
                        onCreated(r.secret_id);
                    } catch (e) {
                        setError((e as Error).message ?? "create failed");
                    } finally {
                        setBusy(false);
                    }
                }}
                data-testid="secret-picker-new-confirm"
            >
                Save
            </Button>
            <Button
                variant="ghost"
                size="sm"
                onClick={onCancel}
                disabled={busy}
            >
                Cancel
            </Button>
            {error && (
                <span style={{ fontSize: 11, color: palette.danger }}>
                    {error}
                </span>
            )}
        </div>
    );
}
