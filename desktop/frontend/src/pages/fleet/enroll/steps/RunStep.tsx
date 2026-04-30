import { useMemo, useState } from "react";
import { Copy } from "lucide-react";
import { toast } from "sonner";

import Mono from "../../../../components/Mono";
import { palette } from "../../../../layout/theme";
import { IssueInstallResponse } from "../../../../lib/api";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import DownloaderPicker, {
    bundleOneLinerFor,
    defaultDownloader,
} from "../DownloaderPicker";

interface Props {
    result: IssueInstallResponse;
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
export default function RunStep({ result }: Props) {
    const [tab, setTab] = useState<"script" | "bundle">("script");
    const [downloader, setDownloader] = useState<string>(
        defaultDownloader(result.target_os),
    );
    const isWindows = (result.target_os ?? "").toLowerCase() === "windows";
    const scriptTabLabel = isWindows ? "iwr | iex" : "curl | sh";

    // Backwards-compat: older server builds that haven't shipped
    // install_commands yet only return install_command. Synthesise a
    // single-entry map keyed by the family default so the picker
    // still has something to select (and renders that default).
    const commands = useMemo<Record<string, string>>(() => {
        if (result.install_commands && Object.keys(result.install_commands).length > 0) {
            return result.install_commands;
        }
        return { [defaultDownloader(result.target_os)]: result.install_command };
    }, [result.install_commands, result.install_command, result.target_os]);

    const scriptOneLiner = commands[downloader] ?? result.install_command;
    const bundleOneLiner = bundleOneLinerFor(downloader, result.bundle_url);

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
                    <DownloaderRow
                        downloader={downloader}
                        onChange={setDownloader}
                        available={Object.keys(commands)}
                        targetOS={result.target_os}
                    />
                    <CodeBlock>{scriptOneLiner}</CodeBlock>
                    <CopyRow text={scriptOneLiner} onCopy={copy} />
                </TabsContent>
                <TabsContent value="bundle" className="mt-3 space-y-2">
                    <div style={{ fontSize: 12, color: palette.textMuted }}>
                        {bundleHint}
                    </div>
                    <DownloaderRow
                        downloader={downloader}
                        onChange={setDownloader}
                        available={Object.keys(commands)}
                        targetOS={result.target_os}
                    />
                    <CodeBlock>{bundleOneLiner}</CodeBlock>
                    <CopyRow text={bundleOneLiner} onCopy={copy} />
                </TabsContent>
            </Tabs>
        </div>
    );
}

function DownloaderRow({
    downloader,
    onChange,
    available,
    targetOS,
}: {
    downloader: string;
    onChange: (next: string) => void;
    available: string[];
    targetOS?: string;
}) {
    return (
        <div className="flex items-center gap-2">
            <span style={{ fontSize: 12, color: palette.textMuted }}>
                Downloader
            </span>
            <DownloaderPicker
                value={downloader}
                onChange={onChange}
                available={available}
                targetOS={targetOS}
            />
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
