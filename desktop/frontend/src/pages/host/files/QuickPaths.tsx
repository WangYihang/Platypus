import type { Host } from "../../../lib/api";
import { palette, radius, space } from "../../../layout/theme";
import { quickPathsForHost } from "./quickPaths";

interface Props {
    host: Host | null;
    onSelect: (path: string) => void;
}

// QuickPaths is the chip row above the FileBrowser breadcrumb. Each
// chip teleports the directory listing to a common root (/, ~,
// /etc, /var, /tmp on Unix; C:\, ~, C:\Windows on Windows). The path
// set comes from quickPathsForHost — pure data — and the component
// is a thin button list.
//
// Renders nothing while the host is still loading so the row
// doesn't pop in mid-mount. Chip styling matches the Activities
// page's quick-filter chips so the patterns read as the same idea
// across the app.
export default function QuickPaths({ host, onSelect }: Props) {
    const paths = quickPathsForHost(host);
    if (!paths || paths.length === 0) return null;

    return (
        <div
            data-testid="files-quick-paths"
            style={{
                display: "flex",
                flexWrap: "wrap",
                alignItems: "center",
                gap: space[1],
            }}
        >
            <span
                style={{
                    fontSize: 11,
                    color: palette.textMuted,
                    marginRight: space[1],
                }}
            >
                Jump to:
            </span>
            {paths.map((p) => (
                <button
                    key={p.path}
                    type="button"
                    onClick={() => onSelect(p.path)}
                    title={p.title}
                    style={{
                        display: "inline-flex",
                        alignItems: "center",
                        padding: `2px ${space[3]}px`,
                        fontSize: 12,
                        fontWeight: 500,
                        lineHeight: 1.6,
                        color: palette.textSecondary,
                        background: "transparent",
                        border: `1px solid ${palette.border}`,
                        borderRadius: radius.pill,
                        cursor: "pointer",
                        whiteSpace: "nowrap",
                        fontFamily: "var(--font-geist-mono)",
                    }}
                    onMouseEnter={(e) => {
                        e.currentTarget.style.background = palette.surfaceHover;
                        e.currentTarget.style.color = palette.textPrimary;
                    }}
                    onMouseLeave={(e) => {
                        e.currentTarget.style.background = "transparent";
                        e.currentTarget.style.color = palette.textSecondary;
                    }}
                >
                    {p.label}
                </button>
            ))}
        </div>
    );
}
