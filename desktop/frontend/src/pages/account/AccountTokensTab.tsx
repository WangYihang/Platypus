import { useCallback, useEffect, useState } from "react";
import { KeyRound, Loader2, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";

import Card from "../../components/Card";
import EmptyState from "../../components/EmptyState";
import Mono from "../../components/Mono";
import RefreshButton from "../../components/RefreshButton";
import Toolbar from "../../components/Toolbar";
import { palette, space } from "../../layout/theme";
import { humanizeError } from "../../lib/humanizeError";
import { fromNow } from "../../lib/time";
import {
    type AccountPAT,
    type IssueAccountPATResponse,
    listAccountPATs,
    revokeAccountPAT,
} from "../../lib/api";
import {
    AlertDialog,
    AlertDialogAction,
    AlertDialogCancel,
    AlertDialogContent,
    AlertDialogDescription,
    AlertDialogFooter,
    AlertDialogHeader,
    AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";

import IssueAccountPATDialog from "./IssueAccountPATDialog";
import IssuedAccountPATDialog from "./IssuedAccountPATDialog";

export default function AccountTokensTab() {
    const [rows, setRows] = useState<AccountPAT[] | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(false);
    const [includeRevoked, setIncludeRevoked] = useState(false);
    const [issueOpen, setIssueOpen] = useState(false);
    const [lastIssued, setLastIssued] = useState<IssueAccountPATResponse | null>(null);
    const [pendingRevoke, setPendingRevoke] = useState<AccountPAT | null>(null);

    const refresh = useCallback(async () => {
        setLoading(true);
        try {
            const data = await listAccountPATs(includeRevoked);
            setRows(data);
            setError(null);
        } catch (e) {
            setError(humanizeError(e));
            toast.error(`Couldn't load tokens: ${humanizeError(e)}`);
        } finally {
            setLoading(false);
        }
    }, [includeRevoked]);

    useEffect(() => {
        refresh();
    }, [refresh]);

    async function confirmRevoke() {
        if (!pendingRevoke) return;
        const r = pendingRevoke;
        setPendingRevoke(null);
        try {
            await revokeAccountPAT(r.token_id);
            toast.success("Token revoked");
            refresh();
        } catch (e) {
            toast.error(`Couldn't revoke: ${humanizeError(e)}`);
        }
    }

    return (
        <>
            <p
                style={{
                    color: palette.textSecondary,
                    fontSize: 13,
                    lineHeight: 1.5,
                    marginTop: 0,
                    marginBottom: space[3],
                }}
            >
                Personal access tokens authenticate API requests as your user.
                Pass them as <Mono>Authorization: Bearer pat_…</Mono>.
            </p>
            <Toolbar
                left={
                    <label
                        style={{
                            display: "inline-flex",
                            alignItems: "center",
                            gap: space[2],
                            fontSize: 13,
                            color: palette.textSecondary,
                            cursor: "pointer",
                        }}
                    >
                        <Checkbox
                            checked={includeRevoked}
                            onCheckedChange={(v) => setIncludeRevoked(Boolean(v))}
                        />
                        Show revoked
                    </label>
                }
                right={
                    <>
                        <RefreshButton loading={loading} onClick={refresh} />
                        <Button size="sm" onClick={() => setIssueOpen(true)}>
                            <Plus className="size-3.5" />
                            Issue token
                        </Button>
                    </>
                }
            />
            {error && (
                <div
                    style={{
                        marginBottom: space[3],
                        padding: `${space[3]}px ${space[4]}px`,
                        border: `1px solid ${palette.danger}`,
                        borderRadius: 6,
                        color: palette.danger,
                        fontSize: 13,
                    }}
                >
                    {error}
                </div>
            )}
            <Card padding={0}>
                {rows === null ? (
                    <div className="flex items-center justify-center p-10">
                        <Loader2 className="size-5 animate-spin text-text-muted" />
                    </div>
                ) : rows.length === 0 ? (
                    <EmptyState
                        icon={<KeyRound className="size-7" />}
                        title="No personal access tokens"
                        description="Tokens you issue will appear here. The plaintext shows once at creation — copy it then."
                    />
                ) : (
                    <PATTable rows={rows} onRevoke={(r) => setPendingRevoke(r)} />
                )}
            </Card>

            <IssueAccountPATDialog
                open={issueOpen}
                onOpenChange={(o) => {
                    setIssueOpen(o);
                    if (!o) refresh();
                }}
                onIssued={(r) => {
                    setLastIssued(r);
                    setIssueOpen(false);
                    refresh();
                }}
            />
            <IssuedAccountPATDialog
                result={lastIssued}
                onClose={() => setLastIssued(null)}
            />

            <AlertDialog
                open={pendingRevoke !== null}
                onOpenChange={(o) => !o && setPendingRevoke(null)}
            >
                <AlertDialogContent>
                    <AlertDialogHeader>
                        <AlertDialogTitle>Revoke this token?</AlertDialogTitle>
                        <AlertDialogDescription>
                            Anyone using <Mono>{pendingRevoke?.name}</Mono> will start
                            getting 401s on the next request.
                        </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                        <AlertDialogCancel>Cancel</AlertDialogCancel>
                        <AlertDialogAction
                            onClick={confirmRevoke}
                            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
                        >
                            Revoke
                        </AlertDialogAction>
                    </AlertDialogFooter>
                </AlertDialogContent>
            </AlertDialog>
        </>
    );
}

function PATTable({
    rows,
    onRevoke,
}: {
    rows: AccountPAT[];
    onRevoke: (r: AccountPAT) => void;
}) {
    return (
        <Table>
            <TableHeader>
                <TableRow>
                    <TableHead className="w-[180px]">Name</TableHead>
                    <TableHead>Scopes</TableHead>
                    <TableHead className="w-[140px]">Last used</TableHead>
                    <TableHead className="w-[140px]">Expires</TableHead>
                    <TableHead className="w-[60px]" />
                </TableRow>
            </TableHeader>
            <TableBody>
                {rows.map((r) => (
                    <TableRow key={r.token_id} style={{ opacity: r.revoked ? 0.55 : 1 }}>
                        <TableCell>
                            <div style={{ fontWeight: 500 }}>{r.name}</div>
                            {r.description && (
                                <div
                                    style={{
                                        color: palette.textMuted,
                                        fontSize: 12,
                                    }}
                                >
                                    {r.description}
                                </div>
                            )}
                        </TableCell>
                        <TableCell>
                            <span
                                style={{
                                    fontFamily: "var(--font-mono)",
                                    fontSize: 11,
                                    color: palette.textSecondary,
                                }}
                            >
                                {r.scopes.join(" ")}
                            </span>
                        </TableCell>
                        <TableCell>
                            {r.last_used_at ? (
                                fromNow(r.last_used_at)
                            ) : (
                                <span className="text-text-muted">never</span>
                            )}
                        </TableCell>
                        <TableCell>
                            {r.revoked ? "revoked" : fromNow(r.expires_at)}
                        </TableCell>
                        <TableCell>
                            {!r.revoked && (
                                <Button
                                    variant="ghost"
                                    size="sm"
                                    className="h-auto px-2 py-1 text-destructive hover:text-destructive"
                                    onClick={() => onRevoke(r)}
                                    aria-label={`Revoke ${r.name}`}
                                >
                                    <Trash2 className="size-3.5" />
                                </Button>
                            )}
                        </TableCell>
                    </TableRow>
                ))}
            </TableBody>
        </Table>
    );
}
