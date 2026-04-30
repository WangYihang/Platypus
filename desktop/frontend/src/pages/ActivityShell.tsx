import { Outlet, useLocation, useNavigate } from "react-router-dom";

import PageShell from "../components/PageShell";
import { useCurrentProject } from "../layout/ProjectShell";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";

// ActivityShell is the project-wide rollup of every per-host time-
// series resource: live sessions, recorded shells, audit events, and
// in-flight file transfers. Each sub-tab is the same data the per-host
// tabs surface, unioned across the fleet.
const TABS = ["sessions", "events", "recordings", "transfers"] as const;
type ActivityTab = (typeof TABS)[number];

const TAB_LABELS: Record<ActivityTab, string> = {
    sessions: "Sessions",
    events: "Events",
    recordings: "Recordings",
    transfers: "Transfers",
};

export default function ActivityShell() {
    const project = useCurrentProject();
    const navigate = useNavigate();
    const { pathname } = useLocation();
    const segments = pathname.split("/").filter(Boolean);
    const last = segments[segments.length - 1] ?? "sessions";
    const activeTab: ActivityTab = (TABS as readonly string[]).includes(last)
        ? (last as ActivityTab)
        : "sessions";

    return (
        <PageShell
            title="Activity"
            subtitle={`What's happening across ${project.name}`}
            tabs={
                <Tabs
                    value={activeTab}
                    onValueChange={(v) =>
                        navigate(`/projects/${project.slug}/activity/${v}`)
                    }
                >
                    <TabsList className="h-7" data-testid="activity-subtabs">
                        {TABS.map((t) => (
                            <TabsTrigger key={t} value={t}>
                                {TAB_LABELS[t]}
                            </TabsTrigger>
                        ))}
                    </TabsList>
                </Tabs>
            }
            bodyPadding={0}
            bodyStyle={{ display: "flex", flexDirection: "column", padding: 0 }}
        >
            <Outlet />
        </PageShell>
    );
}
