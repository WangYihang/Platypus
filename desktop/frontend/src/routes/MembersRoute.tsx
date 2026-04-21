import ProjectMembers from "../pages/ProjectMembers";
import { useCurrentProject } from "../layout/ProjectShell";

export default function MembersRoute() {
    const project = useCurrentProject();
    return <ProjectMembers project={project} />;
}
