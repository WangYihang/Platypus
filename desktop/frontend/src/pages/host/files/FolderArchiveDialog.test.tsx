import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";

import FolderArchiveDialog from "./FolderArchiveDialog";

// FolderArchiveDialog is the modal that pops up when a download
// includes one or more folders. The download path doesn't make
// sense for folders without a packaging step — operators have to
// choose tar / tar.gz / zip — so this dialog is the only place that
// choice happens.
//
// Contract pinned here:
//   1. The dialog only renders when `open` is true.
//   2. It shows the names being archived (so operators see exactly
//      which selection they're packaging).
//   3. It exposes one radio per format (tar.gz / tar / zip).
//   4. The Download button calls onConfirm with the chosen format.
//   5. tar.gz is the default — the most common Linux operator pick.

describe("<FolderArchiveDialog>", () => {
    it("renders nothing when closed", () => {
        const { container } = render(
            <FolderArchiveDialog
                open={false}
                onOpenChange={vi.fn()}
                names={["nginx"]}
                onConfirm={vi.fn()}
            />,
        );
        expect(container.querySelector("[role=dialog]")).toBeNull();
    });

    it("lists every name being archived", () => {
        render(
            <FolderArchiveDialog
                open
                onOpenChange={vi.fn()}
                names={["nginx", "app-data"]}
                onConfirm={vi.fn()}
            />,
        );
        expect(screen.getByText(/nginx/)).toBeInTheDocument();
        expect(screen.getByText(/app-data/)).toBeInTheDocument();
    });

    it("offers tar.gz, tar, and zip as format choices", () => {
        render(
            <FolderArchiveDialog
                open
                onOpenChange={vi.fn()}
                names={["nginx"]}
                onConfirm={vi.fn()}
            />,
        );
        // aria-label is the bare format key — exact match avoids
        // collisions with the visible labels (e.g. "gzip-compressed").
        expect(screen.getByRole("radio", { name: "tar.gz" })).toBeInTheDocument();
        expect(screen.getByRole("radio", { name: "tar" })).toBeInTheDocument();
        expect(screen.getByRole("radio", { name: "zip" })).toBeInTheDocument();
    });

    it("defaults to tar.gz", () => {
        render(
            <FolderArchiveDialog
                open
                onOpenChange={vi.fn()}
                names={["nginx"]}
                onConfirm={vi.fn()}
            />,
        );
        expect(screen.getByRole("radio", { name: "tar.gz" })).toBeChecked();
    });

    it("calls onConfirm with the chosen format when Download is clicked", async () => {
        const onConfirm = vi.fn();
        render(
            <FolderArchiveDialog
                open
                onOpenChange={vi.fn()}
                names={["nginx"]}
                onConfirm={onConfirm}
            />,
        );
        fireEvent.click(screen.getByRole("radio", { name: "zip" }));
        fireEvent.click(screen.getByRole("button", { name: /download/i }));
        await waitFor(() => {
            expect(onConfirm).toHaveBeenCalledWith("zip");
        });
    });
});
