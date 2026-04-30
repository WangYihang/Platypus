import { Switch } from "@/components/ui/switch";
import { palette } from "../../../../layout/theme";

interface Props {
    skipTLS: boolean;
    onSkipTLSChange: (v: boolean) => void;
}

export default function DownloadTLSStep({ skipTLS, onSkipTLSChange }: Props) {
    return (
        <div className="space-y-3" data-testid="enroll-wizard-download-tls">
            <div style={{ fontSize: 13, color: palette.textSecondary }}>
                Choose whether install download commands skip TLS certificate
                verification.
            </div>
            <label
                className="flex items-center gap-2"
                style={{ fontSize: 13, color: palette.textSecondary, cursor: "pointer" }}
                title="This choice controls server-rendered command flavor and the install script's internal downloader trust mode."
            >
                <Switch checked={skipTLS} onCheckedChange={onSkipTLSChange} />
                Skip TLS verification
            </label>
            <div style={{ fontSize: 11, color: palette.textMuted }}>
                Keep enabled for self-signed bootstraps. Disable for production
                certificates trusted by the OS.
            </div>
        </div>
    );
}
