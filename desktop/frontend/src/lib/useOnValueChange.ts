import { useState } from "react";

/**
 * Runs `onChange` during the render in which `value` first differs from
 * the previous render — React's documented way to adjust state when a
 * prop changes, without an effect.
 *
 * The alternative this replaces was everywhere in the dialogs:
 *
 *     useEffect(() => {
 *         if (open) setName("");
 *     }, [open]);
 *
 * which paints the stale value first and then immediately re-renders
 * with the reset one. React re-runs the component for a state update
 * made during render before committing anything, so nothing reaches
 * the screen in between. It is also honest about what is happening:
 * this is derived-state maintenance, not a synchronisation with
 * something outside React, which is what an effect is for.
 *
 * The state that drives this lives here rather than in the caller, so
 * a component keeps one line instead of a store-the-previous-value
 * dance repeated per field.
 *
 * Only call the setters of state this component owns from `onChange`.
 * Anything else — a fetch, a subscription, a parent's callback —
 * belongs in an effect, and calling it here would fire it during a
 * render React may still discard.
 */
export function useOnValueChange<T>(value: T, onChange: (next: T, prev: T) => void): void {
    const [prev, setPrev] = useState(value);
    if (!Object.is(value, prev)) {
        setPrev(value);
        onChange(value, prev);
    }
}

/**
 * Convenience over useOnValueChange for the common dialog case: run
 * `reset` each time `open` goes false → true, so every opening starts
 * from a clean form.
 */
export function useResetOnOpen(open: boolean, reset: () => void): void {
    useOnValueChange(open, (next) => {
        if (next) reset();
    });
}
