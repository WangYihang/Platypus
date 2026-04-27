import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Loader2, RotateCcw, Trash2 } from "lucide-react";
import { toast } from "sonner";

import Card from "../components/Card";
import DataList from "../components/DataList";
import Mono from "../components/Mono";
import PageHeader from "../components/PageHeader";
import { useCurrentProject, useShell } from "../layout/ProjectShell";
import { palette, space } from "../layout/theme";
import { deleteProject } from "../lib/api";
import { humanizeError } from "../lib/humanizeError";
import {
    PreferenceDefs,
    preferenceDefaults,
    resetPreference,
    usePreference,
} from "../lib/preferences";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "@/components/ui/select";
import {
    AlertDialog,
    AlertDialogAction,
    AlertDialogCancel,
    AlertDialogContent,
    AlertDialogDescription,
    AlertDialogFooter,
    AlertDialogHeader,
    AlertDialogTitle,
} from "@/components/ui/alert-dialog";

// ProjectSettings groups configuration into four tabs:
//   · General — project identity (read-only) + danger zone
//   · Display — UI preferences (density, default Fleet view, …)
//   · Terminal — terminal preferences (font size, cursor, scrollback)
//   · Behaviour — confirmation prompts & misc client-side flags
//
// Display / Terminal / Behaviour persist to localStorage under
// "platypus.pref.*" via lib/preferences. Project identity is server-
// side and read-only here (rename has its own surface in admin).
export default function ProjectSettings() {
    const project = useCurrentProject();

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            <PageHeader title="Settings" subtitle={`Project · ${project.slug}`} />
            <div style={{ flex: 1, overflow: "auto", padding: space[8] }}>
                <div style={{ maxWidth: 720 }}>
                    <Tabs defaultValue="general">
                        <TabsList
                            data-testid="settings-tabs"
                            className="mb-4"
                        >
                            <TabsTrigger value="general">General</TabsTrigger>
                            <TabsTrigger value="display">Display</TabsTrigger>
                            <TabsTrigger value="terminal">Terminal</TabsTrigger>
                            <TabsTrigger value="behaviour">Behaviour</TabsTrigger>
                        </TabsList>
                        <TabsContent value="general" className="space-y-4">
                            <GeneralTab />
                        </TabsContent>
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
            </div>
        </div>
    );
}

function GeneralTab() {
    const project = useCurrentProject();
    const { refresh } = useShell();
    const navigate = useNavigate();
    const [deleting, setDeleting] = useState(false);
    const [confirmOpen, setConfirmOpen] = useState(false);

    const handleDelete = async () => {
        setDeleting(true);
        try {
            await deleteProject(project.id);
            toast.success(`Deleted project ${project.slug}`);
            await refresh();
            navigate("/projects", { replace: true });
        } catch (e) {
            toast.error(`delete: ${humanizeError(e)}`);
            setDeleting(false);
            setConfirmOpen(false);
        }
    };

    return (
        <>
            <Card header="Identity" padding={5}>
                <DataList
                    items={[
                        { label: "name", value: project.name },
                        { label: "slug", value: <Mono>{project.slug}</Mono> },
                        { label: "id", value: <Mono size={11}>{project.id}</Mono> },
                    ]}
                />
            </Card>

            <Card
                header={
                    <span style={{ color: palette.danger }}>Danger zone</span>
                }
                padding={5}
            >
                <div
                    style={{
                        display: "flex",
                        alignItems: "center",
                        justifyContent: "space-between",
                        gap: space[4],
                    }}
                >
                    <div
                        style={{
                            color: palette.textSecondary,
                            fontSize: 13,
                            lineHeight: 1.5,
                        }}
                    >
                        Deleting a project removes its hosts, sessions, tokens,
                        and install artifacts. This cannot be undone.
                    </div>
                    <Button
                        variant="destructive"
                        size="sm"
                        onClick={() => setConfirmOpen(true)}
                        disabled={deleting}
                    >
                        {deleting ? (
                            <Loader2 className="size-3.5 animate-spin" />
                        ) : (
                            <Trash2 className="size-3.5" />
                        )}
                        Delete project
                    </Button>
                </div>
            </Card>

            <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
                <AlertDialogContent>
                    <AlertDialogHeader>
                        <AlertDialogTitle>Delete this project?</AlertDialogTitle>
                        <AlertDialogDescription>
                            <span>Deleting </span>
                            <Mono>{project.slug}</Mono>
                            <span>
                                {" "}
                                permanently removes every host, session, token, and
                                install artifact inside it. This cannot be undone.
                            </span>
                        </AlertDialogDescription>
                    </AlertDialogHeader>
                    <AlertDialogFooter>
                        <AlertDialogCancel disabled={deleting}>Cancel</AlertDialogCancel>
                        <AlertDialogAction
                            onClick={(e) => {
                                e.preventDefault();
                                void handleDelete();
                            }}
                            disabled={deleting}
                        >
                            {deleting && <Loader2 className="size-3.5 animate-spin" />}
                            Delete
                        </AlertDialogAction>
                    </AlertDialogFooter>
                </AlertDialogContent>
            </AlertDialog>
        </>
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

            <ResetRow keys={["ui.density", "ui.fleet.defaultView", "ui.activities.defaultRange"]} />
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

            <ResetRow keys={["terminal.fontSize", "terminal.cursorBlink", "terminal.scrollback"]} />
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
