import { useEffect, useRef, useState } from "react";
import { CheckCircle2, Clock, Loader2, X } from "lucide-react";
import { Link, useSearchParams } from "react-router-dom";

import { palette, radius, space } from "../layout/theme";
import { Host, listHosts } from "../lib/api";
import { Button } from "@/components/ui/button";

// EnrollmentWaitBanner is the "did my agent show up yet?" affordance
// shown above Fleet after a user has just generated an install
// command. Without it, the path "Generate command → run on box → ???
// → check Fleet" left the user wondering whether anything happened —
// no signal that we're actively watching, no confirmation when the
// agent dialled back.
//
// Activates only when the URL carries `?await=enroll`. On mount it
// snapshots the current host count; thereafter it polls listHosts
// every 3s and switches into a green "X new agent(s) enrolled" state
// once the count grows. After 90s without any new host the wording
// gracefully degrades to a "still waiting" message — so a user who
// walks away comes back to honest copy rather than a cheery green
// banner that's been lying for an hour.
//
// The component intentionally has no fetch error handling beyond
// silently resetting the spinner — if the API is failing, the rest
// of Fleet will surface the same error in its own toast.

const POLL_MS = 3000;
const STALE_AFTER_MS = 90_000;

interface Props {
    projectID: string;
    projectSlug: string;
}

type State =
    | { kind: "waiting"; baselineCount: number; startedAt: number }
    | { kind: "arrived"; baselineCount: number; newHosts: Host[] }
    | { kind: "stale"; baselineCount: number };

export default function EnrollmentWaitBanner({ projectID, projectSlug }: Props) {
    const [params, setParams] = useSearchParams();
    const active = params.get("await") === "enroll";

    const [state, setState] = useState<State | null>(null);
    const initial = useRef(true);

    // Reset when the active flag flips. We keep the ref so multiple
    // poll cycles can compare against the same baseline.
    useEffect(() => {
        if (!active) {
            setState(null);
            initial.current = true;
            return;
        }
        let cancelled = false;
        const tick = async () => {
            try {
                const hosts = await listHosts(projectID);
                if (cancelled) return;
                if (initial.current) {
                    initial.current = false;
                    setState({
                        kind: "waiting",
                        baselineCount: hosts.length,
                        startedAt: Date.now(),
                    });
                    return;
                }
                setState((prev) => {
                    if (!prev) return prev;
                    if (prev.kind === "arrived") return prev;
                    if (hosts.length > prev.baselineCount) {
                        const newHosts = hosts.slice(prev.baselineCount);
                        return {
                            kind: "arrived",
                            baselineCount: prev.baselineCount,
                            newHosts,
                        };
                    }
                    if (
                        prev.kind === "waiting" &&
                        Date.now() - prev.startedAt > STALE_AFTER_MS
                    ) {
                        return { kind: "stale", baselineCount: prev.baselineCount };
                    }
                    return prev;
                });
            } catch {
                /* network blip — next tick will retry */
            }
        };
        void tick();
        const id = window.setInterval(tick, POLL_MS);
        return () => {
            cancelled = true;
            window.clearInterval(id);
        };
    }, [active, projectID]);

    if (!active || !state) return null;

    function dismiss() {
        const next = new URLSearchParams(params);
        next.delete("await");
        setParams(next, { replace: true });
    }

    if (state.kind === "arrived") {
        const first = state.newHosts[0];
        const label =
            first.primary_alias || first.hostname || first.machine_id?.slice(0, 12) || "new host";
        const hostHref = `/projects/${projectSlug}/hosts/${first.id}/info`;
        return (
            <Frame
                tone="success"
                icon={<CheckCircle2 className="size-4" />}
                title={
                    state.newHosts.length === 1
                        ? `Agent ${label} enrolled`
                        : `${state.newHosts.length} agents enrolled`
                }
                action={
                    <Link to={hostHref}>
                        <Button size="sm">Open host</Button>
                    </Link>
                }
                onDismiss={dismiss}
            />
        );
    }

    if (state.kind === "stale") {
        return (
            <Frame
                tone="warning"
                icon={<Clock className="size-4" />}
                title="Still waiting for an agent"
                description="No new host has dialled back yet. Re-run the install command on the target machine, or check that the network can reach this server."
                onDismiss={dismiss}
            />
        );
    }

    return (
        <Frame
            tone="info"
            icon={<Loader2 className="size-4 animate-spin" />}
            title="Waiting for an agent to enroll"
            description="Run the install command on the target machine. New hosts appear here automatically, usually within 10 seconds."
            onDismiss={dismiss}
        />
    );
}

interface FrameProps {
    tone: "info" | "success" | "warning";
    icon: React.ReactNode;
    title: string;
    description?: string;
    action?: React.ReactNode;
    onDismiss: () => void;
}

function Frame({ tone, icon, title, description, action, onDismiss }: FrameProps) {
    const colour =
        tone === "success" ? palette.success : tone === "warning" ? palette.warning : palette.info;
    return (
        <div
            data-testid="enrollment-wait-banner"
            data-tone={tone}
            style={{
                margin: `${space[3]}px ${space[8]}px 0`,
                padding: `${space[3]}px ${space[4]}px`,
                border: `1px solid ${colour}`,
                borderRadius: radius.md,
                background: "transparent",
                display: "flex",
                alignItems: "center",
                gap: space[3],
                color: palette.textPrimary,
                fontSize: 13,
            }}
        >
            <span style={{ color: colour, display: "inline-flex" }}>{icon}</span>
            <div style={{ flex: 1, lineHeight: 1.4 }}>
                <div style={{ fontWeight: 500 }}>{title}</div>
                {description && (
                    <div style={{ color: palette.textSecondary, marginTop: 2, fontSize: 12 }}>
                        {description}
                    </div>
                )}
            </div>
            {action}
            <button
                aria-label="Dismiss"
                onClick={onDismiss}
                style={{
                    background: "none",
                    border: "none",
                    cursor: "pointer",
                    color: palette.textMuted,
                    display: "inline-flex",
                    padding: 4,
                }}
            >
                <X className="size-4" />
            </button>
        </div>
    );
}
