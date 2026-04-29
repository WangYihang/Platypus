import { useState } from "react";
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

    async function copy(text: string) {
        await navigator.clipboard.writeText(text);
        toast.success("Copied to clipboard");
    }

    function done() {
        onClose();
        navigate(`/projects/${projectSlug}/fleet?await=enroll`);
    }

    const bundleOneLiner = result
        ? `platypus-agent "$(curl -fsSL ${result.bundle_url})"`
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
                        <TabsTrigger value="script">curl | sh</TabsTrigger>
                        <TabsTrigger value="bundle">offline bundle</TabsTrigger>
                    </TabsList>
                    <TabsContent value="script" className="mt-3 space-y-2">
                        <div className="text-xs text-text-muted">
                            Run on the target machine. Downloads the agent binary, then
                            enrols using the freshly-minted PAT.
                        </div>
                        <pre className="rounded border border-border bg-surface p-3 font-mono text-xs break-all whitespace-pre-wrap">
                            {result?.install_command}
                        </pre>
                    </TabsContent>
                    <TabsContent value="bundle" className="mt-3 space-y-2">
                        <div className="text-xs text-text-muted">
                            For air-gapped or scripted bootstraps where{" "}
                            <Mono>| sh</Mono> isn't appropriate. <Mono>curl</Mono> returns
                            a self-contained <Mono>pinst_</Mono> token; pipe it straight
                            to <Mono>platypus-agent</Mono> (binary must already be on the
                            host).
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
                            copy(
                                tab === "script"
                                    ? result?.install_command ?? ""
                                    : bundleOneLiner,
                            )
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
