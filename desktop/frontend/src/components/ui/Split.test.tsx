import { render, fireEvent, act } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import Split from "./Split";

beforeEach(() => {
    window.localStorage.clear();
});
afterEach(() => {
    window.localStorage.clear();
});

describe("<Split>", () => {
    it("mounts both panes with flex sizes from defaultPercent", () => {
        const { container, getByText } = render(
            <Split direction="row" defaultPercent={62}>
                <div>L</div>
                <div>R</div>
            </Split>,
        );
        expect(getByText("L")).toBeInTheDocument();
        expect(getByText("R")).toBeInTheDocument();

        const root = container.firstElementChild as HTMLElement;
        expect(root.getAttribute("data-split-direction")).toBe("row");
        const panes = Array.from(root.children).filter(
            (el) => el.getAttribute("role") !== "separator",
        );
        expect(panes).toHaveLength(2);
        // Flex shorthand uses defaultPercent / (100 - defaultPercent)
        // as the grow ratios. The seam sits between with shrink-0.
        expect((panes[0] as HTMLElement).style.flex).toBe("62 1 0%");
        expect((panes[1] as HTMLElement).style.flex).toBe("38 1 0%");
    });

    it("rehydrates the persisted percent across mounts", () => {
        window.localStorage.setItem("platypus.split.feature-x", "73");
        const { container } = render(
            <Split storageKey="feature-x" defaultPercent={50}>
                <div>L</div>
                <div>R</div>
            </Split>,
        );
        const root = container.firstElementChild as HTMLElement;
        const panes = Array.from(root.children).filter(
            (el) => el.getAttribute("role") !== "separator",
        );
        expect((panes[0] as HTMLElement).style.flex).toBe("73 1 0%");
        expect((panes[1] as HTMLElement).style.flex).toBe("27 1 0%");
    });

    it("emits a separator with the correct aria-orientation", () => {
        const row = render(
            <Split direction="row">
                <div>L</div>
                <div>R</div>
            </Split>,
        );
        const rowSep = row.container.querySelector('[role="separator"]');
        expect(rowSep?.getAttribute("aria-orientation")).toBe("vertical");
        row.unmount();

        const col = render(
            <Split direction="column">
                <div>T</div>
                <div>B</div>
            </Split>,
        );
        const colSep = col.container.querySelector('[role="separator"]');
        expect(colSep?.getAttribute("aria-orientation")).toBe("horizontal");
    });

    it("a pointer drag updates the percent and writes it to localStorage", () => {
        const { container } = render(
            <Split
                direction="row"
                defaultPercent={50}
                minPercent={10}
                maxPercent={90}
                storageKey="drag-test"
            >
                <div>L</div>
                <div>R</div>
            </Split>,
        );
        const root = container.firstElementChild as HTMLElement;
        const seam = root.querySelector<HTMLElement>('[role="separator"]')!;

        // jsdom can't run flex layout but we can fake the container's
        // bounding rect; the drag math reads getBoundingClientRect().
        const r: DOMRect = {
            x: 0, y: 0, top: 0, left: 0, right: 1000, bottom: 600,
            width: 1000, height: 600, toJSON: () => ({}),
        };
        root.getBoundingClientRect = () => r;

        // Stub setPointerCapture (jsdom doesn't implement it).
        seam.setPointerCapture = () => undefined;
        seam.releasePointerCapture = () => undefined;
        seam.hasPointerCapture = () => true;

        fireEvent.pointerDown(seam, { pointerId: 1, clientX: 500 });
        // Drag to x=300 → percent should be 30. dispatchEvent
        // doesn't auto-act in React 19, so wrap explicitly.
        act(() => {
            window.dispatchEvent(
                new PointerEvent("pointermove", { pointerId: 1, clientX: 300 }),
            );
        });
        act(() => {
            window.dispatchEvent(new PointerEvent("pointerup", { pointerId: 1 }));
        });

        const panes = Array.from(root.children).filter(
            (el) => el.getAttribute("role") !== "separator",
        );
        expect((panes[0] as HTMLElement).style.flex).toBe("30 1 0%");
        expect((panes[1] as HTMLElement).style.flex).toBe("70 1 0%");
        expect(window.localStorage.getItem("platypus.split.drag-test")).toBe("30");
    });

    it("clamps a drag past minPercent / maxPercent", () => {
        const { container } = render(
            <Split direction="row" defaultPercent={50} minPercent={20} maxPercent={80}>
                <div>L</div>
                <div>R</div>
            </Split>,
        );
        const root = container.firstElementChild as HTMLElement;
        const seam = root.querySelector<HTMLElement>('[role="separator"]')!;
        root.getBoundingClientRect = () =>
            ({ x: 0, y: 0, top: 0, left: 0, right: 1000, bottom: 600,
                width: 1000, height: 600, toJSON: () => ({}) }) as DOMRect;
        seam.setPointerCapture = () => undefined;
        seam.releasePointerCapture = () => undefined;
        seam.hasPointerCapture = () => true;

        // Drag well below the min.
        fireEvent.pointerDown(seam, { pointerId: 1, clientX: 500 });
        act(() => {
            window.dispatchEvent(
                new PointerEvent("pointermove", { pointerId: 1, clientX: 50 }),
            );
        });
        act(() => {
            window.dispatchEvent(new PointerEvent("pointerup", { pointerId: 1 }));
        });

        const panes = Array.from(root.children).filter(
            (el) => el.getAttribute("role") !== "separator",
        );
        expect((panes[0] as HTMLElement).style.flex).toBe("20 1 0%");
        expect((panes[1] as HTMLElement).style.flex).toBe("80 1 0%");
    });
});
