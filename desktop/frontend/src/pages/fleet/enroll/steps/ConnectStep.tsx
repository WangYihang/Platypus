import Mono from "../../../../components/Mono";
import { palette, radius, space } from "../../../../layout/theme";
import { formatSeconds } from "../../../../lib/time";
import { Input } from "@/components/ui/input";

interface Props {
    serverEndpoint: string;
    onServerEndpointChange: (v: string) => void;
    ttlSeconds: number | undefined;
    onTtlSecondsChange: (v: number | undefined) => void;
    patMaxUses: number | undefined;
    onPatMaxUsesChange: (v: number | undefined) => void;
    autoApprove: boolean;
    onAutoApproveChange: (v: boolean) => void;
    description: string;
    onDescriptionChange: (v: string) => void;
}

// ConnectStep is step 3 of the EnrollAgentWizard — the only step
// with required input (`serverEndpoint`). Validation is handled by
// the WizardFooter (`Generate` is disabled when the field is empty)
// rather than inside this component, so the visual stays uncluttered
// and the orchestrator stays the source of truth for "can I move
// forward yet".
export default function ConnectStep({
    serverEndpoint,
    onServerEndpointChange,
    ttlSeconds,
    onTtlSecondsChange,
    patMaxUses,
    onPatMaxUsesChange,
    autoApprove,
    onAutoApproveChange,
    description,
    onDescriptionChange,
}: Props) {
    return (
        <div className="space-y-4" data-testid="enroll-wizard-connect">
            <Field label="Server endpoint" hint="host:port the agent will dial. Defaults to this server's unified ingress address.">
                <Input
                    autoFocus
                    placeholder="203.0.113.5:13337"
                    value={serverEndpoint}
                    onChange={(e) => onServerEndpointChange(e.target.value)}
                />
            </Field>

            <Field
                label="Expires in (seconds)"
                hint={
                    typeof ttlSeconds === "number" && ttlSeconds > 0
                        ? `How long the install URL stays valid. = ${formatSeconds(ttlSeconds)}`
                        : "How long the install URL stays valid. Default 300 (= 5m)."
                }
            >
                <Input
                    type="number"
                    inputMode="numeric"
                    placeholder="300 (= 5m)"
                    value={ttlSeconds ?? ""}
                    onChange={(e) =>
                        onTtlSecondsChange(
                            e.target.value === "" ? undefined : Number(e.target.value),
                        )
                    }
                />
            </Field>

            <Field
                label="PAT max uses"
                hint="How many times the enrollment token minted from this install link can be redeemed. Default 1."
            >
                <Input
                    type="number"
                    inputMode="numeric"
                    placeholder="1"
                    value={patMaxUses ?? ""}
                    onChange={(e) =>
                        onPatMaxUsesChange(
                            e.target.value === "" ? undefined : Number(e.target.value),
                        )
                    }
                />
            </Field>

            <label
                style={{
                    display: "flex",
                    alignItems: "flex-start",
                    gap: space[2],
                    border: `1px solid ${palette.border}`,
                    borderRadius: radius.md,
                    padding: space[3],
                    cursor: "pointer",
                }}
            >
                <input
                    type="checkbox"
                    checked={autoApprove}
                    onChange={(e) => onAutoApproveChange(e.target.checked)}
                    style={{ marginTop: 4 }}
                />
                <span style={{ fontSize: 12, lineHeight: 1.5 }}>
                    <span style={{ fontWeight: 500 }}>
                        Skip admin approval (automation)
                    </span>
                    <br />
                    <span style={{ color: palette.textMuted }}>
                        When off (default), the new host lands in <Mono>pending</Mono>{" "}
                        and an admin has to click Approve before the agent can open a
                        link. Turn on for unattended flows (Ansible, CI, cloud-init).
                    </span>
                </span>
            </label>

            <Field
                label="Description (optional)"
                hint="Free-form note shown in Audit → Enrollment."
            >
                <Input
                    placeholder="Deploy for web-01"
                    value={description}
                    onChange={(e) => onDescriptionChange(e.target.value)}
                />
            </Field>
        </div>
    );
}

function Field({
    label,
    hint,
    children,
}: {
    label: string;
    hint: React.ReactNode;
    children: React.ReactNode;
}) {
    return (
        <div>
            <label
                style={{
                    display: "block",
                    fontSize: 12,
                    fontWeight: 500,
                    marginBottom: 4,
                }}
            >
                {label}
            </label>
            {children}
            <div style={{ fontSize: 11, color: palette.textMuted, marginTop: 4 }}>
                {hint}
            </div>
        </div>
    );
}
