import DispatchPanel from "../pages/DispatchPanel";
import { useCurrentProject } from "../layout/ProjectShell";

export default function DispatchRoute() {
    const project = useCurrentProject();
    return <DispatchPanel projectID={project.id} projectName={project.name} />;
}
