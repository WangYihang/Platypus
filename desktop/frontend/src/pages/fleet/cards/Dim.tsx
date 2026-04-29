import { palette } from "../../../layout/theme";

// Dim wraps "—" / placeholder spans inside cards so the muted color
// doesn't have to be repeated inline at every callsite. Local to
// cards/ to keep the shared component surface small; promote later
// if other surfaces want it.
export default function Dim({ children }: { children: React.ReactNode }) {
    return <span style={{ color: palette.textMuted }}>{children}</span>;
}
