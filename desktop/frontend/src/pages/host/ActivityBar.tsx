import { ReactNode } from "react";
import {
    Cable,
    KeyRound,
    Puzzle,
    ShieldCheck,
    type LucideProps,
} from "lucide-react";

import { palette } from "../../layout/theme";
import {
    entryActivityKey,
    entryReady,
    type PluginUIEntry,
} from "./plugins/registry";

// First-party (hardcoded) activities.  Plugin-shipped activities
// are passed in as props and rendered after a divider.
//
// "files" / "info" (Q2) and "sessions" / "processes" (Q3) lived here
// historically; they're now PLUGIN_UI_REGISTRY entries with stable
// activityKey overrides preserving their URL slugs. The remaining
// hardcoded list will continue to shrink in Q4-Q5 until everything
// flows through the registry.
export const FIRST_PARTY_ACTIVITIES = [
    "security",
    "config",
    "tunnels",
    "plugins",
] as const;
export type FirstPartyActivity = (typeof FIRST_PARTY_ACTIVITIES)[number];

/**
 * Activity is either a hardcoded first-party slug ("processes",
 * "sessions", etc.) or a plugin-shipped key of the form
 * "plugin/<plugin_id>". Plugin keys are produced by
 * `pluginActivityKey` from registry.ts.
 */
export type Activity = string;
// Backward-compat alias — older code imports `ACTIVITIES` for the
// hardcoded list. Keeping the name avoids a sweep through callers.
export const ACTIVITIES = FIRST_PARTY_ACTIVITIES;

interface ActivitySpec {
    key: FirstPartyActivity;
    label: string;
    icon: ReactNode;
}

const ICON_SIZE = "size-4";

export const ACTIVITY_SPECS: ActivitySpec[] = [
    { key: "security", label: "Security", icon: <ShieldCheck className={ICON_SIZE} /> },
    { key: "config", label: "Config", icon: <KeyRound className={ICON_SIZE} /> },
    { key: "tunnels", label: "Tunnels", icon: <Cable className={ICON_SIZE} /> },
    { key: "plugins", label: "Plugins", icon: <Puzzle className={ICON_SIZE} /> },
];

interface Props {
    active: Activity;
    onSelect: (activity: Activity) => void;
    /**
     * Numeric badges keyed by activityKey (so first-party slugs like
     * "sessions" + plugin-shipped keys like "plugin:com.platypus.x"
     * share one map). Rendered as a small red pill in the icon's
     * top-right corner; values <=0 / undefined → no pill. The pill
     * caps display at "9+" but accepts the raw number for the
     * aria-label.
     */
    badges?: Record<string, number | undefined>;
    /**
     * Activities whose required plugins aren't installed yet. The
     * bar dims those icons + appends "(needs plugin)" to the
     * tooltip so the operator sees up front which tabs are gated.
     * Selecting a dimmed icon still works — the tab body shows a
     * RequiresPlugins install guide.
     */
    needsInstall?: Partial<Record<FirstPartyActivity, boolean>>;
    /**
     * Plugin-shipped activity entries (PLUGIN_UI_REGISTRY filtered
     * by `installed plugins ∩ host.os` + alwaysVisible). Rendered
     * after a horizontal divider. Each one's activity key follows
     * the `plugin:<plugin_id>` convention defined in registry.ts
     * (overridable per-entry via entry.activityKey).
     */
    pluginEntries?: ReadonlyArray<PluginUIEntry>;
    /**
     * Plugin ids the operator hasn't clicked yet. Each renders a
     * small "new" dot on its icon — same affordance VS Code uses
     * for newly-installed extensions. The parent's onSelect should
     * call `markSeen(pluginID)` to clear the dot on click.
     */
    newPluginIDs?: ReadonlySet<string>;
    /**
     * Set of installed plugin ids (the "agent's installed plugins"
     * cache). Used to dim alwaysVisible plugin entries whose
     * required plugins aren't all present, matching the existing
     * hardcoded-tab dimming pattern (orange dot indicator on the
     * icon corner). Optional; when undefined, no dimming.
     */
    installedPluginIDs?: ReadonlySet<string>;
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

export default function ActivityBar({
    active,
    onSelect,
    badges,
    needsInstall,
    pluginEntries,
    newPluginIDs,
    installedPluginIDs,
}: Props) {
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
                const dimmed = !!needsInstall?.[spec.key];
                // Dimmed icons stay clickable — the tab body shows
                // the install guide. The visual cue is colour, not
                // pointer-events: blocking the click would hide
                // the install affordance behind a tooltip.
                const tooltip = dimmed ? `${spec.label} (needs plugin)` : spec.label;
                return (
                    <button
                        key={spec.key}
                        type="button"
                        onClick={() => onSelect(spec.key)}
                        data-testid={`host-activity-${spec.key}`}
                        data-active={isActive || undefined}
                        data-needs-install={dimmed || undefined}
                        aria-current={isActive ? "true" : undefined}
                        title={tooltip}
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
                            // Active wins over dimmed (showing a
                            // non-installed tab the operator clicked
                            // through to is the default — the badge
                            // dot communicates "still needs install").
                            color: isActive ? palette.textPrimary : palette.textMuted,
                            opacity: dimmed && !isActive ? 0.45 : 1,
                            // Left bar marks the active activity in VSCode
                            // fashion — a 2px accent stripe flush against
                            // the inner edge of the bar.
                            boxShadow: isActive
                                ? `inset 2px 0 0 0 ${palette.accent}`
                                : undefined,
                        }}
                    >
                        {spec.icon}
                        {dimmed && (
                            <span
                                aria-hidden
                                title=""
                                style={{
                                    position: "absolute",
                                    bottom: 6,
                                    right: 8,
                                    width: 6,
                                    height: 6,
                                    borderRadius: 999,
                                    background: palette.warning,
                                }}
                            />
                        )}
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

            {pluginEntries && pluginEntries.length > 0 && (
                <>
                    <div
                        aria-hidden
                        data-testid="host-activity-plugin-divider"
                        style={{
                            margin: "8px 8px",
                            height: 1,
                            background: palette.border,
                        }}
                    />
                    {pluginEntries.map((entry) => {
                        const key = entryActivityKey(entry);
                        const ready = installedPluginIDs
                            ? entryReady(entry, installedPluginIDs)
                            : true;
                        return (
                            <PluginActivityButton
                                key={entry.pluginID}
                                entry={entry}
                                activityKey={key}
                                isActive={active === key}
                                isNew={
                                    newPluginIDs?.has(entry.pluginID) ?? false
                                }
                                isDimmed={!ready}
                                badge={badges?.[key]}
                                onSelect={onSelect}
                            />
                        );
                    })}
                </>
            )}
        </nav>
    );
}

function PluginActivityButton({
    entry,
    activityKey,
    isActive,
    isNew,
    isDimmed,
    badge,
    onSelect,
}: {
    entry: PluginUIEntry;
    activityKey: string;
    isActive: boolean;
    isNew: boolean;
    /**
     * True when not all of entry.requiredPluginIDs are installed.
     * For alwaysVisible: true entries this means the icon shows
     * dimmed with an orange dot at the bottom-right (matching the
     * legacy hardcoded-tab missing-plugin affordance); clicking
     * still works — the activity body shows the install guide.
     */
    isDimmed: boolean;
    /** Numeric pill in the icon's top-right corner (e.g. session count). */
    badge: number | undefined;
    onSelect: (a: Activity) => void;
}) {
    const Icon = entry.icon as React.ComponentType<LucideProps>;
    const tooltipParts: string[] = [entry.title];
    if (isDimmed) tooltipParts.push("(needs plugin)");
    if (isNew) tooltipParts.push("(new)");
    return (
        <button
            type="button"
            onClick={() => onSelect(activityKey)}
            data-testid={`host-activity-${activityKey}`}
            data-plugin-activity={entry.pluginID}
            data-active={isActive || undefined}
            data-new={isNew || undefined}
            data-needs-install={isDimmed || undefined}
            aria-current={isActive ? "true" : undefined}
            title={tooltipParts.join(" ")}
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
                opacity: isDimmed && !isActive ? 0.45 : 1,
                boxShadow: isActive ? `inset 2px 0 0 0 ${palette.accent}` : undefined,
            }}
        >
            <Icon className={ICON_SIZE} />
            {isDimmed && (
                <span
                    aria-hidden
                    title=""
                    data-testid={`host-activity-needs-install-${entry.pluginID}`}
                    style={{
                        position: "absolute",
                        bottom: 6,
                        right: 8,
                        width: 6,
                        height: 6,
                        borderRadius: 999,
                        background: palette.warning,
                    }}
                />
            )}
            {isNew && (
                <span
                    aria-label="new"
                    data-testid={`host-activity-new-${entry.pluginID}`}
                    style={{
                        position: "absolute",
                        top: 6,
                        right: 8,
                        width: 8,
                        height: 8,
                        borderRadius: 999,
                        background: palette.accent,
                        boxShadow: `0 0 0 2px ${palette.rail}`,
                    }}
                />
            )}
            {typeof badge === "number" && badge > 0 && !isNew && (
                <span
                    aria-label={`${badge} ${entry.title}`}
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
}
