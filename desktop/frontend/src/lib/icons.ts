// Single source of truth for "this domain noun is rendered as that
// lucide icon". Without this registry, the same noun ("host" /
// "agent" / "session" / "fleet") drifted into three or four different
// icons across pages, training muscle memory for the wrong concept.
//
// Every nav surface, status pill, and inline glyph that represents a
// domain noun should import from here rather than reaching directly
// into lucide-react. New noun? Add it once, import everywhere.

import {
    Activity,
    AppWindow,
    ArrowDownUp,
    Cable,
    Clock,
    CloudDownload,
    Command,
    File,
    Folder,
    HardDrive,
    History,
    KeyRound,
    LayoutGrid,
    LineChart,
    Monitor,
    Network,
    Plug,
    Server,
    ServerCog,
    Settings2,
    ShieldCheck,
    ShieldHalf,
    Circle,
    Terminal,
    Users,
    Wrench,
    Zap,
} from "lucide-react";

// Map domain nouns to their canonical icon. Keys are the user-facing
// concept names (matching the IA + sidebar labels), values are
// lucide-react components.
//
// Adding here is cheap; introducing two icons for the same noun via
// direct lucide imports elsewhere is what we're trying to avoid.
export const icons = {
    // Top-bar navigation (project context)
    project: LayoutGrid,
    fleet: Monitor,
    operations: Wrench,
    history: History,
    members: Users,
    settings: Settings2,

    // Top-bar navigation (global context)
    projects: LayoutGrid,
    servers: ServerCog,
    admin: ShieldHalf,

    // Surfaces nested inside History / Operations
    activity: Clock,
    transfers: ArrowDownUp,
    recordings: Circle,
    enrollment: CloudDownload,

    // Domain entities
    host: Server,
    session: Plug,
    accessToken: KeyRound,
    installCommand: Zap,
    mesh: Network,
    file: File,
    folder: Folder,
    process: AppWindow,
    disk: HardDrive,

    // Affordances
    shell: Terminal,
    tunnel: Cable,
    chart: LineChart,
    audit: ShieldCheck,
    health: Activity,

    // App controls
    palette: Command,
} as const;

// IconName is the closed set of registry keys; useful when a component
// wants to take a noun string and look the icon up at runtime.
export type IconName = keyof typeof icons;
