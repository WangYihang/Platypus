import { ApiOutlined } from "@ant-design/icons";

import Mono from "../components/Mono";
import { Listener } from "../lib/api";
import { palette, space } from "./theme";

interface Props {
    listener: Listener;
    selected: boolean;
    onSelect: () => void;
}

// ListenerRow renders one listener in the sidebar.
// Endpoint shown in Geist Mono so host:port lines align across rows.
export default function ListenerRow({ listener, selected, onSelect }: Props) {
    return (
        <div
            role="button"
            tabIndex={0}
            onClick={onSelect}
            onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") onSelect();
            }}
            style={{
                display: "flex",
                alignItems: "center",
                gap: space[2],
                padding: `${space[2]}px ${space[3]}px ${space[2]}px 28px`,
                cursor: "pointer",
                color: selected ? palette.textPrimary : palette.textSecondary,
                background: selected ? palette.surfaceHover : "transparent",
                borderLeft: selected
                    ? `2px solid ${palette.textPrimary}`
                    : "2px solid transparent",
                fontSize: 13,
                userSelect: "none",
            }}
        >
            <ApiOutlined style={{ color: palette.textMuted, fontSize: 12 }} />
            <Mono size={12} color={selected ? palette.textPrimary : palette.textSecondary}>
                {listener.host}:{listener.port}
            </Mono>
        </div>
    );
}
