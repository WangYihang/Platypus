import RequiresPlugins from "../../RequiresPlugins";
import SecurityTab from "../../SecurityTab";
import type { PluginUIProps } from "../registry";
import { useHostContext } from "../../HostContext";

export default function SecurityActivity({
    projectID,
    agentID,
    active,
}: PluginUIProps) {
    const { hostID } = useHostContext();
    return (
        <RequiresPlugins
            projectID={projectID}
            agentID={agentID}
            activity="security"
        >
            <SecurityTab
                projectID={projectID}
                hostID={hostID}
                agentID={agentID}
                active={active}
            />
        </RequiresPlugins>
    );
}
