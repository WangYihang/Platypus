import { useEffect, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { CheckCircle2, Loader2, XCircle } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";
import { ScrollArea } from "@/components/ui/scroll-area";

import EmptyState from "../../../components/EmptyState";
import CapabilityConfirmDialog, {
    DeclaredCapability,
} from "../../../components/CapabilityConfirmDialog";
import { palette, space } from "../../../layout/theme";
import { humanizeError } from "../../../lib/humanizeError";
import {
    InstallProgress,
    InstallResult,
    installFromMarketplace,
} from "../../../lib/api/agents/plugins";
import {
    MarketplacePlugin,
    searchPlugins,
} from "../../../lib/api/marketplace";

interface Props {
    open: boolean;
    projectID: string;
    /** The host's agent_id (cert SAN). NOT the host row UUID — the
     *  per-agent install endpoint keys on this. */
    agentID: string;
    onClose: () => void;
    /** Called once after a plugin is successfully installed so the
     *  parent can invalidate its plugin list. */
    onInstalled: (pluginID: string) => void;
}

// InstallFromMarketplaceDialog drives the three phases of an
// install-from-catalog click:
//
//   1. PICK     — search/browse the marketplace catalog, click a row
//   2. CONFIRM  — CapabilityConfirmDialog shows the plugin's declared
//                 caps; operator ticks the ones to grant
//   3. PROGRESS — install fires; phases stream back; we render
//                 verify_sig → extract → load → installed/failed
//
// The dialog is the user's only point of contact for the whole flow,
// even though phase 2 is a separate Radix Dialog stacked on top.
export default function InstallFromMarketplaceDialog({
    open,
    projectID,
    agentID,
    onClose,
    onInstalled,
}: Props) {
    const [query, setQuery] = useState("");
    const [picked, setPicked] = useState<MarketplacePlugin | null>(null);
    const [installResult, setInstallResult] = useState<InstallResult | null>(null);

    // Reset the picker on every reopen — re-using state across opens
    // would surprise an operator who closed mid-flow then reopened.
    useEffect(() => {
        if (open) {
            setQuery("");
            setPicked(null);
            setInstallResult(null);
        }
    }, [open]);

    const plugins = useQuery({
        queryKey: ["marketplace", "search", query],
        queryFn: () => searchPlugins(query),
        enabled: open,
        refetchOnWindowFocus: false,
    });

    const install = useMutation({
        mutationFn: (vars: { pluginID: string; version: string; granted: string[] }) =>
            installFromMarketplace(projectID, agentID, {
                pluginID: vars.pluginID,
                version: vars.version,
                grantedCapabilities: vars.granted,
            }),
        onSuccess: (res) => {
            setInstallResult(res);
            // Close the cap dialog so the operator sees the progress
            // in the outer dialog (we do the close by clearing
            // `picked`); fire onInstalled on the happy path so the
            // parent refetches its list.
            setPicked(null);
            if (res.status === "installed") {
                toast.success(`Installed ${res.plugin_id}`);
                onInstalled(res.plugin_id);
            }
        },
        onError: (err) => {
            toast.error(humanizeError(err));
            setPicked(null);
        },
    });

    return (
        <>
            <Dialog
                open={open}
                onOpenChange={(next) => {
                    if (!next) onClose();
                }}
            >
                <DialogContent className="max-w-2xl">
                    <DialogHeader>
                        <DialogTitle>Install from Marketplace</DialogTitle>
                        <DialogDescription>
                            Pick a plugin to install on this agent. The next
                            step asks you to authorise its capabilities.
                        </DialogDescription>
                    </DialogHeader>

                    {installResult ? (
                        <ProgressView
                            result={installResult}
                            onClose={onClose}
                            onReset={() => setInstallResult(null)}
                        />
                    ) : install.isPending ? (
                        <InstallingView />
                    ) : (
                        <PickerBody
                            query={query}
                            setQuery={setQuery}
                            plugins={plugins.data ?? []}
                            isLoading={plugins.isLoading}
                            error={plugins.error}
                            onPick={setPicked}
                        />
                    )}
                </DialogContent>
            </Dialog>

            <CapabilityConfirmDialog
                open={picked !== null}
                onOpenChange={(next) => {
                    if (!next) setPicked(null);
                }}
                pluginID={picked?.plugin_id ?? ""}
                pluginVersion={picked?.version ?? ""}
                pluginName={picked?.name ?? ""}
                declared={pickedDeclared(picked)}
                onApprove={(granted) => {
                    if (!picked) return;
                    install.mutate({
                        pluginID: picked.plugin_id,
                        version: picked.version,
                        granted,
                    });
                }}
            />
        </>
    );
}

function pickedDeclared(p: MarketplacePlugin | null): DeclaredCapability[] {
    if (!p) return [];
    // The catalog row only carries flat family names — the structured
    // upper bound (paths/commands/hosts) lives in the manifest, which
    // the catalog doesn't currently store. The dialog renders fine
    // with just the family list; a future iteration can fetch the
    // manifest at pick-time to populate scope info.
    return p.capabilities.map((family) => ({ family }));
}

function PickerBody({
    query,
    setQuery,
    plugins,
    isLoading,
    error,
    onPick,
}: {
    query: string;
    setQuery: (q: string) => void;
    plugins: MarketplacePlugin[];
    isLoading: boolean;
    error: unknown;
    onPick: (p: MarketplacePlugin) => void;
}) {
    return (
        <div style={{ display: "flex", flexDirection: "column", gap: space[2] }}>
            <Input
                placeholder="Search by plugin name…"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                autoFocus
            />
            {isLoading ? (
                <div style={{ display: "flex", justifyContent: "center", padding: space[5] }}>
                    <Loader2 className="size-5 animate-spin" />
                </div>
            ) : error ? (
                <EmptyState
                    title="Couldn't load marketplace"
                    description={humanizeError(error)}
                />
            ) : plugins.length === 0 ? (
                <EmptyState
                    title="No plugins"
                    description={
                        query
                            ? `No plugin matches "${query}".`
                            : "The marketplace catalog is empty. Sync it from /marketplace first."
                    }
                />
            ) : (
                <ScrollArea className="max-h-[50vh]">
                    <ul
                        style={{
                            listStyle: "none",
                            margin: 0,
                            padding: 0,
                            display: "flex",
                            flexDirection: "column",
                            gap: space[1],
                        }}
                    >
                        {plugins.map((p) => (
                            <li key={`${p.plugin_id}@${p.version}`}>
                                <button
                                    type="button"
                                    onClick={() => onPick(p)}
                                    style={{
                                        width: "100%",
                                        textAlign: "left",
                                        border: `1px solid ${palette.border}`,
                                        borderRadius: 8,
                                        padding: space[3],
                                        background: palette.surface,
                                        cursor: "pointer",
                                        display: "flex",
                                        flexDirection: "column",
                                        gap: 2,
                                    }}
                                >
                                    <div
                                        style={{
                                            display: "flex",
                                            justifyContent: "space-between",
                                            alignItems: "baseline",
                                        }}
                                    >
                                        <span
                                            style={{
                                                fontSize: 13,
                                                fontWeight: 600,
                                                color: palette.textPrimary,
                                            }}
                                        >
                                            {p.name}
                                        </span>
                                        <span
                                            style={{
                                                fontSize: 11,
                                                color: palette.textMuted,
                                            }}
                                        >
                                            v{p.version}
                                        </span>
                                    </div>
                                    <div
                                        style={{
                                            fontSize: 11,
                                            color: palette.textMuted,
                                            fontFamily: "monospace",
                                        }}
                                    >
                                        {p.plugin_id}
                                    </div>
                                    {p.description && (
                                        <div
                                            style={{
                                                fontSize: 12,
                                                color: palette.textSecondary,
                                            }}
                                        >
                                            {p.description}
                                        </div>
                                    )}
                                </button>
                            </li>
                        ))}
                    </ul>
                </ScrollArea>
            )}
        </div>
    );
}

function InstallingView() {
    return (
        <div
            style={{
                display: "flex",
                flexDirection: "column",
                alignItems: "center",
                gap: space[2],
                padding: space[5],
            }}
        >
            <Loader2 className="size-6 animate-spin" />
            <span style={{ fontSize: 13, color: palette.textMuted }}>
                Installing — verifying signature, extracting, loading…
            </span>
        </div>
    );
}

function ProgressView({
    result,
    onClose,
    onReset,
}: {
    result: InstallResult;
    onClose: () => void;
    onReset: () => void;
}) {
    const ok = result.status === "installed";
    return (
        <div style={{ display: "flex", flexDirection: "column", gap: space[3] }}>
            <div
                style={{
                    display: "flex",
                    alignItems: "center",
                    gap: space[2],
                    padding: space[3],
                    border: `1px solid ${palette.border}`,
                    borderRadius: 8,
                    background: palette.surface,
                }}
            >
                {ok ? (
                    <CheckCircle2 className="size-5 text-green-600" />
                ) : (
                    <XCircle className="size-5 text-red-600" />
                )}
                <div style={{ display: "flex", flexDirection: "column" }}>
                    <span style={{ fontSize: 13, fontWeight: 600 }}>
                        {ok ? "Installed" : "Install failed"}
                    </span>
                    <span style={{ fontSize: 11, color: palette.textMuted }}>
                        {result.plugin_id} v{result.version}
                    </span>
                </div>
            </div>
            <ol
                style={{
                    margin: 0,
                    padding: 0,
                    listStyle: "none",
                    display: "flex",
                    flexDirection: "column",
                    gap: 2,
                }}
            >
                {result.progress.map((p, i) => (
                    <li
                        key={i}
                        style={{
                            fontSize: 11,
                            fontFamily: "monospace",
                            color: phaseColor(p),
                        }}
                    >
                        <PhaseLine progress={p} />
                    </li>
                ))}
            </ol>
            <div style={{ display: "flex", justifyContent: "flex-end", gap: space[2] }}>
                {!ok && (
                    <Button variant="outline" onClick={onReset}>
                        Try another
                    </Button>
                )}
                <Button onClick={onClose}>Close</Button>
            </div>
        </div>
    );
}

function PhaseLine({ progress }: { progress: InstallProgress }) {
    const phaseLabel = progress.phase.replace(/^PHASE_/, "");
    if (progress.error_message) {
        return (
            <span>
                {phaseLabel}: {progress.error_code}: {progress.error_message}
            </span>
        );
    }
    if (progress.bytes_total) {
        return (
            <span>
                {phaseLabel} — {progress.bytes_done ?? 0} / {progress.bytes_total} bytes
            </span>
        );
    }
    return <span>{phaseLabel}</span>;
}

function phaseColor(p: InstallProgress): string {
    if (p.phase.endsWith("FAILED")) return palette.danger;
    if (p.phase.endsWith("INSTALLED")) return palette.success ?? palette.textPrimary;
    return palette.textMuted;
}
