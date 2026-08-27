import {
    Link,
    Outlet,
    useLocation,
    useNavigate,
    useParams,
} from "react-router-dom";
import { ShieldAlert } from "lucide-react";
import { useCallback, useMemo, useRef } from "react";
import { useQuery } from "@tanstack/react-query";

import EnrollmentWaitBanner from "../components/EnrollmentWaitBanner";
import PageShell from "../components/PageShell";
import StatusPills from "../components/StatusPills";
import { useCurrentProject } from "../layout/ProjectShell";
import { cn } from "@/lib/cn";
import { icons } from "../lib/icons";
import { listHosts, pendingApprovalCount } from "../lib/api";
import { qk } from "../lib/queryKeys";
import { isOnline } from "../lib/time";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";

import EnrollAgentWizard from "./fleet/enroll/EnrollAgentWizard";

// HostsShell is the parent route for /projects/:slug/hosts/*. Owns
// the page title (Hosts), online/offline pills, Enroll button, and
// the View toggle (List ↔ Topology). The host-detail master view
// lives under /hosts/:id/:tab and renders inside the same shell so
// the chrome stays in place when jumping between hosts.
//
// The bottom terminal drawer is NOT owned here. Shells are host-scoped
// and the drawer does hide itself off /hosts, but that is a visibility
// concern it handles internally; mounting it here also unmounted it on
// every navigation away, closing the WebSocket behind each open shell.
// It now sits in ShellChrome via terminal/TerminalDock.
const VIEWS = ["list", "topology"] as const;
type HostsView = (typeof VIEWS)[number];

const VIEW_LABELS: Record<HostsView, string> = {
    list: "List",
    topology: "Topology",
};

export default function HostsShell() {
    const project = useCurrentProject();
    const navigate = useNavigate();
    const { pathname } = useLocation();
    const { hostId } = useParams<{ hostId?: string }>();

    // Pick the active view from the URL. /hosts → list, /hosts/topology
    // → topology, /hosts/:hostId/* → list (so the rail stays grouped
    // with the list-mode chrome).
    const segments = pathname.split("/").filter(Boolean);
    const hostsIdx = segments.indexOf("hosts");
    const after = hostsIdx >= 0 ? segments[hostsIdx + 1] : undefined;
    const activeView: HostsView = after === "topology" ? "topology" : "list";

    const { data: hosts } = useQuery({
        queryKey: qk.hosts(project.id),
        queryFn: () => listHosts(project.id),
    });

    // Pending approvals badge polls every 10s. The badge surfaces in
    // the header so the operator sees pending approvals from anywhere
    // under /hosts; clicking jumps to /enrollment/approvals.
    const { data: pendingCount = 0 } = useQuery({
        queryKey: qk.pendingHostsCount(project.id),
        queryFn: () => pendingApprovalCount(project.id),
        refetchInterval: 10_000,
    });

    const counts = useMemo(() => {
        const list = hosts ?? [];
        let online = 0;
        for (const h of list) {
            if (isOnline(h.last_seen_at)) online++;
        }
        return { online, offline: list.length - online };
    }, [hosts]);

    const EnrollIcon = icons.enrollment;
    const actions = (
        <span style={{ display: "inline-flex", alignItems: "center", gap: 8 }}>
            {pendingCount > 0 && (
                <Button asChild variant="outline" size="sm" data-testid="hosts-pending-approvals">
                    <Link
                        to={`/projects/${project.slug}/enrollment/approvals`}
                        title="Hosts awaiting admin approval — agents can't open links until approved"
                    >
                        <ShieldAlert className="size-3.5" />
                        {pendingCount} pending
                    </Link>
                </Button>
            )}
            <Button asChild variant="outline" size="sm">
                <Link to="?enroll=1" data-testid="hosts-enroll-trigger">
                    <EnrollIcon className="size-3.5" />
                    Enroll agent
                </Link>
            </Button>
        </span>
    );

    // The List/Topology toggle hides itself when a specific host is
    // selected — the host detail view fills the surface and the
    // toggle would be visually unrelated to the master-detail rail.
    const tabs = !hostId ? (
        <Tabs
            value={activeView}
            onValueChange={(v) =>
                navigate(
                    v === "list"
                        ? `/projects/${project.slug}/hosts`
                        : `/projects/${project.slug}/hosts/${v}`,
                )
            }
        >
            <TabsList className="h-7" data-testid="hosts-subtabs">
                {VIEWS.map((v) => (
                    <TabsTrigger key={v} value={v}>
                        {VIEW_LABELS[v]}
                    </TabsTrigger>
                ))}
            </TabsList>
        </Tabs>
    ) : null;

    return (
        <PageShell
            title="Hosts"
            actions={actions}
            tabs={tabs}
            pills={
                <StatusPills
                    pills={[
                        { tone: "success", count: counts.online, label: "online" },
                        { tone: "muted", count: counts.offline, label: "offline" },
                    ]}
                />
            }
            bodyPadding={0}
            bodyStyle={{ display: "flex", flexDirection: "column", padding: 0 }}
        >
            <EnrollmentWaitBanner projectID={project.id} projectSlug={project.slug} />
            {/* Mounted at the parent so the wizard floats over any
                sub-view. Open / closed state is driven by the
                `?enroll=1` URL param via useEnrollWizardOpen. */}
            <EnrollAgentWizard />
            <HostsBody />
        </PageShell>
    );
}

// HostsBody is just the routed view now. The terminal drawer used to be
// stacked here, which meant clicking Overview unmounted it and closed
// every open shell's WebSocket. It lives in ShellChrome instead — see
// terminal/TerminalDock.
function HostsBody() {
    return (
        <div style={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "column" }}>
            <Outlet />
        </div>
    );
}
