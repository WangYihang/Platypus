import { ApiOutlined } from "@ant-design/icons";

import { Listener } from "../lib/api";
import { palette } from "./theme";

interface Props {
    listener: Listener;
    selected: boolean;
    onSelect: () => void;
}

// ListenerRow renders one listener in the sidebar. Endpoint style:
// [icon] host:port   — no per-row count yet (sessions-under-listener
// requires another API call; deferred to P10 when the listener detail
// view actually shows that data).
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
                gap: 8,
                padding: "6px 12px 6px 28px",
                cursor: "pointer",
                color: selected ? palette.textPrimary : palette.textSecondary,
                background: selected ? palette.main : "transparent",
                borderLeft: selected ? `2px solid ${palette.accent}` : "2px solid transparent",
                fontSize: 13,
                userSelect: "none",
            }}
        >
            <ApiOutlined />
            <span>
                {listener.host}:{listener.port}
            </span>
        </div>
    );
}
