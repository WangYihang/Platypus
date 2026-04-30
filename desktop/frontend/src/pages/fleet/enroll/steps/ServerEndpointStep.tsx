import { palette } from "../../../../layout/theme";
import { Input } from "@/components/ui/input";

interface Props {
    serverEndpoint: string;
    onChange: (v: string) => void;
}

export default function ServerEndpointStep({ serverEndpoint, onChange }: Props) {
    return (
        <div className="space-y-3" data-testid="enroll-wizard-server">
            <div style={{ fontSize: 13, color: palette.textSecondary }}>
                Set the host:port the agent should dial after install.
            </div>
            <Input
                autoFocus
                placeholder="203.0.113.5:13337"
                value={serverEndpoint}
                onChange={(e) => onChange(e.target.value)}
            />
            <div style={{ fontSize: 11, color: palette.textMuted }}>
                Defaults to this server's unified ingress address.
            </div>
        </div>
    );
}
