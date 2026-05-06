// ProcessesActivity wraps ProcessesTab. Pulls hostID from
// HostContext (server routes key off it) and forwards `active` from
// PluginUIProps so the underlying tab can pause its 5s polling
// while offscreen.
//
// The registry registers one entry per host OS so install gating
// uses the OS-specific plugin id (sys-procs-linux on Linux,
// sys-procs-darwin on macOS, sys-procs-windows on Windows). All
// three share this single React component because the wire shape
// is identical across OSes.

import ProcessesTab from "../../ProcessesTab";
import RequiresPlugins from "../../RequiresPlugins";
import type { PluginUIProps } from "../registry";
import { useHostContext } from "../../HostContext";

export default function ProcessesActivity({
    projectID,
    agentID,
    active,
}: PluginUIProps) {
    const { hostID } = useHostContext();
    return (
        <RequiresPlugins
            projectID={projectID}
            agentID={agentID}
            activity="processes"
        >
            <ProcessesTab projectID={projectID} hostID={hostID} active={active} />
        </RequiresPlugins>
    );
}
