import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@wails/go/app/App", () => ({
    ReadFile: vi.fn(),
}));

// Stub react-pdf so the test never has to touch pdfjs / canvas. The mock
// renders a page-N marker instead of the real glyphs but exposes the
// same `numPages` plumbing PdfViewer relies on. Captures every `file`
// prop value so tests can assert what kind of payload PdfViewer hands
// to the worker (regression for the ArrayBuffer-detached crash).
const fileProps: unknown[] = [];

vi.mock("react-pdf", () => {
    type DocProps = {
        file: unknown;
        onLoadSuccess?: (info: { numPages: number }) => void;
        onLoadError?: (err: unknown) => void;
        children?: React.ReactNode;
    };
    type PageProps = { pageNumber: number };

    const Document = ({ file, onLoadSuccess, onLoadError, children }: DocProps) => {
        fileProps.push(file);
        Promise.resolve().then(() => {
            try {
                if (file == null) throw new Error("missing file");
                onLoadSuccess?.({ numPages: 3 });
            } catch (err) {
                onLoadError?.(err);
            }
        });
        return <div data-testid="pdf-document">{children}</div>;
    };
    const Page = ({ pageNumber }: PageProps) => (
        <div data-testid={`pdf-page-${pageNumber}`}>page-{pageNumber}</div>
    );
    const pdfjs = { GlobalWorkerOptions: { workerSrc: "" } };
    return { Document, Page, pdfjs };
});

import { ReadFile } from "@wails/go/app/App";
import PdfViewer from "./PdfViewer";

const PDF_BYTES = [37, 80, 68, 70]; // "%PDF" — payload doesn't matter, the mock ignores it.

beforeEach(() => {
    fileProps.length = 0;
    vi.spyOn(URL, "createObjectURL").mockImplementation(() => "blob:test/pdf");
    vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => {});
});

afterEach(() => {
    vi.mocked(ReadFile).mockReset();
    vi.restoreAllMocks();
});

describe("<PdfViewer>", () => {
    it("sets pdfjs.GlobalWorkerOptions.workerSrc at module load", async () => {
        // Regression for the dev-mode "Setting up fake worker failed"
        // crash: importing PdfViewer must populate workerSrc with a
        // non-empty string. Under the vitest alias the value is the
        // stub URL rather than a real asset, but the assignment must
        // still happen — that's the contract the runtime relies on.
        const { pdfjs } = await import("react-pdf");
        await import("./PdfViewer");
        expect(pdfjs.GlobalWorkerOptions.workerSrc).toBeTruthy();
        expect(typeof pdfjs.GlobalWorkerOptions.workerSrc).toBe("string");
    });

    it("loads bytes and renders the first page once load succeeds", async () => {
        vi.mocked(ReadFile).mockResolvedValueOnce(PDF_BYTES);

        render(
            <PdfViewer
                projectID="p"
                sessionHash="s"
                path="/tmp/spec.pdf"
                size={PDF_BYTES.length}
            />,
        );

        await waitFor(() => {
            expect(screen.getByTestId("pdf-page-1")).toBeInTheDocument();
        });
        expect(ReadFile).toHaveBeenCalledWith("p", "s", "/tmp/spec.pdf", 0, 0);
    });

    it("paginates with next/prev controls bounded to numPages", async () => {
        vi.mocked(ReadFile).mockResolvedValueOnce(PDF_BYTES);

        render(
            <PdfViewer
                projectID="p"
                sessionHash="s"
                path="/tmp/spec.pdf"
                size={PDF_BYTES.length}
            />,
        );

        await waitFor(() => {
            // Wait for "Page 1 of 3" status to be visible.
            expect(screen.getByText(/page 1 of 3/i)).toBeInTheDocument();
        });

        fireEvent.click(screen.getByRole("button", { name: /next/i }));
        expect(screen.getByText(/page 2 of 3/i)).toBeInTheDocument();
        expect(screen.getByTestId("pdf-page-2")).toBeInTheDocument();

        fireEvent.click(screen.getByRole("button", { name: /next/i }));
        expect(screen.getByText(/page 3 of 3/i)).toBeInTheDocument();

        // Clamp at the upper bound.
        fireEvent.click(screen.getByRole("button", { name: /next/i }));
        expect(screen.getByText(/page 3 of 3/i)).toBeInTheDocument();

        fireEvent.click(screen.getByRole("button", { name: /prev/i }));
        expect(screen.getByText(/page 2 of 3/i)).toBeInTheDocument();
    });

    it("hands <Document> a Blob URL string, not a raw Uint8Array", async () => {
        // Regression: passing { data: Uint8Array } made pdfjs transfer
        // the underlying ArrayBuffer to its worker; the second render
        // then crashed with "ArrayBuffer is already detached". The
        // contract is now a stable Blob URL string — re-renders reuse
        // the same URL, no transferables involved.
        vi.mocked(ReadFile).mockResolvedValueOnce(PDF_BYTES);

        render(
            <PdfViewer
                projectID="p"
                sessionHash="s"
                path="/tmp/spec.pdf"
                size={PDF_BYTES.length}
            />,
        );

        await waitFor(() => {
            expect(screen.getByTestId("pdf-page-1")).toBeInTheDocument();
        });

        const seen = fileProps.filter((f) => f != null);
        expect(seen.length).toBeGreaterThan(0);
        for (const f of seen) {
            expect(typeof f).toBe("string");
            expect(f as string).toMatch(/^blob:/);
        }
    });

    it("surfaces a load error when ReadFile rejects", async () => {
        vi.mocked(ReadFile).mockRejectedValueOnce(new Error("nope"));

        render(
            <PdfViewer
                projectID="p"
                sessionHash="s"
                path="/tmp/spec.pdf"
                size={4}
            />,
        );

        await waitFor(() => {
            expect(screen.getByText(/nope/i)).toBeInTheDocument();
        });
    });
});
