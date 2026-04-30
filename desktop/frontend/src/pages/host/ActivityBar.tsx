import { ReactNode } from "react";
import { File, Info, Plug, AppWindow, ShieldCheck, Cable } from "lucide-react";

import { palette } from "../../layout/theme";

export const ACTIVITIES = [
    "files",
    "info",
    "sessions",
    "processes",
    "security",
    "tunnels",
] as const;
export type Activity = (typeof ACTIVITIES)[number];

interface ActivitySpec {
    key: Activity;
    label: string;
    icon: ReactNode;
}

const ICON_SIZE = "size-4";

export const ACTIVITY_SPECS: ActivitySpec[] = [
    { key: "files", label: "Files", icon: <File className={ICON_SIZE} /> },
    { key: "info", label: "Info", icon: <Info className={ICON_SIZE} /> },
    { key: "sessions", label: "Sessions", icon: <Plug className={ICON_SIZE} /> },
    { key: "processes", label: "Processes", icon: <AppWindow className={ICON_SIZE} /> },
    { key: "security", label: "Security", icon: <ShieldCheck className={ICON_SIZE} /> },
    { key: "tunnels", label: "Tunnels", icon: <Cable className={ICON_SIZE} /> },
];

interface Props {
    active: Activity;
    onSelect: (activity: Activity) => void;
    badges?: Partial<Record<Activity, number>>;
}

// ActivityBar is the 44 px vertical strip on the left of HostView,
// VSCode style. Each icon switches the active activity (URL goes
// `/hosts/<id>/<activity>`); icons map to the same domain
// nouns as the rest of the app via lib/icons.ts.
//
// We render plain lucide icons inline rather than going through
// `lib/icons` because two of these (File, Info) clash with already-
// registered keys for different surfaces — a future cleanup can
// promote them once the registry stabilises.
const BAR_WIDTH = 44;

export default function ActivityBar({ active, onSelect, badges }: Props) {
    return (
        <nav
            data-testid="host-activity-bar"
            aria-label="Host activities"
            style={{
                flexShrink: 0,
                width: BAR_WIDTH,
                borderRight: `1px solid ${palette.border}`,
                background: palette.rail,
                display: "flex",
                flexDirection: "column",
                padding: "6px 0",
                gap: 2,
            }}
        >
            {ACTIVITY_SPECS.map((spec) => {
                const isActive = active === spec.key;
                const badge = badges?.[spec.key];
                return (
                    <button
                        key={spec.key}
                        type="button"
                        onClick={() => onSelect(spec.key)}
                        data-testid={`host-activity-${spec.key}`}
                        data-active={isActive || undefined}
                        aria-current={isActive ? "true" : undefined}
                        title={spec.label}
                        style={{
                            position: "relative",
                            width: BAR_WIDTH,
                            height: 40,
                            display: "inline-flex",
                            alignItems: "center",
                            justifyContent: "center",
                            background: "transparent",
                            border: "none",
                            cursor: "pointer",
                            color: isActive ? palette.textPrimary : palette.textMuted,
                            // Left bar marks the active activity in VSCode
                            // fashion — a 2px accent stripe flush against
                            // the inner edge of the bar.
                            boxShadow: isActive
                                ? `inset 2px 0 0 0 ${palette.accent}`
                                : undefined,
                        }}
                    >
                        {spec.icon}
                        {typeof badge === "number" && badge > 0 && (
                            <span
                                aria-label={`${badge} ${spec.label}`}
                                style={{
                                    position: "absolute",
                                    top: 4,
                                    right: 4,
                                    minWidth: 14,
                                    height: 14,
                                    padding: "0 3px",
                                    borderRadius: 999,
                                    background: palette.danger,
                                    color: "#fff",
                                    fontSize: 9,
                                    fontWeight: 600,
                                    lineHeight: 1,
                                    display: "inline-flex",
                                    alignItems: "center",
                                    justifyContent: "center",
                                }}
                            >
                                {badge > 9 ? "9+" : badge}
                            </span>
                        )}
                    </button>
                );
            })}
        </nav>
    );
}
