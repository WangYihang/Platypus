import { Outlet, useLocation, useNavigate } from "react-router-dom";

import PageShell from "../components/PageShell";
import { useCurrentProject } from "../layout/ProjectShell";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";

// AuditPage is the shared shell for read-only history surfaces.
// Activities, Recordings, and Transfers were three sibling sidebar
// entries before the 2026-04 IA pass — each rendered its own
// PageHeader on a 1–2-card page that conceptually answered the same
// question ("what happened in this project?"). Consolidating them
// behind a single Audit entry with internal tabs:
//
//   1. Drops the project sidebar from 7 to 5 nav items so all four
//      groups (Work / Admin / Audit / Project) fit above the fold
//      without scrolling.
//   2. Reserves URL space for future audit kinds (login attempts,
//      key rotations, …) without sprawling the top-level routes.
//   3. Lets the sub-pages share a project-scoped header — the child
//      panels skip rendering their own PageHeader.
//
// Active sub-tab is derived from the last path segment so deep links
// (/projects/:slug/audit/recordings) continue to land directly.
const TABS = ["activities", "recordings", "transfers"] as const;
type AuditTab = (typeof TABS)[number];

const TAB_LABELS: Record<AuditTab, string> = {
    activities: "Activities",
    recordings: "Recordings",
    transfers: "Transfers",
};

export default function AuditPage() {
    const project = useCurrentProject();
    const navigate = useNavigate();
    const { pathname } = useLocation();
    const segments = pathname.split("/").filter(Boolean);
    // Path layout: /projects/:slug/audit/:tab — the last segment is
    // the tab name, with `audit` as a fallback for the index redirect
    // moment before <Navigate to="activities" replace /> fires.
    const last = segments[segments.length - 1] ?? "activities";
    const activeTab: AuditTab = (TABS as readonly string[]).includes(last)
        ? (last as AuditTab)
        : "activities";

    return (
        <PageShell
            title="Audit"
            subtitle={`History across ${project.name}`}
            tabs={
                <Tabs
                    value={activeTab}
                    onValueChange={(v) =>
                        navigate(`/projects/${project.slug}/audit/${v}`)
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
