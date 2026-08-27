import { describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";
import { useState } from "react";

import { useOnValueChange, useResetOnOpen } from "./useOnValueChange";

describe("useOnValueChange", () => {
    it("does not fire on the first render", () => {
        const onChange = vi.fn();
        renderHook(({ v }) => useOnValueChange(v, onChange), {
            initialProps: { v: 1 },
        });
        expect(onChange).not.toHaveBeenCalled();
    });

    it("fires once per distinct value, with the previous one", () => {
        const onChange = vi.fn();
        const { rerender } = renderHook(({ v }) => useOnValueChange(v, onChange), {
            initialProps: { v: 1 },
        });
        rerender({ v: 2 });
        expect(onChange).toHaveBeenCalledTimes(1);
        expect(onChange).toHaveBeenLastCalledWith(2, 1);

        // A re-render with the same value must not fire again — that is
        // the whole difference from putting this in the render body.
        rerender({ v: 2 });
        expect(onChange).toHaveBeenCalledTimes(1);

        rerender({ v: 3 });
        expect(onChange).toHaveBeenCalledTimes(2);
        expect(onChange).toHaveBeenLastCalledWith(3, 2);
    });

    it("treats NaN as unchanged, like Object.is", () => {
        const onChange = vi.fn();
        const { rerender } = renderHook(({ v }) => useOnValueChange(v, onChange), {
            initialProps: { v: NaN },
        });
        rerender({ v: NaN });
        expect(onChange).not.toHaveBeenCalled();
    });

    // The reason this exists: the reset must be visible in the same
    // paint, not one render later.
    it("applies a state reset before anything is committed", () => {
        const seen: string[] = [];
        const { result, rerender } = renderHook(
            ({ open }) => {
                const [name, setName] = useState("typed");
                useResetOnOpen(open, () => setName(""));
                seen.push(name);
                return name;
            },
            { initialProps: { open: false } },
        );
        expect(result.current).toBe("typed");

        rerender({ open: true });
        expect(result.current).toBe("");
        // "typed" was rendered before the reopen, and the render that
        // saw open=true re-ran with "" — it is never committed as
        // "typed" while open.
        expect(seen[seen.length - 1]).toBe("");
    });
});

describe("useResetOnOpen", () => {
    it("fires on false → true and not on true → false", () => {
        const reset = vi.fn();
        const { rerender } = renderHook(({ open }) => useResetOnOpen(open, reset), {
            initialProps: { open: false },
        });
        rerender({ open: true });
        expect(reset).toHaveBeenCalledTimes(1);
        rerender({ open: false });
        expect(reset).toHaveBeenCalledTimes(1);
        rerender({ open: true });
        expect(reset).toHaveBeenCalledTimes(2);
    });

    it("does not fire when it starts open", () => {
        const reset = vi.fn();
        renderHook(({ open }) => useResetOnOpen(open, reset), {
            initialProps: { open: true },
        });
        expect(reset).not.toHaveBeenCalled();
    });
});
