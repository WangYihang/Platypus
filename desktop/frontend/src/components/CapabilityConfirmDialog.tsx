import { ReactNode, useEffect, useMemo, useState } from "react";
import { ShieldAlert, ShieldCheck } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Separator } from "@/components/ui/separator";

import { capabilityMeta, sortCapabilities } from "@/lib/capabilities";

// Declared capability shape — what the plugin's manifest asks for.
// The `family` is the agent-side capability id ("fs.read", "exec",
// …); the optional fields are the manifest's upper-bound scope. We
// only show these as informational text — the agent's host fns
// enforce them.
export interface DeclaredCapability {
    family: string;
    paths?: string[];
    commands?: string[];
    hosts?: string[];
}

export interface CapabilityConfirmDialogProps {
    open: boolean;
    onOpenChange: (open: boolean) => void;
    pluginID: string;
    pluginVersion: string;
    pluginName: string;
    declared: DeclaredCapability[];
    // Called with the operator-approved subset (family-name list)
    // when the operator clicks Install. Cancel doesn't fire this.
    onApprove: (granted: string[]) => Promise<void> | void;
}

// CapabilityConfirmDialog gates every plugin install behind an
// explicit per-capability approval. Default state is empty — the
// operator must tick each family they want to grant. This is
// deliberately the "secure by default" posture: pressing Install
// without ticking anything yields a permission-less plugin (which
// fails fast at first host_* call), not a fully-trusted one.
//
// Path / command / host lists from the manifest are shown read-only
// beneath each family so the operator sees WHAT they're approving.
// They cannot narrow these from the UI today (per-path checkboxes
// are a future iteration); the agent still enforces the manifest's
// upper bound on every host_fn call.
export default function CapabilityConfirmDialog({
    open,
    onOpenChange,
    pluginID,
    pluginVersion,
    pluginName,
    declared,
    onApprove,
}: CapabilityConfirmDialogProps) {
    // Ticked families. We keep this as a Set for O(1) toggle + use a
    // useEffect to reset when the dialog re-opens — re-using state
    // across opens would surprise an operator who expected each
    // install confirmation to be independent.
    const [granted, setGranted] = useState<Set<string>>(() => new Set());
    const [submitting, setSubmitting] = useState(false);

    useEffect(() => {
        if (open) {
            setGranted(new Set());
            setSubmitting(false);
        }
    }, [open, pluginID, pluginVersion]);

    const sorted = useMemo(() => sortCapabilities(declared), [declared]);

    function toggle(family: string) {
        setGranted((prev) => {
            const next = new Set(prev);
            if (next.has(family)) next.delete(family);
            else next.add(family);
            return next;
        });
    }

    async function handleSubmit() {
        if (submitting) return;
        setSubmitting(true);
        try {
            await onApprove(Array.from(granted));
        } finally {
            setSubmitting(false);
        }
    }

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-xl">
                <DialogHeader>
                    <DialogTitle>
                        Install {pluginName}{" "}
                        <span className="font-mono text-xs text-muted-foreground">
                            v{pluginVersion}
                        </span>
                    </DialogTitle>
                    <DialogDescription>
                        Grant the capabilities this plugin needs. Anything you
                        leave unchecked will be denied at runtime — the plugin
                        fails the first time it tries to use the missing
                        capability.
                    </DialogDescription>
                </DialogHeader>

                <div className="px-1 text-xs font-mono text-muted-foreground">
                    {pluginID}
                </div>

                <ScrollArea className="max-h-[50vh] pr-2">
                    <ul className="flex flex-col gap-2 py-2">
                        {sorted.length === 0 ? (
                            <li className="text-sm text-muted-foreground italic px-2">
                                This plugin declares no capabilities. It will
                                still be sandboxed but won't be able to read
                                files, execute commands, or talk to the network.
                            </li>
                        ) : (
                            sorted.map((cap) => (
                                <CapabilityRow
                                    key={cap.family}
                                    cap={cap}
                                    checked={granted.has(cap.family)}
                                    onToggle={() => toggle(cap.family)}
                                />
                            ))
                        )}
                    </ul>
                </ScrollArea>

                <Separator />
                <div className="text-xs text-muted-foreground px-1">
                    {granted.size} of {sorted.length} capabilities granted
                </div>

                <DialogFooter>
                    <Button
                        variant="outline"
                        type="button"
                        onClick={() => onOpenChange(false)}
                    >
                        Cancel
                    </Button>
                    <Button
                        type="button"
                        onClick={handleSubmit}
                        disabled={submitting}
                    >
                        Install
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

function CapabilityRow({
    cap,
    checked,
    onToggle,
}: {
    cap: DeclaredCapability;
    checked: boolean;
    onToggle: () => void;
}) {
    const meta = capabilityMeta(cap.family);
    const scopeLines = scopeFor(cap);

    return (
        <li className="flex gap-3 rounded-md border border-border p-3">
            <Checkbox
                aria-label={meta.label}
                checked={checked}
                onCheckedChange={onToggle}
                className="mt-1"
            />
            <div className="flex flex-col gap-1 flex-1 min-w-0">
                <div className="flex items-center gap-2">
                    <span className="text-sm font-medium">{meta.label}</span>
                    <RiskBadge risk={meta.risk} />
                    <span className="text-xs font-mono text-muted-foreground">
                        {cap.family}
                    </span>
                </div>
                <p className="text-xs text-muted-foreground">{meta.summary}</p>
                {scopeLines.length > 0 && (
                    <ul className="flex flex-col gap-0.5 mt-1">
                        {scopeLines.map((line, i) => (
                            <li
                                key={i}
                                className="text-xs font-mono text-foreground/80 break-all"
                            >
                                {line}
                            </li>
                        ))}
                    </ul>
                )}
            </div>
        </li>
    );
}

function RiskBadge({ risk }: { risk: "low" | "medium" | "high" }): ReactNode {
    if (risk === "high") {
        return (
            <Badge variant="destructive" className="gap-1">
                <ShieldAlert className="size-3" />
                High risk
            </Badge>
        );
    }
    if (risk === "medium") {
        return (
            <Badge variant="secondary" className="gap-1">
                <ShieldAlert className="size-3" />
                Medium risk
            </Badge>
        );
    }
    return (
        <Badge variant="outline" className="gap-1">
            <ShieldCheck className="size-3" />
            Low risk
        </Badge>
    );
}

function scopeFor(cap: DeclaredCapability): string[] {
    if (cap.paths && cap.paths.length > 0) return cap.paths;
    if (cap.commands && cap.commands.length > 0) return cap.commands;
    if (cap.hosts && cap.hosts.length > 0) return cap.hosts;
    return [];
}
