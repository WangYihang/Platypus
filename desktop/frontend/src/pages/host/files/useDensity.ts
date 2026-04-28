import { usePreference, type PreferenceDefs } from "../../../lib/preferences";

// useDensity is now a thin wrapper over the global `ui.density`
// preference. The Files tab used to ship its own
// `platypus:filesDensity` localStorage key with a "compact" default;
// we unified onto the project-wide preference so flipping it in
// /preferences immediately propagates to file tables, host tables,
// and any future surface that respects the same flag. The default
// stays "compact" — see DEFAULTS in lib/preferences.ts.
//
// Public API kept as a `[Density, setDensity]` tuple so every existing
// call site keeps working without an import-list change.

export type Density = PreferenceDefs["ui.density"];

export function useDensity(): [Density, (next: Density) => void] {
    const [value, setValue] = usePreference("ui.density");
    return [value, setValue];
}
