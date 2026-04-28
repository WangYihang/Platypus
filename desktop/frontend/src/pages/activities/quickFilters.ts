import type { ActivityOutcome, ActivitySource } from "../../lib/api";

export type ActivitiesTimeRange = "24h" | "7d" | "30d" | "all";

// QuickFilterPreset is the discriminator for the chip row above the
// Activities toolbar. Each preset maps to a partial filter patch the
// page merges with its existing state — so the chips are read-only,
// stateless affordances on top of the same control surface the
// toolbar already exposes.
export type QuickFilterPreset = "my" | "failures" | "24h" | "clear";

export interface QuickFilterPatch {
    actor?: string;
    outcome?: ActivityOutcome | "";
    query?: string;
    range?: ActivitiesTimeRange;
    categories?: string[];
    sources?: ActivitySource[];
}

export interface QuickFilterContext {
    username: string;
}

export function applyQuickFilter(
    preset: QuickFilterPreset,
    ctx: QuickFilterContext,
): QuickFilterPatch {
    switch (preset) {
        case "my":
            return { actor: ctx.username, range: "24h" };
        case "failures":
            return { outcome: "error" };
        case "24h":
            return { range: "24h" };
        case "clear":
            return {
                actor: "",
                outcome: "",
                query: "",
                range: "7d",
                categories: [],
                sources: [],
            };
    }
}
