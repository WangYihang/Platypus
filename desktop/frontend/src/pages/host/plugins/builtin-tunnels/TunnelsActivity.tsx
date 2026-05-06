import RequiresPlugins from "../../RequiresPlugins";
import TunnelsTab from "../../TunnelsTab";
import type { PluginUIProps } from "../registry";
import { useHostContext } from "../../HostContext";

export default function TunnelsActivity({ projectID, agentID }: PluginUIProps) {
    const { hostID } = useHostContext();
    return (
        <RequiresPlugins
            projectID={projectID}
            agentID={agentID}
            activity="tunnels"
        >
            <TunnelsTab projectID={projectID} hostID={hostID} />
        </RequiresPlugins>
    );
}
