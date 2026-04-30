import { useMemo, useState } from "react";
import { Copy } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";

import Mono from "../../../components/Mono";
import { IssueInstallResponse } from "../../../lib/api";
import { Button } from "@/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import DownloaderPicker, {
    bundleOneLinerFor,
    defaultDownloader,
} from "../../fleet/enroll/DownloaderPicker";

interface Props {
    result: IssueInstallResponse | null;
    projectSlug: string;
    onClose: () => void;
}

// IssuedInstallDialog renders the freshly-issued install command in
// two parallel shapes:
//
//  • script — `curl ... | sh` — the default; downloads the agent
//    binary + runs it. Fastest path on a stock Linux box.
//  • bundle — `platypus-agent <pinst_…>` — for offline / scripted
//    flows (Ansible, cloud-init, air-gapped) where shell-script
//    execution isn't desirable. The agent binary is presumed to
//    already be on the box.
//
// Both consume the same single-use install token, so the operator
// picks one or the other but never both. After dismissing the
// dialog, the page redirects to Fleet with `?await=enroll` so the
// EnrollmentWaitBanner picks up the next dial-back.
export default function IssuedInstallDialog({
    result,
    projectSlug,
    onClose,
}: Props) {
    const navigate = useNavigate();
    const [tab, setTab] = useState<"script" | "bundle">("script");
    const [downloader, setDownloader] = useState<string>(
        defaultDownloader(result?.target_os),
    );

    async function copy(text: string) {
        await navigator.clipboard.writeText(text);
        toast.success("Copied to clipboard");
    }

    function done() {
        onClose();
        navigate(`/projects/${projectSlug}/hosts?await=enroll`);
    }

    // Backwards-compat with older server builds: synthesise a
    // single-entry map from install_command when install_commands is
    // missing, so the picker still has something to select.
    const commands = useMemo<Record<string, string>>(() => {
        if (!result) return {};
        if (
            result.install_commands &&
            Object.keys(result.install_commands).length > 0
        ) {
            return result.install_commands;
        }
        return { [defaultDownloader(result.target_os)]: result.install_command };
    }, [result]);

    const scriptOneLiner = result
        ? commands[downloader] ?? result.install_command
        : "";
    const bundleOneLiner = result
        ? bundleOneLinerFor(downloader, result.bundle_url)
        : "";

    return (
        <Dialog open={result !== null} onOpenChange={(o) => !o && onClose()}>
            <DialogContent className="sm:max-w-[640px]">
                <DialogHeader>
                    <DialogTitle>Install command generated</DialogTitle>
                    <DialogDescription>
                        This is the only time the command is shown. After closing, the
                        server discards the plaintext.
                    </DialogDescription>
                </DialogHeader>
                <Tabs value={tab} onValueChange={(v) => setTab(v as "script" | "bundle")}>
                    <TabsList>
                        <TabsTrigger value="script">script | sh</TabsTrigger>
                        <TabsTrigger value="bundle">offline bundle</TabsTrigger>
                    </TabsList>
                    <TabsContent value="script" className="mt-3 space-y-2">
                        <div className="text-xs text-text-muted">
                            Run on the target machine. Downloads the agent binary, then
                            enrols using the freshly-minted PAT.
                        </div>
                        <div className="flex items-center gap-2">
                            <span className="text-xs text-text-muted">Downloader</span>
                            <DownloaderPicker
                                value={downloader}
                                onChange={setDownloader}
                                available={Object.keys(commands)}
                                targetOS={result?.target_os}
                            />
                        </div>
                        <pre className="rounded border border-border bg-surface p-3 font-mono text-xs break-all whitespace-pre-wrap">
                            {scriptOneLiner}
                        </pre>
                    </TabsContent>
                    <TabsContent value="bundle" className="mt-3 space-y-2">
                        <div className="text-xs text-text-muted">
                            For air-gapped or scripted bootstraps where{" "}
                            <Mono>| sh</Mono> isn't appropriate. The downloader returns
                            a self-contained <Mono>pinst_</Mono> token; pipe it straight
                            to <Mono>platypus-agent</Mono> (binary must already be on the
                            host).
                        </div>
                        <div className="flex items-center gap-2">
                            <span className="text-xs text-text-muted">Downloader</span>
                            <DownloaderPicker
                                value={downloader}
                                onChange={setDownloader}
                                available={Object.keys(commands)}
                                targetOS={result?.target_os}
                            />
                        </div>
                        <pre className="rounded border border-border bg-surface p-3 font-mono text-xs break-all whitespace-pre-wrap">
                            {bundleOneLiner}
                        </pre>
                    </TabsContent>
                </Tabs>
                <div className="text-xs text-text-muted">
                    After running it on the host, return to Fleet — new agents appear
                    there automatically within a few seconds.
                </div>
                <DialogFooter>
                    <Button
                        variant="outline"
                        onClick={() =>
                            copy(tab === "script" ? scriptOneLiner : bundleOneLiner)
                        }
                    >
                        <Copy className="size-3.5" />
                        Copy
                    </Button>
                    <Button onClick={done}>I'll run this — show me Fleet</Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}
