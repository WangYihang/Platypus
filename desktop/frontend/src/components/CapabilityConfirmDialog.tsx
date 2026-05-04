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

import {
    CAPABILITY_COLLECTIONS,
    CapabilityCollection,
    capabilityMeta,
    matchingCollection,
    sortCapabilities,
} from "@/lib/capabilities";

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
// explicit capability approval. Two layers, both opt-in:
//
//   1. COLLECTIONS — preset bundles ("Read-only inspection", "File
//      management", "Process control", "Network access", "Full
//      access"). Operators usually pick a collection that matches
//      the plugin's purpose and stop there.
//
//   2. ADVANCED — per-capability checkboxes for operators who want
//      to grant a custom subset. Selecting a collection pre-fills
//      these; ticking individual ones clears the collection
//      highlight (we recompute matchingCollection on every toggle).
//
// Default state is "no collection picked, no boxes ticked" — the
// secure-by-default posture: pressing Install with nothing chosen
// yields a permission-less plugin that fails on its first host_*
// call. Path / command / host scope strings come straight from the
// manifest and surface beneath the per-family rows; they're
// informational because the agent's host fns enforce them.
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
    const [advancedOpen, setAdvancedOpen] = useState(false);

    useEffect(() => {
        if (open) {
            setGranted(new Set());
            setAdvancedOpen(false);
            setSubmitting(false);
        }
    }, [open, pluginID, pluginVersion]);

    const sorted = useMemo(() => sortCapabilities(declared), [declared]);
    const declaredSet = useMemo(
        () => new Set(declared.map((d) => d.family)),
        [declared],
    );
    // Compute which collection (if any) the current grant set matches.
    // Drives the visual "selected" pill on the collection cards so an
    // operator who tweaked individual boxes can still see they're on
    // a recognised preset (or fell off into Custom territory).
    const activeCollection = useMemo(
        () => matchingCollection(declaredSet, granted),
        [declaredSet, granted],
    );

    function toggle(family: string) {
        setGranted((prev) => {
            const next = new Set(prev);
            if (next.has(family)) next.delete(family);
            else next.add(family);
            return next;
        });
    }

    function selectCollection(c: CapabilityCollection) {
        // Pre-fill with the collection's families, restricted to what
        // the plugin actually declared — granting a family the plugin
        // never asked for is meaningless and would confuse the
        // matchingCollection check on the next render.
        const next = new Set<string>();
        for (const f of c.families) {
            if (declaredSet.has(f)) next.add(f);
        }
        setGranted(next);
    }

    function clearAll() {
        setGranted(new Set());
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
                        Pick the smallest collection that lets the plugin do
                        its job. You can mix and match individual capabilities
                        under "Advanced". Anything you leave off is denied at
                        runtime — the plugin fails the first time it tries to
                        use the missing capability.
                    </DialogDescription>
                </DialogHeader>

                <div className="px-1 text-xs font-mono text-muted-foreground">
                    {pluginID}
                </div>

                {sorted.length === 0 ? (
                    <p className="text-sm text-muted-foreground italic px-2 py-3">
                        This plugin declares no capabilities. It will still be
                        sandboxed but won't be able to read files, execute
                        commands, or talk to the network.
                    </p>
                ) : (
                    <ScrollArea className="max-h-[55vh] pr-2">
                        <div className="flex flex-col gap-3 py-2">
                            <CollectionsGrid
                                declaredSet={declaredSet}
                                activeID={activeCollection?.id ?? null}
                                onPick={selectCollection}
                                onClear={clearAll}
                            />

                            <button
                                type="button"
                                onClick={() => setAdvancedOpen((o) => !o)}
                                className="self-start text-xs text-muted-foreground hover:text-foreground underline-offset-2 hover:underline"
                            >
                                {advancedOpen ? "Hide" : "Show"} advanced (per-capability)
                            </button>

                            {advancedOpen && (
                                <ul
                                    aria-label="advanced capabilities"
                                    className="flex flex-col gap-2"
                                >
                                    {sorted.map((cap) => (
                                        <CapabilityRow
                                            key={cap.family}
                                            cap={cap}
                                            checked={granted.has(cap.family)}
                                            onToggle={() => toggle(cap.family)}
                                        />
                                    ))}
                                </ul>
                            )}
                        </div>
                    </ScrollArea>
                )}

                <Separator />
                <div className="text-xs text-muted-foreground px-1">
                    {granted.size} of {sorted.length} capabilities granted
                    {activeCollection && (
                        <span> · matches "{activeCollection.label}"</span>
                    )}
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

// CollectionsGrid is the row of "preset" cards above the per-cap
// list. Each card shows what it grants in plain English; clicking
// it pre-fills the operator's grant set. A separate "Clear" card
// (rendered last) lets the operator drop back to nothing without
// hunting for the toggle.
function CollectionsGrid({
    declaredSet,
    activeID,
    onPick,
    onClear,
}: {
    declaredSet: Set<string>;
    activeID: string | null;
    onPick: (c: CapabilityCollection) => void;
    onClear: () => void;
}) {
    return (
        <div
            role="group"
            aria-label="capability collections"
            className="grid grid-cols-1 sm:grid-cols-2 gap-2"
        >
            {CAPABILITY_COLLECTIONS.map((c) => {
                const declaredCount = c.families.filter((f) => declaredSet.has(f)).length;
                const totalCount = c.families.length;
                const disabled = declaredCount === 0;
                const isActive = activeID === c.id;
                return (
                    <button
                        key={c.id}
                        type="button"
                        disabled={disabled}
                        aria-pressed={isActive}
                        onClick={() => onPick(c)}
                        className={[
                            "flex flex-col items-start gap-1 rounded-md border p-3 text-left transition-colors",
                            disabled
                                ? "border-border opacity-50 cursor-not-allowed"
                                : "border-border hover:bg-accent",
                            isActive ? "border-primary ring-2 ring-primary/30" : "",
                        ].join(" ")}
                    >
                        <div className="flex items-center gap-2 w-full">
                            <span className="text-sm font-medium flex-1">
                                {c.label}
                            </span>
                            <RiskBadge risk={c.risk} />
                        </div>
                        <p className="text-xs text-muted-foreground">{c.summary}</p>
                        <span className="text-xs text-muted-foreground">
                            Grants {declaredCount} of {totalCount} families this plugin declares
                        </span>
                    </button>
                );
            })}
            <button
                type="button"
                onClick={onClear}
                className="flex flex-col items-start gap-1 rounded-md border border-dashed border-border p-3 text-left hover:bg-accent transition-colors"
            >
                <span className="text-sm font-medium">Custom / clear</span>
                <p className="text-xs text-muted-foreground">
                    Reset every grant. Use the advanced panel below to tick
                    capabilities one by one.
                </p>
            </button>
        </div>
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
