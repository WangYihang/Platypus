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

const MAX_SHELLS = 8;
const HEIGHT_KEY = "platypus.terminalDrawer.height";
const OPEN_KEY = "platypus.terminalDrawer.open";
const DEFAULT_HEIGHT = 320;
const MIN_HEIGHT = 140;
const MAX_HEIGHT_RATIO = 0.85;

export interface ShellEntry {
    id: string;
    label: string;
    projectSlug: string;
    hostId: string;
    sessionHash: string;
}

export interface OpenShellInput {
    projectSlug: string;
    hostId: string;
    sessionHash: string;
    label: string;
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
}

const GlobalTerminalContext = createContext<GlobalTerminalContextValue | null>(null);

let shellCounter = 0;
function nextShellId(): string {
    shellCounter += 1;
    return `shell-${Date.now()}-${shellCounter}`;
}

function readPersistedHeight(): number {
    try {
        const raw = localStorage.getItem(HEIGHT_KEY);
        if (!raw) return DEFAULT_HEIGHT;
        const n = parseInt(raw, 10);
        if (Number.isNaN(n)) return DEFAULT_HEIGHT;
        return clampHeight(n);
    } catch {
        return DEFAULT_HEIGHT;
    }
}

function readPersistedOpen(): boolean {
    try {
        return localStorage.getItem(OPEN_KEY) === "1";
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
    const [drawerOpen, setDrawerOpen] = useState<boolean>(readPersistedOpen);
    const [drawerHeight, setDrawerHeightState] = useState<number>(readPersistedHeight);

    useEffect(() => {
        try {
            localStorage.setItem(HEIGHT_KEY, String(drawerHeight));
        } catch {
            // ignore quota / disabled storage
        }
    }, [drawerHeight]);

    useEffect(() => {
        try {
            localStorage.setItem(OPEN_KEY, drawerOpen ? "1" : "0");
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
                    projectSlug: input.projectSlug,
                    hostId: input.hostId,
                    sessionHash: input.sessionHash,
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
