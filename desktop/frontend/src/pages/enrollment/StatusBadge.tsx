import StatusPill from "../../components/StatusPill";
import { EnrollmentStatus, STATUS_LABEL, STATUS_TONE } from "./status";

// StatusBadge maps the four-state EnrollmentStatus enum onto a colored
// StatusPill. The label / tone tables live in ./status so they can be
// unit-tested independently of any rendering surface.
export default function StatusBadge({ status }: { status: EnrollmentStatus }) {
    return <StatusPill tone={STATUS_TONE[status]}>{STATUS_LABEL[status]}</StatusPill>;
}
