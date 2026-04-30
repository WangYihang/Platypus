import { useMemo, useState } from "react";
import { Copy } from "lucide-react";
import { toast } from "sonner";

import Mono from "../../../../components/Mono";
import { palette } from "../../../../layout/theme";
import { IssueInstallResponse } from "../../../../lib/api";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import DownloaderPicker, { defaultDownloader } from "../DownloaderPicker";

interface Props {
    result: IssueInstallResponse;
    initialSkipTLS?: boolean;
}

// RunStep is the terminal step of the EnrollAgentWizard. It renders
// the freshly-issued install command in two parallel shapes:
//
//  • script  — `<downloader> ... | sh` (or `iwr | iex` on Windows)
//  • bundle  — `platypus-agent "$(<downloader> ...)"`
//
// Both consume the same single-use install token, so the operator
// picks one — never both. The downloader sub-picker on each tab lets
// them switch between curl / wget / python3 / php / ruby (unix) or
// powershell / pwsh (windows) without re-issuing the token, which
// matters when their target machine's default downloader (e.g.
// macOS's LibreSSL-linked curl) is broken against the server's TLS
// cert. After this step the wizard's "Done" button arms the
// EnrollmentWaitBanner via `?await=enroll`; the poll loop there
// picks up whichever shape was actually run.
export default function RunStep({ result, initialSkipTLS = true }: Props) {
    const [tab, setTab] = useState<"script" | "bundle">("script");
    const [downloader, setDownloader] = useState<string>(
        defaultDownloader(result.target_os),
    );
    // Default ON: most Platypus deployments first-boot with a
    // self-signed cert, so the rendered command needs to skip
    // verification to actually run. Operators with a real cert turn
    // it off to copy a stricter, MITM-resistant one-liner.
    const [skipTLS, setSkipTLS] = useState<boolean>(initialSkipTLS);
    const isWindows = (result.target_os ?? "").toLowerCase() === "windows";
    const scriptTabLabel = isWindows ? "iwr | iex" : "curl | sh";

    // Pick the per-flavour map for both the script and bundle tabs.
    // Both are server-rendered so the FE just selects by downloader
    // key. If the server is on an older build that hasn't shipped
    // these maps yet, fall back to the legacy single-string fields
    // (script) and degrade the bundle one-liner to "unsupported on
    // this server" — strictly cosmetic since the backend would also
    // be missing the wizard for them.
    const scriptCommands = useMemo<Record<string, string>>(() => {
        const fromServer = skipTLS
            ? result.install_commands
            : result.install_commands_strict;
        if (fromServer && Object.keys(fromServer).length > 0) {
            return fromServer;
        }
        return { [defaultDownloader(result.target_os)]: result.install_command };
    }, [
        skipTLS,
        result.install_commands,
        result.install_commands_strict,
        result.install_command,
        result.target_os,
    ]);

    const bundleCommands = useMemo<Record<string, string>>(() => {
        const fromServer = skipTLS
            ? result.bundle_commands
            : result.bundle_commands_strict;
        return fromServer ?? {};
    }, [skipTLS, result.bundle_commands, result.bundle_commands_strict]);

    const scriptOneLiner = scriptCommands[downloader] ?? result.install_command;
    const bundleOneLiner = bundleCommands[downloader] ?? "";

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
            isn't appropriate. The downloader returns a self-contained{" "}
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
                    <OptionsRow
                        downloader={downloader}
                        onDownloaderChange={setDownloader}
                        available={Object.keys(scriptCommands)}
                        targetOS={result.target_os}
                        skipTLS={skipTLS}
                        onSkipTLSChange={setSkipTLS}
                    />
                    <CodeBlock>{scriptOneLiner}</CodeBlock>
                    <CopyRow text={scriptOneLiner} onCopy={copy} />
                </TabsContent>
                <TabsContent value="bundle" className="mt-3 space-y-2">
                    <div style={{ fontSize: 12, color: palette.textMuted }}>
                        {bundleHint}
                    </div>
                    <OptionsRow
                        downloader={downloader}
                        onDownloaderChange={setDownloader}
                        available={Object.keys(scriptCommands)}
                        targetOS={result.target_os}
                        skipTLS={skipTLS}
                        onSkipTLSChange={setSkipTLS}
                    />
                    <CodeBlock>{bundleOneLiner}</CodeBlock>
                    <CopyRow text={bundleOneLiner} onCopy={copy} />
                </TabsContent>
            </Tabs>
        </div>
    );
}

function OptionsRow({
    downloader,
    onDownloaderChange,
    available,
    targetOS,
    skipTLS,
    onSkipTLSChange,
}: {
    downloader: string;
    onDownloaderChange: (next: string) => void;
    available: string[];
    targetOS?: string;
    skipTLS: boolean;
    onSkipTLSChange: (next: boolean) => void;
}) {
    return (
        <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
            <div className="flex items-center gap-2">
                <span style={{ fontSize: 12, color: palette.textMuted }}>
                    Downloader
                </span>
                <DownloaderPicker
                    value={downloader}
                    onChange={onDownloaderChange}
                    available={available}
                    targetOS={targetOS}
                />
            </div>
            <label
                className="flex items-center gap-2"
                style={{ fontSize: 12, color: palette.textMuted, cursor: "pointer" }}
                title="Skip TLS verification on the install endpoint. Default ON because most Platypus servers first-boot with a self-signed cert. Turn off when the server has a real, system-trusted cert."
            >
                <Switch
                    checked={skipTLS}
                    onCheckedChange={onSkipTLSChange}
                    data-testid="skip-tls-toggle"
                />
                Skip TLS verification
            </label>
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
