import { Loader2, XCircle } from "lucide-react";

import { palette, space } from "../../layout/theme";
import { hostnameFromURL } from "../../lib/servers";
import { PublicServerInfo } from "../../lib/auth";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

import { Field } from "./common";

interface Props {
    url: string;
    name: string;
    probing: boolean;
    probeError: string | null;
    probeInfo: PublicServerInfo | null;
    // True when the last probe attempt failed against this exact URL.
    blocked: boolean;
    onURL: (v: string) => void;
    onName: (v: string) => void;
    onBack: () => void;
    onNext: () => void;
}

export default function ProbeStep({
    url,
    name,
    probing,
    probeError,
    blocked,
    onURL,
    onName,
    onBack,
    onNext,
}: Props) {
    return (
        <div style={{ display: "flex", flexDirection: "column", gap: space[4] }}>
            <div>
                <h2 style={{ margin: 0, fontSize: 20, fontWeight: 600 }}>Your server</h2>
                <p
                    style={{
                        margin: `${space[1]}px 0 0`,
                        color: palette.textSecondary,
                        fontSize: 13,
                    }}
                >
                    Paste the server URL. We'll probe it and pick the right next
                    step automatically.
                </p>
            </div>
            <Field label="Server URL">
                <Input
                    value={url}
                    onChange={(e) => onURL(e.target.value)}
                    placeholder="https://localhost:9443"
                    autoFocus
                    data-testid="onboarding-url"
                />
            </Field>
            <Field label="Display name (optional)">
                <Input
                    value={name}
                    onChange={(e) => onName(e.target.value)}
                    placeholder={hostnameFromURL(url)}
                    data-testid="onboarding-name"
                />
            </Field>
            {probeError && (
                <div
                    style={{
                        display: "flex",
                        alignItems: "center",
                        gap: space[2],
                        color: palette.danger,
                        fontSize: 12,
                    }}
                >
                    <XCircle className="size-3.5" />
                    <span>{probeError}</span>
                </div>
            )}
            <div style={{ display: "flex", gap: space[2] }}>
                <Button variant="outline" onClick={onBack}>
                    Back
                </Button>
                <Button
                    onClick={onNext}
                    disabled={!url || probing || blocked}
                    style={{ marginLeft: "auto" }}
                    data-testid="onboarding-probe"
                >
                    {probing && <Loader2 className="size-3.5 animate-spin" />}
                    Continue
                </Button>
            </div>
        </div>
    );
}
