import { Outlet, useLocation, useNavigate } from "react-router-dom";

import PageShell from "../components/PageShell";
import { useCurrentProject } from "../layout/ProjectShell";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";

// OperationsPage is the parent route for write-capable runtime
// surfaces — Transfers (in-flight file moves) and Enrollment (token /
// install-artifact management). Both used to live under the Audit hub,
// where the "Audit" label mis-signalled "read-only history". Splitting
// them out so the IA labels match what the page lets you actually do.
//
// Sister surface HistoryPage owns the truly read-only audit views
// (Activities, Recordings).
const TABS = ["transfers", "enrollment"] as const;
type OperationsTab = (typeof TABS)[number];

const TAB_LABELS: Record<OperationsTab, string> = {
    transfers: "Transfers",
    enrollment: "Enrollment",
};

export default function OperationsPage() {
    const project = useCurrentProject();
    const navigate = useNavigate();
    const { pathname } = useLocation();
    const segments = pathname.split("/").filter(Boolean);
    const last = segments[segments.length - 1] ?? "transfers";
    const activeTab: OperationsTab = (TABS as readonly string[]).includes(last)
        ? (last as OperationsTab)
        : "transfers";

    return (
        <PageShell
            title="Operations"
            subtitle={`Live runtime state across ${project.name}`}
            tabs={
                <Tabs
                    value={activeTab}
                    onValueChange={(v) =>
                        navigate(`/projects/${project.slug}/operations/${v}`)
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
