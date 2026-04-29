import {
    ReactNode,
    createContext,
    useCallback,
    useContext,
    useEffect,
    useMemo,
    useState,
} from "react";
import { toast } from "sonner";

import { onActiveChange } from "../lib/auth";
import { getActiveServerId } from "../lib/servers";

const MAX_SHELLS = 8;
const DEFAULT_HEIGHT = 320;
const MIN_HEIGHT = 140;
const MAX_HEIGHT_RATIO = 0.85;

// Drawer layout state (height / open) is persisted per active server
// so switching between workspaces doesn't leak one user's preferred
// drawer size into another. When no active server is set we skip
// persistence entirely — the drawer is only meaningful inside an
// authenticated project shell anyway. Legacy unscoped keys are read
// once and copied under the active server's namespace the first
// time this module runs with a session.
const LEGACY_HEIGHT_KEY = "platypus.terminalDrawer.height";
const LEGACY_OPEN_KEY = "platypus.terminalDrawer.open";

function heightKey(serverId: string): string {
    return `platypus.${serverId}.terminalDrawer.height`;
}
function openKey(serverId: string): string {
    return `platypus.${serverId}.terminalDrawer.open`;
}

export interface ShellEntry {
    id: string;
    label: string;
    projectID: string;
    projectSlug: string;
    hostId: string;
    sessionHash: string;
    // Optional one-shot command piped into the shell as soon as
    // the agent reports the terminal as open. Only sent on the
    // first attach for this entry; clearing it after the send
    // prevents duplicate execution if <Terminal> remounts (e.g.
    // drawer collapse / expand).
    initialCommand?: string;
}

export interface OpenShellInput {
    projectID: string;
    projectSlug: string;
    hostId: string;
    sessionHash: string;
    label: string;
    initialCommand?: string;
}

interface GlobalTerminalContextValue {
    shells: ShellEntry[];
    activeId: string | null;
    drawerOpen: boolean;
    drawerHeight: number;
    openShell: (input: OpenShellInput) => string | null;
    closeShell: (id: string) => void;
    setActive: (id: string) => void;
    toggleDrawer: () => void;
    openDrawer: () => void;
    closeDrawer: () => void;
    setDrawerHeight: (h: number) => void;
    // consumeInitialCommand returns the pending one-shot command for
    // a shell (if any) and atomically clears it. The Terminal mount
    // calls this once the WebSocket reports open so the cd/seed only
    // ever runs on first attach — a remount (drawer collapse /
    // expand, route nav within the same host) returns undefined.
    consumeInitialCommand: (id: string) => string | undefined;
}

const GlobalTerminalContext = createContext<GlobalTerminalContextValue | null>(null);

let shellCounter = 0;
function nextShellId(): string {
    shellCounter += 1;
    return `shell-${Date.now()}-${shellCounter}`;
}

function readPersistedHeight(serverId: string | null): number {
    try {
        const raw = serverId
            ? localStorage.getItem(heightKey(serverId)) ??
              localStorage.getItem(LEGACY_HEIGHT_KEY)
            : localStorage.getItem(LEGACY_HEIGHT_KEY);
        if (!raw) return DEFAULT_HEIGHT;
        const n = parseInt(raw, 10);
        if (Number.isNaN(n)) return DEFAULT_HEIGHT;
        return clampHeight(n);
    } catch {
        return DEFAULT_HEIGHT;
    }
}

function readPersistedOpen(serverId: string | null): boolean {
    try {
        const raw = serverId
            ? localStorage.getItem(openKey(serverId)) ??
              localStorage.getItem(LEGACY_OPEN_KEY)
            : localStorage.getItem(LEGACY_OPEN_KEY);
        return raw === "1";
    } catch {
        return false;
    }
}

function clampHeight(h: number): number {
    const max = typeof window !== "undefined"
        ? Math.floor(window.innerHeight * MAX_HEIGHT_RATIO)
        : 800;
    return Math.max(MIN_HEIGHT, Math.min(max, h));
}

export function GlobalTerminalProvider({ children }: { children: ReactNode }) {
    const [shells, setShells] = useState<ShellEntry[]>([]);
    const [activeId, setActiveId] = useState<string | null>(null);
    const [activeServerId, setActiveServerIdState] = useState<string | null>(() =>
        getActiveServerId(),
    );
    const [drawerOpen, setDrawerOpen] = useState<boolean>(() =>
        readPersistedOpen(activeServerId),
    );
    const [drawerHeight, setDrawerHeightState] = useState<number>(() =>
        readPersistedHeight(activeServerId),
    );

    // Rehydrate when the user switches servers so each workspace
    // remembers its own drawer size.
    useEffect(() => {
        const unsub = onActiveChange(() => {
            const next = getActiveServerId();
            setActiveServerIdState(next);
            setDrawerHeightState(readPersistedHeight(next));
            setDrawerOpen(readPersistedOpen(next));
        });
        return unsub;
    }, []);

    useEffect(() => {
        if (!activeServerId) return;
        try {
            localStorage.setItem(heightKey(activeServerId), String(drawerHeight));
        } catch {
            // ignore quota / disabled storage
        }
    }, [drawerHeight, activeServerId]);

    useEffect(() => {
        if (!activeServerId) return;
        try {
            localStorage.setItem(openKey(activeServerId), drawerOpen ? "1" : "0");
        } catch {
            // ignore
        }
    }, [drawerOpen]);

    const openShell = useCallback((input: OpenShellInput): string | null => {
        let created: string | null = null;
        setShells((prev) => {
            if (prev.length >= MAX_SHELLS) {
                toast.warning(`Shell limit reached (${MAX_SHELLS}). Close a shell to open a new one.`);
                return prev;
            }
            const id = nextShellId();
            const existingForHost = prev.filter((s) => s.hostId === input.hostId).length;
            const label = existingForHost > 0
                ? `${input.label} · ${existingForHost + 1}`
                : input.label;
            created = id;
            return [
                ...prev,
                {
                    id,
                    label,
                    projectID: input.projectID,
                    projectSlug: input.projectSlug,
                    hostId: input.hostId,
                    sessionHash: input.sessionHash,
                    initialCommand: input.initialCommand,
                },
            ];
        });
        if (created) {
            setActiveId(created);
            setDrawerOpen(true);
        }
        return created;
    }, []);

    const closeShell = useCallback((id: string) => {
        setShells((prev) => {
            const next = prev.filter((s) => s.id !== id);
            setActiveId((current) => {
                if (current !== id) return current;
                if (next.length === 0) return null;
                return next[next.length - 1].id;
            });
            return next;
        });
    }, []);

    const setActive = useCallback((id: string) => {
        setActiveId(id);
        setDrawerOpen(true);
    }, []);

    const toggleDrawer = useCallback(() => {
        setDrawerOpen((v) => !v);
    }, []);

    const openDrawer = useCallback(() => setDrawerOpen(true), []);
    const closeDrawer = useCallback(() => setDrawerOpen(false), []);

    const setDrawerHeight = useCallback((h: number) => {
        setDrawerHeightState(clampHeight(h));
    }, []);

    const consumeInitialCommand = useCallback((id: string): string | undefined => {
        let cmd: string | undefined;
        setShells((prev) => {
            const idx = prev.findIndex((s) => s.id === id);
            if (idx < 0) return prev;
            cmd = prev[idx].initialCommand;
            if (!cmd) return prev;
            const next = prev.slice();
            next[idx] = { ...prev[idx], initialCommand: undefined };
            return next;
        });
        return cmd;
    }, []);

    const value = useMemo<GlobalTerminalContextValue>(
        () => ({
            shells,
            activeId,
            drawerOpen,
            drawerHeight,
            openShell,
            closeShell,
            setActive,
            toggleDrawer,
            openDrawer,
            closeDrawer,
            setDrawerHeight,
            consumeInitialCommand,
        }),
        [
            shells,
            activeId,
            drawerOpen,
            drawerHeight,
            openShell,
            closeShell,
            setActive,
            toggleDrawer,
            openDrawer,
            closeDrawer,
            setDrawerHeight,
            consumeInitialCommand,
        ],
    );

    return (
        <GlobalTerminalContext.Provider value={value}>
            {children}
        </GlobalTerminalContext.Provider>
    );
}

export function useGlobalTerminal(): GlobalTerminalContextValue {
    const ctx = useContext(GlobalTerminalContext);
    if (!ctx) throw new Error("useGlobalTerminal must be used inside GlobalTerminalProvider");
    return ctx;
}

// useGlobalTerminalSafe returns null when no provider is mounted
// instead of throwing. Used by surfaces that may render outside the
// project shell (e.g. StatusBar on the login screen, or in unit
// tests) where "no terminals" is the correct answer rather than a
// crash.
export function useGlobalTerminalSafe(): GlobalTerminalContextValue | null {
    return useContext(GlobalTerminalContext);
}
