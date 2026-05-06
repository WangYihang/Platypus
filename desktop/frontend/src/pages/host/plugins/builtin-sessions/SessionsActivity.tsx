// SessionsActivity wraps SessionsTab with the registry's
// PluginUIProps shape, pulling sessions[] from HostContext. The
// sys-process plugin gate (used to open a shell from a row's
// context menu) flows through <RequiresPlugins activity="sessions">
// matching the legacy hardcoded behaviour.

import RequiresPlugins from "../../RequiresPlugins";
import SessionsTab from "../../SessionsTab";
import type { PluginUIProps } from "../registry";
import { useHostContext } from "../../HostContext";

export default function SessionsActivity({ projectID, agentID }: PluginUIProps) {
    const { sessions } = useHostContext();
    return (
        <RequiresPlugins
            projectID={projectID}
            agentID={agentID}
            activity="sessions"
        >
            <SessionsTab sessions={[...sessions]} />
        </RequiresPlugins>
    );
}
