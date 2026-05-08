import ConfigTab from "../../ConfigTab";
import RequiresPlugins from "../../RequiresPlugins";
import type { PluginUIProps } from "../registry";
import { useHostContext } from "../../HostContext";

export default function ConfigActivity({
    projectID,
    agentID,
    active,
}: PluginUIProps) {
    const { hostID } = useHostContext();
    return (
        <RequiresPlugins
            projectID={projectID}
            agentID={agentID}
            activity="config"
        >
            <ConfigTab
                projectID={projectID}
                hostID={hostID}
                agentID={agentID}
                active={active}
            />
        </RequiresPlugins>
    );
}
