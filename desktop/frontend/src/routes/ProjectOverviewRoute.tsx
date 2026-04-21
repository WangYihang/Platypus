import { useNavigate } from "react-router-dom";

import ProjectOverview from "../pages/ProjectOverview";
import { useCurrentProject } from "../layout/ProjectShell";

export default function ProjectOverviewRoute() {
    const project = useCurrentProject();
    const navigate = useNavigate();
    return (
        <ProjectOverview
            project={project}
            onOpenMembers={() => navigate(`/projects/${project.slug}/members`)}
        />
    );
}
