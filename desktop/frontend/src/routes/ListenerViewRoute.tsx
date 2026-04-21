import { useNavigate, useParams } from "react-router-dom";

import ListenerView from "../pages/ListenerView";
import { useCurrentProject } from "../layout/ProjectShell";

// ListenerViewRoute serves both /listeners (list) and /listeners/:listenerId
// (detail). The existing ListenerView component handles both modes —
// step 7 will split them into two route components.
export default function ListenerViewRoute() {
    const project = useCurrentProject();
    const { listenerId } = useParams<{ listenerId?: string }>();
    const navigate = useNavigate();
    return (
        <ListenerView
            projectID={project.id}
            listenerID={listenerId}
            onSelectListener={(lid) =>
                navigate(`/projects/${project.slug}/listeners/${lid}`)
            }
        />
    );
}
