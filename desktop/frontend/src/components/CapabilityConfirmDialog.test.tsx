import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

import CapabilityConfirmDialog, {
    DeclaredCapability,
} from "./CapabilityConfirmDialog";

// CapabilityConfirmDialog is the gate before any plugin install.
// Contract:
//   1. Default state grants nothing — a hostile or buggy plugin
//      install must NEVER yield permissions the operator didn't
//      explicitly tick. (User decision: "more secure" in the
//      original requirements thread.)
//   2. The structured upper-bound the manifest declared (paths,
//      commands, hosts) is shown read-only beneath each family
//      checkbox so the operator can see WHAT they're approving, not
//      just an opaque label.
//   3. High-risk families (exec, fs.write, net.http) carry a visible
//      risk badge — the colour drift is design-driven and not pinned
//      here, but the risk text must be present so screen readers see
//      it.
//   4. onApprove receives the selected family-name set; cancel
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

describe("<CapabilityConfirmDialog>", () => {
    it("shows the plugin name + version in the header", () => {
        render(<CapabilityConfirmDialog {...baseProps()} />);
        expect(screen.getByText(/Test Plugin/i)).toBeInTheDocument();
        expect(screen.getByText(/1\.0\.0/)).toBeInTheDocument();
    });

    it("renders one row per declared capability", () => {
        render(<CapabilityConfirmDialog {...baseProps()} />);
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
        const install = screen.getByRole("button", { name: /install/i });
        fireEvent.click(install);
        expect(onApprove).toHaveBeenCalledWith([]);
    });

    it("propagates ticked families through onApprove", async () => {
        const onApprove = vi.fn();
        render(<CapabilityConfirmDialog {...baseProps({ onApprove })} />);

        // Tick the fs.read row. Checkboxes are addressed by the
        // accessible name set from the family label.
        const fsRead = screen.getByRole("checkbox", { name: /filesystem read/i });
        fireEvent.click(fsRead);

        const install = screen.getByRole("button", { name: /install/i });
        fireEvent.click(install);

        await waitFor(() => {
            expect(onApprove).toHaveBeenCalledWith(["fs.read"]);
        });
    });

    it("renders the manifest's path / command upper bounds beneath each family", () => {
        render(<CapabilityConfirmDialog {...baseProps()} />);
        // Paths from the fs.read entry.
        expect(screen.getByText("/proc")).toBeInTheDocument();
        expect(screen.getByText("/etc")).toBeInTheDocument();
        // Command from the exec entry.
        expect(screen.getByText("/usr/bin/curl")).toBeInTheDocument();
    });

    it("marks high-risk families so an operator review notices them", () => {
        render(<CapabilityConfirmDialog {...baseProps()} />);
        // The exec capability is high-risk per capabilityMeta. Match
        // the risk badge by text — colour / icon are presentation.
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

    it("orders high-risk families above low-risk ones", () => {
        render(<CapabilityConfirmDialog {...baseProps()} />);
        const labels = screen
            .getAllByRole("checkbox")
            .map((cb) => cb.getAttribute("aria-label") ?? "");
        // exec (high) must appear before log (low).
        const execIdx = labels.findIndex((l) => l.match(/execute commands/i));
        const logIdx = labels.findIndex((l) => l.match(/logging/i));
        expect(execIdx).toBeGreaterThanOrEqual(0);
        expect(logIdx).toBeGreaterThanOrEqual(0);
        expect(execIdx).toBeLessThan(logIdx);
    });
});
