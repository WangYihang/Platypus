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
//  • script  — `curl … | sh`  (or `iwr … | iex` on Windows)
//  • bundle  — `platypus-agent "$(curl …)"` (or PowerShell
//                                            equivalent on Windows)
//
// Both consume the same single-use install token, so the operator
// picks one — never both. After this step the wizard's "Done"
// button arms the EnrollmentWaitBanner via `?await=enroll`; the
// poll loop there picks up whichever shape was actually run.
//
// We branch on result.target_os when rendering the bundle one-liner
// because curl is not guaranteed on stock Windows. The script tab
// just echoes whatever the backend returned — it already picks the
// right shell for us based on target_os when the install token was
// minted.
export default function RunStep({ result }: Props) {
    const [tab, setTab] = useState<"script" | "bundle">("script");
    const isWindows = result.target_os === "windows";
    const scriptTabLabel = isWindows ? "iwr | iex" : "curl | sh";
    const bundleOneLiner = isWindows
        ? `& platypus-agent.exe (Invoke-RestMethod -UseBasicParsing -Uri '${result.bundle_url}')`
        : `platypus-agent "$(curl -fsSL ${result.bundle_url})"`;
    const bundleHint = isWindows ? (
        <>
            For air-gapped or scripted bootstraps where{" "}
            <Mono>iwr | iex</Mono> isn't appropriate.{" "}
            <Mono>Invoke-RestMethod</Mono> returns a self-contained{" "}
            <Mono>pinst_</Mono> token; pass it straight to{" "}
            <Mono>platypus-agent.exe</Mono>.
        </>
    ) : (
        <>
            For air-gapped or scripted bootstraps where <Mono>| sh</Mono>{" "}
            isn't appropriate. <Mono>curl</Mono> returns a self-contained{" "}
            <Mono>pinst_</Mono> token; pipe it straight to{" "}
            <Mono>platypus-agent</Mono>.
        </>
    );

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
                    <TabsTrigger value="script">{scriptTabLabel}</TabsTrigger>
                    <TabsTrigger value="bundle">offline bundle</TabsTrigger>
                </TabsList>
                <TabsContent value="script" className="mt-3 space-y-2">
                    <CodeBlock>{result.install_command}</CodeBlock>
                    <CopyRow text={result.install_command} onCopy={copy} />
                </TabsContent>
                <TabsContent value="bundle" className="mt-3 space-y-2">
                    <div style={{ fontSize: 12, color: palette.textMuted }}>
                        {bundleHint}
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
