import PageHeader from "../components/PageHeader";
import TransferTaskList from "../components/TransferTaskList";
import { useCurrentProject } from "../layout/ProjectShell";
import { space } from "../layout/theme";

// TransfersPage is the project-scoped global view of every file
// transfer (downloads + uploads) across all hosts in the project.
// The per-host transfers tab in HostView reuses TransferTaskList
// with a hostId filter — this page omits that filter so callers see
// the project-wide timeline.
export default function TransfersPage() {
    const project = useCurrentProject();
    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            <PageHeader
                title="Transfers"
                subtitle={`File downloads and uploads in ${project.name}`}
            />
            <div style={{ flex: 1, padding: space[6], overflow: "auto" }}>
                <TransferTaskList projectId={project.id} />
            </div>
        </div>
    );
}
