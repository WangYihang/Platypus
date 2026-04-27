// computeScrollSwap is the pure brain behind HostView's per-tab
// scroll restoration. Each tab panel shares a single scroll
// container, so every tab change without help would reset scrollTop
// to 0 — operators lose their place flipping between Sessions and
// Info on a long page.
//
// Inputs:
//   · prevMap         — current saved-scroll map (tabId → scrollTop)
//   · leavingTab      — the tab the user is leaving (null on first mount)
//   · leavingScrollTop — the container's scrollTop at the moment of swap
//   · nextTab         — the tab the user is going to
//
// Returns:
//   · map        — the new saved-scroll map (immutable, never mutates input)
//   · scrollTop  — the value to apply to the shared container for the new tab
//
// Caller is responsible for:
//   1. Reading scrollTop off the container BEFORE this call
//   2. Calling this synchronously (e.g. inside useLayoutEffect)
//   3. Writing scrollTop back onto the container AFTER this call

export interface ScrollSwapResult {
    map: Map<string, number>;
    scrollTop: number;
}

export function computeScrollSwap(
    prevMap: Map<string, number>,
    leavingTab: string | null,
    leavingScrollTop: number,
    nextTab: string,
): ScrollSwapResult {
    const map = new Map(prevMap);
    if (leavingTab !== null) {
        map.set(leavingTab, leavingScrollTop);
    }
    const restored = map.get(nextTab) ?? 0;
    return { map, scrollTop: restored };
}
