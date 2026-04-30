import {
    Link,
    Outlet,
    useLocation,
    useNavigate,
} from "react-router-dom";
import { ShieldAlert } from "lucide-react";
import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";

import EnrollmentWaitBanner from "../components/EnrollmentWaitBanner";
import PageShell from "../components/PageShell";
import StatusPills from "../components/StatusPills";
import { useCurrentProject } from "../layout/ProjectShell";
import { icons } from "../lib/icons";
import { listHosts, pendingApprovalCount } from "../lib/api";
import { qk } from "../lib/queryKeys";
import { isOnline } from "../lib/time";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";

import EnrollAgentWizard from "./fleet/enroll/EnrollAgentWizard";

const TABS = ["hosts", "sessions", "topology", "approvals"] as const;
type FleetTab = (typeof TABS)[number];

const TAB_LABELS: Record<FleetTab, string> = {
    hosts: "Hosts",
    sessions: "Sessions",
    topology: "Topology",
    approvals: "Approvals",
};

// FleetPage is the parent route for /projects/:slug/fleet. It owns
// the page identity (title, online/offline pills, Enroll button) and
// the sub-tab strip (Hosts · Sessions · Topology · Approvals);
// individual panels render through the <Outlet />.
//
// The historical four-way Cards/Table/Timeline/Graph toggle was split:
// Sessions and Topology became their own tabs (each with their own
// canonical URL), and Cards/Table became a toggle local to the Hosts
// tab. Master-detail host inspection lives under
// `fleet/hosts/:hostId/:activity` — see pages/fleet/HostsView.tsx.
export default function FleetPage() {
    const project = useCurrentProject();
    const navigate = useNavigate();
    const { pathname } = useLocation();

    // Pick the active tab from the path. Anything we don't recognise
    // falls through to "hosts" — the route table redirects /fleet to
    // /fleet/hosts so this only matters during transient navigation.
    const segments = pathname.split("/").filter(Boolean);
    const fleetIdx = segments.indexOf("fleet");
    const candidate = fleetIdx >= 0 ? segments[fleetIdx + 1] : undefined;
    const activeTab: FleetTab = (TABS as readonly string[]).includes(
        candidate ?? "",
    )
        ? (candidate as FleetTab)
        : "hosts";

    const { data: hosts } = useQuery({
        queryKey: qk.hosts(project.id),
        queryFn: () => listHosts(project.id),
    });

    // Pending approvals badge: cheap COUNT(*) endpoint that polls
    // every 10s. Drives both the Approvals tab badge and the older
    // header pill so the steady-state UI stays in sync.
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
                <Button asChild variant="outline" size="sm" data-testid="fleet-pending-approvals">
                    <Link
                        to={`/projects/${project.slug}/fleet/approvals`}
                        title="Hosts awaiting admin approval — agents can't open links until approved"
                    >
                        <ShieldAlert className="size-3.5" />
                        {pendingCount} pending
                    </Link>
                </Button>
            )}
            <Button asChild variant="outline" size="sm">
                <Link to="?enroll=1" data-testid="fleet-enroll-trigger">
                    <EnrollIcon className="size-3.5" />
                    Enroll agent
                </Link>
            </Button>
        </span>
    );

    const tabs = (
        <Tabs
            value={activeTab}
            onValueChange={(v) => navigate(`/projects/${project.slug}/fleet/${v}`)}
        >
            <TabsList className="h-7" data-testid="fleet-subtabs">
                {TABS.map((t) => (
                    <TabsTrigger key={t} value={t}>
                        {TAB_LABELS[t]}
                        {t === "approvals" && pendingCount > 0 && (
                            <span
                                aria-label={`${pendingCount} pending`}
                                style={{
                                    marginLeft: 6,
                                    minWidth: 16,
                                    height: 16,
                                    padding: "0 4px",
                                    borderRadius: 999,
                                    background: "var(--color-warning, #b45309)",
                                    color: "#fff",
                                    fontSize: 10,
                                    fontWeight: 600,
                                    display: "inline-flex",
                                    alignItems: "center",
                                    justifyContent: "center",
                                }}
                            >
                                {pendingCount}
                            </span>
                        )}
                    </TabsTrigger>
                ))}
            </TabsList>
        </Tabs>
    );

    return (
        <PageShell
            title="Fleet"
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
            {/* Mounted at the parent so it floats over any sub-tab.
                Open / closed state is driven by the `?enroll=1` URL
                param (see useEnrollWizardOpen). */}
            <EnrollAgentWizard />
            <div style={{ flex: 1, minHeight: 0, position: "relative", display: "flex", flexDirection: "column" }}>
                <Outlet />
            </div>
        </PageShell>
    );
}
