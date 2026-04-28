import { usePreference, type PreferenceDefs } from "../../../lib/preferences";

// useViewMode is now a thin wrapper over the `ui.files.viewMode`
// preference. The previous `platypus:filesViewMode` localStorage key
// was retired when we unified onto the typed preference registry.

export type ViewMode = PreferenceDefs["ui.files.viewMode"];

export function useViewMode(): [ViewMode, (next: ViewMode) => void] {
    const [value, setValue] = usePreference("ui.files.viewMode");
    return [value, setValue];
}
