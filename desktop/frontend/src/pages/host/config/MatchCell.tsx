import Mono from "../../../components/Mono";
import { palette } from "../../../layout/theme";

// MatchCell is the **last line of defence** against accidentally
// rendering a plaintext credential. The agent applies RedactSecret
// before transmission (first 4 + "****" + last 4) and the server
// stores only the redacted form, so by the time we get here the
// value should already contain a "*" mask. If it does not, we
// suspect a server-side bug or a future code path that forgot to
// redact, and we render "<redacted>" rather than passing through
// what could be a real secret.
//
// The check is intentionally conservative — anything containing at
// least one "*" passes; behavioural rules use literal markers like
// "AKIA****WXYZ", or strings such as "mode=0644" that are not
// secrets but contain no asterisk at all. We allow the latter
// through by trusting the marker prefix `mode=` and a few other
// known shapes; everything else without a `*` is treated as
// suspect.
const SAFE_PREFIXES = [
    "mode=",
    "no requirepass",
    "protected-mode no",
    "missing security.authorization",
    "PEM PRIVATE KEY",
    "ssh-rsa",
    "ssh-ed25519",
    "ssh-dss",
    "ecdsa-",
    "sk-",
];

function looksRedacted(value: string): boolean {
    if (value.includes("*")) return true;
    return SAFE_PREFIXES.some((p) => value.startsWith(p));
}

interface Props {
    value: string;
}

export function MatchCell({ value }: Props) {
    const safe = looksRedacted(value);
    return (
        <span
            title={
                safe
                    ? value
                    : "value did not arrive redacted; rendering as <redacted>"
            }
            style={{ display: "inline-block" }}
        >
            <Mono
                style={{
                    fontSize: 12,
                    color: safe ? palette.textSecondary : palette.danger,
                    background: palette.surfaceHover,
                    padding: "2px 6px",
                    borderRadius: 4,
                    whiteSpace: "nowrap",
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                    maxWidth: 320,
                    display: "inline-block",
                    verticalAlign: "middle",
                }}
            >
                {safe ? value : "<redacted>"}
            </Mono>
        </span>
    );
}

export default MatchCell;
