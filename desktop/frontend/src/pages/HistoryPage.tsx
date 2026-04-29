import { Outlet, useLocation, useNavigate } from "react-router-dom";

import PageShell from "../components/PageShell";
import { useCurrentProject } from "../layout/ProjectShell";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";

// HistoryPage is the parent route for read-only audit surfaces:
// Activities (event log) and Recordings (asciinema-style session
// playback). Both answer the same question — "what happened in this
// project?" — and benefit from a shared header + tab strip.
//
// Sister surface OperationsPage owns the write-capable runtime
// surfaces (Transfers, Enrollment) that used to share the now-retired
// Audit hub. Keeping read-only and write-capable separate so the
// "Audit" label stops over-promising.
const TABS = ["activities", "recordings"] as const;
type HistoryTab = (typeof TABS)[number];

const TAB_LABELS: Record<HistoryTab, string> = {
    activities: "Activities",
    recordings: "Recordings",
};

export default function HistoryPage() {
    const project = useCurrentProject();
    const navigate = useNavigate();
    const { pathname } = useLocation();
    const segments = pathname.split("/").filter(Boolean);
    const last = segments[segments.length - 1] ?? "activities";
    const activeTab: HistoryTab = (TABS as readonly string[]).includes(last)
        ? (last as HistoryTab)
        : "activities";

    return (
        <PageShell
            title="History"
            subtitle={`Read-only audit across ${project.name}`}
            tabs={
                <Tabs
                    value={activeTab}
                    onValueChange={(v) =>
                        navigate(`/projects/${project.slug}/history/${v}`)
                    }
                >
                    <TabsList className="h-7">
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
