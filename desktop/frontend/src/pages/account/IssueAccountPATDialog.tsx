import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";

import { palette } from "../../layout/theme";
import { useOnValueChange, useResetOnOpen } from "@/lib/useOnValueChange";
import { humanizeError } from "../../lib/humanizeError";
import {
    type IssueAccountPATResponse,
    issueAccountPAT,
    listMyPermissions,
} from "../../lib/api";
import { Button } from "@/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";

import ScopeGroup from "./ScopeGroup";
import { groupScopesByResource } from "./scopes";

interface Props {
    open: boolean;
    onOpenChange: (o: boolean) => void;
    onIssued: (r: IssueAccountPATResponse) => void;
}

export default function IssueAccountPATDialog({ open, onOpenChange, onIssued }: Props) {
    const [name, setName] = useState("");
    const [description, setDescription] = useState("");
    // available is the caller's effective permission set, fetched on open.
    // selected is the subset they want to put on the new PAT.
    // Fetched on open. useQuery rather than a hand-rolled load: it
    // owns the in-flight/error states and dedupes if the operator
    // opens the dialog twice in a row.
    const permsQuery = useQuery({
        queryKey: ["myPermissions"],
        queryFn: listMyPermissions,
        enabled: open,
        refetchOnWindowFocus: false,
        retry: false,
    });
    const available = permsQuery.data ?? null;
    const [selected, setSelected] = useState<Set<string>>(new Set());
    const [ttlDays, setTtlDays] = useState<number>(90);
    const [submitting, setSubmitting] = useState(false);

    // Each opening starts from a clean form.
    useResetOnOpen(open, () => {
        setName("");
        setDescription("");
        setTtlDays(90);
        setSubmitting(false);
        setSelected(new Set());
    });

    // Everything is pre-selected when the permission list lands, and
    // the operator narrows from there — so this is state seeded from
    // async data, not derived from it, and has to be written once per
    // arrival rather than recomputed.
    useOnValueChange(permsQuery.data, (perms) => {
        if (perms) setSelected(new Set(perms));
    });

    // A toast is an external side effect, so it belongs in an effect —
    // useOnValueChange runs during a render React is still free to
    // discard, which would fire this for a render nobody saw.
    useEffect(() => {
        if (permsQuery.error) {
            toast.error(`Couldn't load permissions: ${humanizeError(permsQuery.error)}`);
        }
    }, [permsQuery.error]);

    function toggleScope(s: string) {
        setSelected((prev) => {
            const next = new Set(prev);
            if (next.has(s)) next.delete(s);
            else next.add(s);
            return next;
        });
    }

    async function submit(e: React.FormEvent) {
        e.preventDefault();
        if (!name.trim() || selected.size === 0) return;
        setSubmitting(true);
        try {
            const r = await issueAccountPAT({
                name: name.trim(),
                description: description.trim() || undefined,
                scopes: Array.from(selected),
                ttl_seconds: ttlDays * 24 * 60 * 60,
            });
            onIssued(r);
        } catch (e) {
            toast.error(`Couldn't issue: ${humanizeError(e)}`);
        } finally {
            setSubmitting(false);
        }
    }

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="sm:max-w-[520px]">
                <DialogHeader>
                    <DialogTitle>Issue personal access token</DialogTitle>
                    <DialogDescription>
                        The plaintext appears once after creation — copy it before
                        closing the next dialog.
                    </DialogDescription>
                </DialogHeader>
                <form onSubmit={submit} className="space-y-4">
                    <div className="space-y-1">
                        <Label htmlFor="pat-name">Name</Label>
                        <Input
                            id="pat-name"
                            placeholder="e.g. ci-runner"
                            value={name}
                            onChange={(e) => setName(e.target.value)}
                            autoFocus
                        />
                    </div>
                    <div className="space-y-1">
                        <Label htmlFor="pat-desc">Description (optional)</Label>
                        <Textarea
                            id="pat-desc"
                            rows={2}
                            value={description}
                            onChange={(e) => setDescription(e.target.value)}
                        />
                    </div>

                    <div className="space-y-2">
                        <Label>Scopes</Label>
                        {available === null ? (
                            <div className="flex items-center gap-2 text-text-muted text-xs">
                                <Loader2 className="size-3.5 animate-spin" />
                                Loading your permissions…
                            </div>
                        ) : available.length === 0 ? (
                            <p
                                style={{
                                    color: palette.textMuted,
                                    fontSize: 12,
                                    margin: 0,
                                }}
                            >
                                Your role doesn't grant any permissions — there's
                                nothing to scope a token to. Ask an admin to update
                                your role.
                            </p>
                        ) : (
                            groupScopesByResource(available).map(([resource, perms]) => (
                                <ScopeGroup
                                    key={resource}
                                    label={resource}
                                    options={perms}
                                    selected={selected}
                                    onToggle={toggleScope}
                                />
                            ))
                        )}
                    </div>

                    <div className="space-y-1">
                        <Label htmlFor="pat-ttl">Expires in (days)</Label>
                        <Input
                            id="pat-ttl"
                            type="number"
                            min={1}
                            max={365}
                            value={ttlDays}
                            onChange={(e) =>
                                setTtlDays(
                                    Math.max(1, Math.min(365, Number(e.target.value) || 90)),
                                )
                            }
                        />
                    </div>

                    <DialogFooter>
                        <Button
                            type="button"
                            variant="outline"
                            onClick={() => onOpenChange(false)}
                        >
                            Cancel
                        </Button>
                        <Button
                            type="submit"
                            disabled={submitting || !name.trim() || selected.size === 0}
                        >
                            {submitting && <Loader2 className="size-3.5 animate-spin" />}
                            Issue token
                        </Button>
                    </DialogFooter>
                </form>
            </DialogContent>
        </Dialog>
    );
}
