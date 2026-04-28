import { RotateCcw } from "lucide-react";
import { toast } from "sonner";

import Card from "../components/Card";
import PageShell from "../components/PageShell";
import { palette, space } from "../layout/theme";
import {
    PreferenceDefs,
    preferenceDefaults,
    resetPreference,
    usePreference,
} from "../lib/preferences";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "@/components/ui/select";

// Preferences is the *client-local* settings surface. Storage scope is
// "this browser × this server profile" (see lib/preferences.ts), which
// is why it lives at /preferences (top-level, account-flavoured)
// rather than under /projects/:slug/settings — switching projects
// must not reset these, and they don't sync across browsers / devices.
//
// Three tabs:
//   · Display    — UI density, default Fleet view, Activities default range
//   · Terminal   — xterm font size, cursor blink, scrollback
//   · Behaviour  — confirmation prompts and other ad-hoc client flags
//
// Every value flows through usePreference / writePreference; no server
// round-trip. To add a preference: declare it in PreferenceDefs +
// DEFAULTS in lib/preferences.ts and add a SettingRow here.
export default function Preferences() {
    return (
        <PageShell
            title="Preferences"
            subtitle="This browser only · not synced across devices"
            bodyPadding={8}
        >
            <div style={{ maxWidth: 720 }}>
                <Tabs defaultValue="display">
                    <TabsList
                        data-testid="preferences-tabs"
                        className="mb-4"
                    >
                        <TabsTrigger value="display">Display</TabsTrigger>
                        <TabsTrigger value="terminal">Terminal</TabsTrigger>
                        <TabsTrigger value="behaviour">Behaviour</TabsTrigger>
                    </TabsList>
                    <TabsContent value="display" className="space-y-4">
                        <DisplayTab />
                    </TabsContent>
                    <TabsContent value="terminal" className="space-y-4">
                        <TerminalTab />
                    </TabsContent>
                    <TabsContent value="behaviour" className="space-y-4">
                        <BehaviourTab />
                    </TabsContent>
                </Tabs>
            </div>
        </PageShell>
    );
}

function DisplayTab() {
    const [density, setDensity] = usePreference("ui.density");
    const [fleetView, setFleetView] = usePreference("ui.fleet.defaultView");
    const [activitiesRange, setActivitiesRange] = usePreference(
        "ui.activities.defaultRange",
    );

    return (
        <Card header="Display" padding={5}>
            <SettingRow
                label="UI density"
                description="Comfortable spacing reads better; compact fits more rows on a small screen."
            >
                <Select
                    value={density}
                    onValueChange={(v) => setDensity(v as PreferenceDefs["ui.density"])}
                >
                    <SelectTrigger className="h-8 w-[180px]">
                        <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                        <SelectItem value="comfortable">Comfortable</SelectItem>
                        <SelectItem value="compact">Compact</SelectItem>
                    </SelectContent>
                </Select>
            </SettingRow>

            <SettingRow
                label="Default Fleet view"
                description="Which view (cards, table, timeline, graph) Fleet opens to."
            >
                <Select
                    value={fleetView}
                    onValueChange={(v) =>
                        setFleetView(v as PreferenceDefs["ui.fleet.defaultView"])
                    }
                >
                    <SelectTrigger className="h-8 w-[180px]">
                        <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                        <SelectItem value="cards">Cards</SelectItem>
                        <SelectItem value="table">Table</SelectItem>
                        <SelectItem value="timeline">Timeline</SelectItem>
                        <SelectItem value="graph">Graph</SelectItem>
                    </SelectContent>
                </Select>
            </SettingRow>

            <SettingRow
                label="Activities default range"
                description="Time window applied when first opening Activities."
            >
                <Select
                    value={activitiesRange}
                    onValueChange={(v) =>
                        setActivitiesRange(
                            v as PreferenceDefs["ui.activities.defaultRange"],
                        )
                    }
                >
                    <SelectTrigger className="h-8 w-[180px]">
                        <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                        <SelectItem value="24h">Last 24 hours</SelectItem>
                        <SelectItem value="7d">Last 7 days</SelectItem>
                        <SelectItem value="30d">Last 30 days</SelectItem>
                        <SelectItem value="all">All time</SelectItem>
                    </SelectContent>
                </Select>
            </SettingRow>

            <ResetRow
                keys={[
                    "ui.density",
                    "ui.fleet.defaultView",
                    "ui.activities.defaultRange",
                ]}
            />
        </Card>
    );
}

function TerminalTab() {
    const [fontSize, setFontSize] = usePreference("terminal.fontSize");
    const [cursorBlink, setCursorBlink] = usePreference("terminal.cursorBlink");
    const [scrollback, setScrollback] = usePreference("terminal.scrollback");

    return (
        <Card header="Terminal" padding={5}>
            <SettingRow
                label="Font size"
                description="Pixel size of the xterm cell. Defaults to 13."
            >
                <Input
                    type="number"
                    min={9}
                    max={28}
                    value={fontSize}
                    onChange={(e) => {
                        const n = parseInt(e.target.value, 10);
                        if (Number.isFinite(n)) setFontSize(clamp(n, 9, 28));
                    }}
                    className="h-8 w-[100px]"
                />
            </SettingRow>

            <SettingRow
                label="Blinking cursor"
                description="Animate the cursor block. Disable on low-end remote displays to save GPU."
            >
                <Switch
                    checked={cursorBlink}
                    onCheckedChange={(v) => setCursorBlink(v)}
                />
            </SettingRow>

            <SettingRow
                label="Scrollback lines"
                description="How many lines xterm keeps in its scrollback buffer per shell."
            >
                <Input
                    type="number"
                    min={500}
                    max={50000}
                    step={500}
                    value={scrollback}
                    onChange={(e) => {
                        const n = parseInt(e.target.value, 10);
                        if (Number.isFinite(n)) setScrollback(clamp(n, 500, 50000));
                    }}
                    className="h-8 w-[120px]"
                />
            </SettingRow>

            <ResetRow
                keys={[
                    "terminal.fontSize",
                    "terminal.cursorBlink",
                    "terminal.scrollback",
                ]}
            />
        </Card>
    );
}

function BehaviourTab() {
    const [confirmDelete, setConfirmDelete] = usePreference("ui.confirmDelete");

    return (
        <Card header="Behaviour" padding={5}>
            <SettingRow
                label="Confirm before deleting"
                description="Show a confirmation dialog before destructive actions. Off skips the prompt — use only if you trust your muscle memory."
            >
                <Switch
                    checked={confirmDelete}
                    onCheckedChange={(v) => setConfirmDelete(v)}
                />
            </SettingRow>

            <ResetRow keys={["ui.confirmDelete"]} />
        </Card>
    );
}

interface SettingRowProps {
    label: string;
    description?: string;
    children: React.ReactNode;
}

function SettingRow({ label, description, children }: SettingRowProps) {
    return (
        <div
            style={{
                display: "flex",
                alignItems: "flex-start",
                justifyContent: "space-between",
                gap: space[4],
                padding: `${space[3]}px 0`,
                borderBottom: `1px solid ${palette.border}`,
            }}
        >
            <div style={{ flex: 1, minWidth: 0 }}>
                <Label
                    style={{
                        display: "block",
                        marginBottom: 4,
                        color: palette.textPrimary,
                        fontSize: 13,
                        fontWeight: 500,
                    }}
                >
                    {label}
                </Label>
                {description && (
                    <p
                        style={{
                            fontSize: 12,
                            color: palette.textMuted,
                            lineHeight: 1.5,
                            margin: 0,
                        }}
                    >
                        {description}
                    </p>
                )}
            </div>
            <div style={{ flexShrink: 0 }}>{children}</div>
        </div>
    );
}

function ResetRow({ keys }: { keys: Array<keyof PreferenceDefs> }) {
    const onReset = () => {
        keys.forEach((k) => resetPreference(k));
        toast.success(
            `Reset ${keys.length} setting${keys.length === 1 ? "" : "s"} to default`,
        );
    };

    const defaultsSummary = keys
        .map((k) => `${labelFor(k)}: ${formatValue(preferenceDefaults[k])}`)
        .join(" · ");

    return (
        <div
            style={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                gap: space[4],
                paddingTop: space[3],
            }}
        >
            <span
                style={{
                    fontSize: 11,
                    color: palette.textMuted,
                    fontFamily: "var(--font-geist-mono)",
                }}
                title={defaultsSummary}
            >
                Defaults: {defaultsSummary}
            </span>
            <Button variant="outline" size="sm" onClick={onReset}>
                <RotateCcw className="size-3.5" />
                Reset to default
            </Button>
        </div>
    );
}

function labelFor(k: keyof PreferenceDefs): string {
    return k.split(".").slice(-1)[0];
}

function formatValue(v: PreferenceDefs[keyof PreferenceDefs]): string {
    if (typeof v === "boolean") return v ? "on" : "off";
    return String(v);
}

function clamp(n: number, lo: number, hi: number): number {
    return Math.max(lo, Math.min(hi, n));
}
