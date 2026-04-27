import { describe, expect, it, vi, beforeEach } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

vi.mock("sonner", () => ({
    toast: {
        success: vi.fn(),
        error: vi.fn(),
    },
}));

import CopyButton from "./CopyButton";
import { toast } from "sonner";

// CopyButton wraps navigator.clipboard.writeText with a labeled
// affordance and a success toast. Used wherever we display a long
// command, secret, or URL the operator is going to paste elsewhere.
//
// We use fireEvent rather than userEvent because userEvent.setup()
// installs its own clipboard implementation that masks our spy.

let writeText: ReturnType<typeof vi.fn>;

beforeEach(() => {
    writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", {
        configurable: true,
        value: { writeText },
    });
    vi.mocked(toast.success).mockReset();
    vi.mocked(toast.error).mockReset();
});

describe("<CopyButton>", () => {
    it("writes its text prop to the clipboard on click", async () => {
        render(<CopyButton text="hello world" />);
        fireEvent.click(screen.getByRole("button", { name: /copy/i }));
        await waitFor(() => {
            expect(writeText).toHaveBeenCalledWith("hello world");
        });
    });

    it("fires toast.success after a successful copy", async () => {
        render(<CopyButton text="x" label="Copy command" />);
        fireEvent.click(screen.getByRole("button", { name: /copy command/i }));
        await waitFor(() => {
            expect(toast.success).toHaveBeenCalled();
        });
    });
});
