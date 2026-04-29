// Spec for the upgrade-agent button + confirmation dialog.
// Mounts the component directly so the spec doesn't pay the cost of
// HostView's full surface (sessions list, terminal context,
// auto-open-shell, etc.).

import { describe, expect, it, vi } from "vitest";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";

const toastMocks = vi.hoisted(() => ({
    success: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
    warning: vi.fn(),
}));
vi.mock("sonner", () => ({
    toast: toastMocks,
}));

vi.mock("../../lib/api", () => ({
    triggerAgentUpgrade: vi.fn(),
}));

import { triggerAgentUpgrade } from "../../lib/api";
import { UpgradeAgentButton } from "./UpgradeAgentButton";
import { BuildVersionValue } from "./pills";

function makeClient(): QueryClient {
    return new QueryClient({
        defaultOptions: {
            queries: { retry: false, refetchOnMount: false, refetchOnWindowFocus: false },
            mutations: { retry: false },
        },
    });
}

function renderInClient(ui: ReactNode) {
    const client = makeClient();
    function Wrapper({ children }: { children: ReactNode }) {
        return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
    }
    return render(<>{ui}</>, { wrapper: Wrapper });
}

// BuildVersionValue moved to pills.tsx in the host-page refactor.
// Keep the rendering specs here so a future refactor can't silently
// break the mismatch indicator — the upgrade button's UX is built
// on top of the same comparison logic.
describe("<BuildVersionValue>", () => {
    it("renders an em-dash when version is unknown", () => {
        renderInClient(<BuildVersionValue version={undefined} latest="1.6.0" />);
        expect(document.body.textContent).toContain("—");
        expect(screen.queryByText(/up to date/i)).toBeNull();
    });

    it("renders an `up to date` pill when version matches latest", () => {
        renderInClient(<BuildVersionValue version="1.6.0" latest="1.6.0" />);
        expect(screen.getByText(/up to date/i)).toBeInTheDocument();
    });

    it("renders an outdated pill citing latest when versions differ", () => {
        renderInClient(<BuildVersionValue version="1.5.0" latest="1.6.0" />);
        const pill = screen.getByText(/outdated/i);
        expect(pill.textContent).toMatch(/1\.6\.0/);
    });

    it("omits the pill entirely when there's nothing to compare against", () => {
        renderInClient(<BuildVersionValue version="1.5.0" latest={undefined} />);
        expect(screen.queryByText(/outdated/i)).toBeNull();
        expect(screen.queryByText(/up to date/i)).toBeNull();
    });
});

describe("<UpgradeAgentButton>", () => {
    it("does not render when no manifest version is available", () => {
        renderInClient(
            <UpgradeAgentButton
                projectID="p1"
                hostID="h1"
                currentVersion="1.5.0"
                latestVersion={undefined}
            />,
        );
        expect(screen.queryByTestId("host-upgrade-button")).toBeNull();
    });

    it("renders an `Upgrade` button when host build_version trails latest", () => {
        renderInClient(
            <UpgradeAgentButton
                projectID="p1"
                hostID="h1"
                currentVersion="1.5.0"
                latestVersion="1.6.0"
            />,
        );
        const btn = screen.getByTestId("host-upgrade-button");
        expect(btn.textContent).toMatch(/^Upgrade$/i);
        expect(btn).not.toBeDisabled();
    });

    it("renders a `Reinstall` button when host is already on latest", () => {
        renderInClient(
            <UpgradeAgentButton
                projectID="p1"
                hostID="h1"
                currentVersion="1.6.0"
                latestVersion="1.6.0"
            />,
        );
        const btn = screen.getByTestId("host-upgrade-button");
        expect(btn.textContent).toMatch(/^Reinstall$/i);
    });

    it("opens an AlertDialog naming the target version on click", async () => {
        renderInClient(
            <UpgradeAgentButton
                projectID="p1"
                hostID="h1"
                currentVersion="1.5.0"
                latestVersion="1.6.0"
            />,
        );
        fireEvent.click(screen.getByTestId("host-upgrade-button"));
        const dialog = await screen.findByRole("alertdialog");
        expect(dialog.textContent).toMatch(/1\.6\.0/);
        expect(dialog.textContent).toMatch(/Upgrade agent to/i);
    });

    it("fires triggerAgentUpgrade with target_version=latest on confirm", async () => {
        const trigger = vi.mocked(triggerAgentUpgrade);
        trigger.mockResolvedValueOnce({
            status: "exited",
            phase: "PHASE_EXITING",
            resolved_version: "1.6.0",
        });
        renderInClient(
            <UpgradeAgentButton
                projectID="p1"
                hostID="h1"
                currentVersion="1.5.0"
                latestVersion="1.6.0"
            />,
        );
        fireEvent.click(screen.getByTestId("host-upgrade-button"));
        fireEvent.click(await screen.findByTestId("host-upgrade-confirm"));
        await waitFor(() => {
            expect(trigger).toHaveBeenCalledWith("p1", "h1", {
                target_version: "1.6.0",
                channel: "stable",
            });
        });
    });
});
