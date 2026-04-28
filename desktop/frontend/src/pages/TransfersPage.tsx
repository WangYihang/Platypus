import TransferTaskList from "../components/TransferTaskList";
import { useCurrentProject } from "../layout/ProjectShell";
import { space } from "../layout/theme";

// TransfersPage is the project-scoped global view of every file
// transfer (downloads + uploads) across all hosts in the project.
// Lives under <AuditPage> in the routes tree, which owns the page
// header + tab strip — this component renders only the body.
export default function TransfersPage() {
    const project = useCurrentProject();
    return (
        <div style={{ flex: 1, padding: space[4], overflow: "auto" }}>
            <TransferTaskList projectId={project.id} />
        </div>
    );
}
