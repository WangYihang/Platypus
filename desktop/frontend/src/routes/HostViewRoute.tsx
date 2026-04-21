import { useParams } from "react-router-dom";

import HostView from "../pages/HostView";
import { useCurrentProject } from "../layout/ProjectShell";
import EmptyState from "../components/EmptyState";

export default function HostViewRoute() {
    const project = useCurrentProject();
    const { hostId } = useParams<{ hostId: string }>();
    if (!hostId) {
        return <EmptyState title="Missing host id" fill />;
    }
    return <HostView projectID={project.id} hostID={hostId} />;
}
