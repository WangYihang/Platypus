import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

import CapabilityConfirmDialog, {
    DeclaredCapability,
} from "./CapabilityConfirmDialog";

// CapabilityConfirmDialog is the gate before any plugin install.
// Contract:
//   1. Default state grants nothing — a hostile or buggy plugin
//      install must NEVER yield permissions the operator didn't
//      explicitly tick.
//   2. Operators usually pick a COLLECTION (preset bundle) instead
//      of ticking individual capabilities. Collections are the
//      primary affordance; per-capability checkboxes are an
//      "Advanced" reveal so the dialog stays scannable.
//   3. Selecting a collection grants ONLY the families the plugin
//      actually declared — granting a family the plugin never
//      asked for would be meaningless.
//   4. The structured upper-bound the manifest declared (paths,
//      commands, hosts) is shown read-only beneath each family in
//      the advanced panel so the operator can see WHAT they're
//      approving, not just an opaque label.
//   5. onApprove receives the selected family-name set; cancel
//      doesn't fire it.

const declared: DeclaredCapability[] = [
    {
        family: "fs.read",
        paths: ["/proc", "/etc"],
    },
    {
        family: "exec",
        commands: ["/usr/bin/curl"],
    },
    {
        family: "log",
    },
];

function baseProps(overrides: Partial<React.ComponentProps<typeof CapabilityConfirmDialog>> = {}) {
    return {
        open: true,
        onOpenChange: vi.fn(),
        pluginID: "com.example.test",
        pluginVersion: "1.0.0",
        pluginName: "Test Plugin",
        declared,
        onApprove: vi.fn(),
        ...overrides,
    } as React.ComponentProps<typeof CapabilityConfirmDialog>;
}

function openAdvanced() {
    fireEvent.click(screen.getByRole("button", { name: /show advanced/i }));
}

describe("<CapabilityConfirmDialog>", () => {
    it("shows the plugin name + version in the header", () => {
        render(<CapabilityConfirmDialog {...baseProps()} />);
        expect(screen.getByText(/Test Plugin/i)).toBeInTheDocument();
        expect(screen.getByText(/1\.0\.0/)).toBeInTheDocument();
    });

    it("shows the collection grid above the advanced panel", () => {
        render(<CapabilityConfirmDialog {...baseProps()} />);
        // Each preset shows up as a button with its label. The
        // operator's primary affordance is picking one of these,
        // not ticking individual rows.
        expect(screen.getByRole("button", { name: /read-only inspection/i })).toBeInTheDocument();
        expect(screen.getByRole("button", { name: /file management/i })).toBeInTheDocument();
        expect(screen.getByRole("button", { name: /process control/i })).toBeInTheDocument();
        expect(screen.getByRole("button", { name: /network access/i })).toBeInTheDocument();
        expect(screen.getByRole("button", { name: /full access/i })).toBeInTheDocument();
    });

    it("renders one row per declared capability under advanced", () => {
        render(<CapabilityConfirmDialog {...baseProps()} />);
        openAdvanced();
        // Family labels come from capabilityMeta — match against the
        // human-readable label so the test stays decoupled from the
        // raw family string.
        expect(screen.getByText(/Filesystem read/i)).toBeInTheDocument();
        expect(screen.getByText(/Execute commands/i)).toBeInTheDocument();
        expect(screen.getByText(/Logging/i)).toBeInTheDocument();
    });

    it("starts with no capabilities granted (security-first default)", () => {
        const onApprove = vi.fn();
        render(<CapabilityConfirmDialog {...baseProps({ onApprove })} />);
        const install = screen.getByRole("button", { name: /^install$/i });
        fireEvent.click(install);
        expect(onApprove).toHaveBeenCalledWith([]);
    });

    it("propagates ticked families through onApprove", async () => {
        const onApprove = vi.fn();
        render(<CapabilityConfirmDialog {...baseProps({ onApprove })} />);

        openAdvanced();
        const fsRead = screen.getByRole("checkbox", { name: /filesystem read/i });
        fireEvent.click(fsRead);

        const install = screen.getByRole("button", { name: /^install$/i });
        fireEvent.click(install);

        await waitFor(() => {
            expect(onApprove).toHaveBeenCalledWith(["fs.read"]);
        });
    });

    it("clicking a collection grants its declared families in one shot", async () => {
        const onApprove = vi.fn();
        render(<CapabilityConfirmDialog {...baseProps({ onApprove })} />);

        // "Read-only inspection" wants log + sysinfo + fs.read + kv.
        // The plugin only declares fs.read + exec + log, so the click
        // should grant exactly { fs.read, log } — sysinfo / kv are
        // ignored because the plugin never asked for them.
        fireEvent.click(screen.getByRole("button", { name: /read-only inspection/i }));

        const install = screen.getByRole("button", { name: /^install$/i });
        fireEvent.click(install);

        await waitFor(() => {
            expect(onApprove).toHaveBeenCalled();
        });
        const granted = (onApprove.mock.calls[0]![0] as string[]).slice().sort();
        expect(granted).toEqual(["fs.read", "log"]);
    });

    it("the Custom card resets the grant set", async () => {
        const onApprove = vi.fn();
        render(<CapabilityConfirmDialog {...baseProps({ onApprove })} />);

        // Pick the broadest preset, then click Custom to wipe it.
        fireEvent.click(screen.getByRole("button", { name: /full access/i }));
        fireEvent.click(screen.getByRole("button", { name: /custom \/ clear/i }));

        const install = screen.getByRole("button", { name: /^install$/i });
        fireEvent.click(install);
        await waitFor(() => {
            expect(onApprove).toHaveBeenCalledWith([]);
        });
    });

    it("renders the manifest's path / command upper bounds in the advanced panel", () => {
        render(<CapabilityConfirmDialog {...baseProps()} />);
        openAdvanced();
        expect(screen.getByText("/proc")).toBeInTheDocument();
        expect(screen.getByText("/etc")).toBeInTheDocument();
        expect(screen.getByText("/usr/bin/curl")).toBeInTheDocument();
    });

    it("marks high-risk collections so an operator review notices them", () => {
        render(<CapabilityConfirmDialog {...baseProps()} />);
        // Collections + (after opening advanced) per-cap rows can each
        // carry a "High risk" badge. Just assert at least one is
        // present at the default (collection-only) view.
        expect(screen.getAllByText(/high risk/i).length).toBeGreaterThan(0);
    });

    it("Cancel does not call onApprove", () => {
        const onApprove = vi.fn();
        const onOpenChange = vi.fn();
        render(<CapabilityConfirmDialog {...baseProps({ onApprove, onOpenChange })} />);
        fireEvent.click(screen.getByRole("button", { name: /cancel/i }));
        expect(onApprove).not.toHaveBeenCalled();
        expect(onOpenChange).toHaveBeenCalledWith(false);
    });

    it("orders high-risk families above low-risk ones in the advanced panel", () => {
        render(<CapabilityConfirmDialog {...baseProps()} />);
        openAdvanced();
        const labels = screen
            .getAllByRole("checkbox")
            .map((cb) => cb.getAttribute("aria-label") ?? "");
        const execIdx = labels.findIndex((l) => l.match(/execute commands/i));
        const logIdx = labels.findIndex((l) => l.match(/logging/i));
        expect(execIdx).toBeGreaterThanOrEqual(0);
        expect(logIdx).toBeGreaterThanOrEqual(0);
        expect(execIdx).toBeLessThan(logIdx);
    });
});
