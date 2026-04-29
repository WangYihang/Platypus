import { useState } from "react";
import { Copy } from "lucide-react";
import { toast } from "sonner";

import Mono from "../../../../components/Mono";
import { palette } from "../../../../layout/theme";
import { IssueInstallResponse } from "../../../../lib/api";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

interface Props {
    result: IssueInstallResponse;
}

// RunStep is the terminal step of the EnrollAgentWizard. It renders
// the freshly-issued install command in two parallel shapes:
//
//  • script  — `curl … | sh`             (the default; fastest path)
//  • bundle  — `platypus-agent "$(curl …)"` (for air-gapped /
//                                            scripted bootstraps)
//
// Both consume the same single-use install token, so the operator
// picks one — never both. After this step the wizard's "Done"
// button arms the EnrollmentWaitBanner via `?await=enroll`; the
// poll loop there picks up whichever shape was actually run.
export default function RunStep({ result }: Props) {
    const [tab, setTab] = useState<"script" | "bundle">("script");
    const bundleOneLiner = `platypus-agent "$(curl -fsSL ${result.bundle_url})"`;

    async function copy(text: string) {
        await navigator.clipboard.writeText(text);
        toast.success("Copied to clipboard");
    }

    return (
        <div className="space-y-3" data-testid="enroll-wizard-run">
            <div style={{ fontSize: 13, color: palette.textSecondary }}>
                Run this on the target machine. The agent appears in Fleet
                automatically once it dials back — usually within 10 seconds.
            </div>
            <Tabs value={tab} onValueChange={(v) => setTab(v as "script" | "bundle")}>
                <TabsList>
                    <TabsTrigger value="script">curl | sh</TabsTrigger>
                    <TabsTrigger value="bundle">offline bundle</TabsTrigger>
                </TabsList>
                <TabsContent value="script" className="mt-3 space-y-2">
                    <CodeBlock>{result.install_command}</CodeBlock>
                    <CopyRow text={result.install_command} onCopy={copy} />
                </TabsContent>
                <TabsContent value="bundle" className="mt-3 space-y-2">
                    <div style={{ fontSize: 12, color: palette.textMuted }}>
                        For air-gapped or scripted bootstraps where{" "}
                        <Mono>| sh</Mono> isn't appropriate. <Mono>curl</Mono> returns
                        a self-contained <Mono>pinst_</Mono> token; pipe it straight
                        to <Mono>platypus-agent</Mono>.
                    </div>
                    <CodeBlock>{bundleOneLiner}</CodeBlock>
                    <CopyRow text={bundleOneLiner} onCopy={copy} />
                </TabsContent>
            </Tabs>
        </div>
    );
}

function CodeBlock({ children }: { children: React.ReactNode }) {
    return (
        <pre className="rounded border border-border bg-surface p-3 font-mono text-xs break-all whitespace-pre-wrap">
            {children}
        </pre>
    );
}

function CopyRow({
    text,
    onCopy,
}: {
    text: string;
    onCopy: (text: string) => void;
}) {
    return (
        <div style={{ display: "flex", justifyContent: "flex-end" }}>
            <Button variant="outline" size="sm" onClick={() => onCopy(text)}>
                <Copy className="size-3.5" />
                Copy
            </Button>
        </div>
    );
}
