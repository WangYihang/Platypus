import {
    Boxes,
    HelpCircle,
    Laptop,
    Layers,
    Monitor,
    Server,
} from "lucide-react";

import Mono from "../../components/Mono";
import StatusPill from "../../components/StatusPill";
import { palette, space } from "../../layout/theme";

// Small presentational helpers shared across the Host Info / Sessions
// surfaces. Co-located so adding a new pill / status mapping doesn't
// have to re-thread imports across each card file.

// ApprovalStatusPill renders the host's approval_status on the host
// detail KPI strip. Each status maps to a tone in the existing
// vocabulary so it visually clusters with other badges (online /
// offline / fingerprint fallback) on the same row.
export function ApprovalStatusPill({
    status,
}: {
    status: "pending" | "approved" | "rejected";
}) {
    if (status === "approved") {
        return <StatusPill tone="success">approved</StatusPill>;
    }
    if (status === "rejected") {
        return <StatusPill tone="danger">rejected</StatusPill>;
    }
    return <StatusPill tone="warning">pending approval</StatusPill>;
}

// BuildVersionValue renders the host's running agent build_version
// alongside an outdated/up-to-date indicator. Three visual states:
//
//   1. "—"        host hasn't reported a build_version yet (pre-
//                  versioning agent or not enrolled)
//   2. "1.6.0" + green "up to date" — host matches manifest head
//   3. "1.5.1" + amber "outdated · latest 1.6.0" — host trails
//
// `latest` is the current channel head from /v1/install/platforms;
// when it's empty (no manifest published, distributor disabled) we
// can't compare and just render the version with no pill.
export function BuildVersionValue({
    version,
    latest,
}: {
    version: string | undefined;
    latest: string | undefined;
}) {
    if (!version) return <Dim>—</Dim>;
    const cmp = !latest ? "unknown" : version === latest ? "match" : "mismatch";
    return (
        <span style={{ display: "inline-flex", alignItems: "center", gap: space[2] }}>
            <Mono>{version}</Mono>
            {cmp === "match" && <StatusPill tone="success">up to date</StatusPill>}
            {cmp === "mismatch" && (
                <StatusPill tone="warning">{`outdated · latest ${latest}`}</StatusPill>
            )}
        </span>
    );
}

// machineTypeMeta maps Host.machine_type to a label + lucide icon.
// Inlined here (not in lib/icons) because the rendering shape is
// specific to the inline pill — the standalone Fleet card grid uses
// a smaller variant in pages/fleet/cards/machineIcons.tsx.
const machineTypeMeta: Record<
    string,
    { label: string; Icon: React.ComponentType<{ className?: string }> }
> = {
    container: { label: "container", Icon: Boxes },
    vm: { label: "virtual machine", Icon: Layers },
    bare_metal: { label: "bare metal", Icon: Server },
    laptop: { label: "laptop", Icon: Laptop },
    desktop: { label: "desktop", Icon: Monitor },
    unknown: { label: "unknown", Icon: HelpCircle },
};

export function MachineTypePill({ type }: { type?: string }) {
    const meta = type ? machineTypeMeta[type] : undefined;
    if (!meta) return <>—</>;
    const { label, Icon } = meta;
    return (
        <span style={{ display: "inline-flex", alignItems: "center", gap: space[2] }}>
            <Icon className="size-3.5" />
            <span>{label}</span>
        </span>
    );
}

function Dim({ children }: { children: React.ReactNode }) {
    return <span style={{ color: palette.textSecondary }}>{children}</span>;
}
