import PageShell from "../../components/PageShell";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

import PermissionsTab from "./access-control/PermissionsTab";
import RolesTab from "./access-control/RolesTab";

// /admin/access-control — admin-only RBAC management.
//
//   • Roles       — list / create / edit / delete. Builtins (viewer /
//                   operator / admin) cannot be deleted; their slot
//                   affinity (is_global / is_project) is locked, but
//                   the permission set is editable.
//   • Permissions — read-only catalogue, sorted by resource.
export default function AdminAccessControl() {
    return (
        <PageShell
            title="Access control"
            subtitle="Roles and permissions · admin only"
            bodyPadding={8}
        >
            <div style={{ maxWidth: 960 }}>
                <Tabs defaultValue="roles">
                    <TabsList data-testid="access-control-tabs" className="mb-4">
                        <TabsTrigger value="roles">Roles</TabsTrigger>
                        <TabsTrigger value="permissions">Permissions</TabsTrigger>
                    </TabsList>
                    <TabsContent value="roles" className="space-y-4">
                        <RolesTab />
                    </TabsContent>
                    <TabsContent value="permissions" className="space-y-4">
                        <PermissionsTab />
                    </TabsContent>
                </Tabs>
            </div>
        </PageShell>
    );
}
