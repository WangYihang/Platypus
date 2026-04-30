import { Outlet, useLocation, useNavigate } from "react-router-dom";

import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { palette, space } from "../../layout/theme";

// AdminLayout is the parent route for /admin/* — Users, Access Control,
// and Settings share the sub-tab strip the Admin top-tab opens. Body
// stays an Outlet so each child page keeps owning its own PageShell
// (StatusPills, RefreshButton, primary actions). The strip lives in a
// thin band above the outlet rather than inside PageHeader so the
// individual pages don't have to plumb identical Tabs JSX through.
const TABS = ["users", "access-control", "settings"] as const;
type AdminTab = (typeof TABS)[number];

const TAB_LABELS: Record<AdminTab, string> = {
    users: "Users",
    "access-control": "Access Control",
    settings: "Settings",
};

export default function AdminLayout() {
    const navigate = useNavigate();
    const { pathname } = useLocation();
    const segments = pathname.split("/").filter(Boolean);
    const last = segments[segments.length - 1] ?? "users";
    const activeTab: AdminTab = (TABS as readonly string[]).includes(last)
        ? (last as AdminTab)
        : "users";

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            <div
                style={{
                    flexShrink: 0,
                    padding: `${space[2]}px ${space[4]}px`,
                    background: palette.rail,
                    borderBottom: `1px solid ${palette.border}`,
                }}
            >
                <Tabs
                    value={activeTab}
                    onValueChange={(v) => navigate(`/admin/${v}`)}
                >
                    <TabsList className="h-7">
                        {TABS.map((t) => (
                            <TabsTrigger key={t} value={t}>
                                {TAB_LABELS[t]}
                            </TabsTrigger>
                        ))}
                    </TabsList>
                </Tabs>
            </div>
            <div style={{ flex: 1, minHeight: 0, overflow: "auto" }}>
                <Outlet />
            </div>
        </div>
    );
}
