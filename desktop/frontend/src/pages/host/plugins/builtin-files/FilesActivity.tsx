// FilesActivity is the registry-side wrapper around the Files tab.
// It conforms to PluginUIProps (the registry's lingua franca) and
// pulls the extra context (host, pickedSessionID) from HostContext
// so HostView doesn't have to thread those through the generic
// activity-router. The wrapper still leans on <RequiresPlugins> for
// the install-guide behaviour when sys-files-read / -write aren't
// installed (matching the legacy hardcoded-tab UX).

import EmptyState from "../../../../components/EmptyState";
import FilesTab from "../../FilesTab";
import RequiresPlugins from "../../RequiresPlugins";
import type { PluginUIProps } from "../registry";
import { useHostContext } from "../../HostContext";

export default function FilesActivity({ projectID, agentID }: PluginUIProps) {
    const { host, pickedSessionID } = useHostContext();
    return (
        <RequiresPlugins
            projectID={projectID}
            agentID={agentID}
            activity="files"
        >
            {pickedSessionID ? (
                <FilesTab
                    projectID={projectID}
                    sessionHash={pickedSessionID}
                    host={host}
                />
            ) : (
                <EmptyState
                    title="No live session"
                    description="Start or reconnect an agent to use this tab."
                />
            )}
        </RequiresPlugins>
    );
}
