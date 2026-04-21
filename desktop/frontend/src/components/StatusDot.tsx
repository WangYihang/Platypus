import { palette } from "../layout/theme";

type Status = "online" | "offline" | "warning" | "error";

interface Props {
    status: Status;
    size?: number;
    title?: string;
}

const colors: Record<Status, string> = {
    online: palette.successDot,
    offline: palette.textMuted,
    warning: palette.warning,
    error: palette.danger,
};

// StatusDot is a tiny round indicator for online/offline-style states.
// Online gets a subtle outer halo so it visually pops vs. neutral
// offline/error variants.
export default function StatusDot({ status, size = 8, title }: Props) {
    const color = colors[status];
    const isOnline = status === "online";
    return (
        <span
            title={title}
            aria-label={title || status}
            style={{
                display: "inline-block",
                width: size,
                height: size,
                borderRadius: "50%",
                background: color,
                flexShrink: 0,
                boxShadow: isOnline ? `0 0 0 3px rgba(62,207,142,0.15)` : undefined,
                opacity: status === "offline" ? 0.6 : 1,
            }}
        />
    );
}
