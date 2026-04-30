import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "@/components/ui/select";

// Backend ships these names in IssueInstallResponse.install_commands
// (see internal/api/install_downloaders.go). Keys here are the source
// of truth for what the dropdown can show; missing keys in
// install_commands hide that row automatically.
const DISPLAY_LABELS: Record<string, string> = {
    curl: "curl",
    wget: "wget",
    python3: "python3",
    php: "php",
    ruby: "ruby",
    powershell: "PowerShell (Windows PS 5.1+)",
    pwsh: "pwsh (PowerShell 7+)",
};

// Per-OS-family ordering. The first entry is the family default —
// matches the server's defaultCmd selection so the picker opens on
// the same one-liner the legacy install_command field carries.
const UNIX_ORDER = ["curl", "wget", "python3", "php", "ruby"];
const WINDOWS_ORDER = ["powershell", "pwsh"];

export function downloaderOrder(targetOS?: string): string[] {
    return isWindowsTarget(targetOS) ? WINDOWS_ORDER : UNIX_ORDER;
}

export function defaultDownloader(targetOS?: string): string {
    return isWindowsTarget(targetOS) ? "powershell" : "curl";
}

function isWindowsTarget(targetOS?: string): boolean {
    return (targetOS ?? "").toLowerCase() === "windows";
}

interface Props {
    value: string;
    onChange: (next: string) => void;
    // Names available in the current install_commands map. We
    // intersect with DISPLAY_LABELS / family order so a server that
    // ships a new downloader we don't yet have a label for still
    // shows the raw key rather than disappearing entirely.
    available: string[];
    targetOS?: string;
}

// DownloaderPicker is the dropdown the wizard's RunStep renders above
// the install one-liner code block. Lets the operator switch between
// curl / wget / python3 / etc. without re-issuing the (single-use)
// install token. The same picker is reused by IssuedInstallDialog.
export default function DownloaderPicker({ value, onChange, available, targetOS }: Props) {
    const order = downloaderOrder(targetOS);
    const ordered = [
        ...order.filter((k) => available.includes(k)),
        // Surface server-side downloaders we don't have an explicit
        // order entry for at the bottom — keeps the picker honest if
        // the server adds one ahead of the FE.
        ...available.filter((k) => !order.includes(k)),
    ];
    return (
        <Select value={value} onValueChange={onChange}>
            <SelectTrigger size="sm" style={{ minWidth: 200 }} data-testid="downloader-picker">
                <SelectValue />
            </SelectTrigger>
            <SelectContent>
                {ordered.map((k) => (
                    <SelectItem key={k} value={k}>
                        {DISPLAY_LABELS[k] ?? k}
                    </SelectItem>
                ))}
            </SelectContent>
        </Select>
    );
}

// bundleOneLinerFor wraps the bundle URL in the equivalent of
// `platypus-agent "$(<downloader-cmd> <url>)"` for whichever
// downloader the operator picked. The `insecure` flag mirrors the
// backend registry: true emits skip-TLS-verification flavour
// (default for self-signed install endpoints), false emits a strict
// flavour that relies on the host's system trust store.
//
// On Windows the TLS 1.2 force stays in BOTH flavours — Windows
// PowerShell 5.1 defaults to TLS 1.0 and would otherwise fail with
// "Could not create SSL/TLS secure channel" before cert validation
// even runs.
export function bundleOneLinerFor(
    downloader: string,
    bundleURL: string,
    insecure: boolean,
): string {
    switch (downloader) {
        case "wget":
            return insecure
                ? `platypus-agent "$(wget -qO- --no-check-certificate ${bundleURL})"`
                : `platypus-agent "$(wget -qO- ${bundleURL})"`;
        case "python3":
            return insecure
                ? `platypus-agent "$(python3 -c "import ssl,urllib.request as u;print(u.urlopen('${bundleURL}',context=ssl._create_unverified_context()).read().decode())")"`
                : `platypus-agent "$(python3 -c "import urllib.request as u;print(u.urlopen('${bundleURL}').read().decode())")"`;
        case "php":
            return insecure
                ? `platypus-agent "$(php -r "echo file_get_contents('${bundleURL}',false,stream_context_create(['ssl'=>['verify_peer'=>false,'verify_peer_name'=>false]]));")"`
                : `platypus-agent "$(php -r "echo file_get_contents('${bundleURL}');")"`;
        case "ruby":
            return insecure
                ? `platypus-agent "$(ruby -ropen-uri -e "puts URI.open('${bundleURL}',ssl_verify_mode: 0).read")"`
                : `platypus-agent "$(ruby -ropen-uri -e "puts URI.open('${bundleURL}').read")"`;
        case "powershell":
            return insecure
                ? `& { [Net.ServicePointManager]::SecurityProtocol=[Net.SecurityProtocolType]::Tls12; [Net.ServicePointManager]::ServerCertificateValidationCallback={$true}; & platypus-agent.exe (Invoke-RestMethod -UseBasicParsing -Uri '${bundleURL}') }`
                : `& { [Net.ServicePointManager]::SecurityProtocol=[Net.SecurityProtocolType]::Tls12; & platypus-agent.exe (Invoke-RestMethod -UseBasicParsing -Uri '${bundleURL}') }`;
        case "pwsh":
            return insecure
                ? `& { [Net.ServicePointManager]::SecurityProtocol=[Net.SecurityProtocolType]::Tls12; & platypus-agent.exe (Invoke-RestMethod -SkipCertificateCheck -UseBasicParsing -Uri '${bundleURL}') }`
                : `& { [Net.ServicePointManager]::SecurityProtocol=[Net.SecurityProtocolType]::Tls12; & platypus-agent.exe (Invoke-RestMethod -UseBasicParsing -Uri '${bundleURL}') }`;
        case "curl":
        default:
            return insecure
                ? `platypus-agent "$(curl -fsSL --tlsv1.2 -k ${bundleURL})"`
                : `platypus-agent "$(curl -fsSL --tlsv1.2 ${bundleURL})"`;
    }
}
