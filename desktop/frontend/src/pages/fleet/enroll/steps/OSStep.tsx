import { Loader2 } from "lucide-react";

import { palette } from "../../../../layout/theme";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

import { osLabel } from "../../../enrollment/platforms";
import { PlatformsState } from "../steps";

interface Props {
    platforms: PlatformsState;
    osList: string[];
    value: string;
    onChange: (next: string) => void;
}

// OSStep is the manual OS picker. ToggleGroup is `single` so leaving
// it deselected — by clicking the active item again or by never
// picking — yields an empty `value`, which the caller maps to
// "auto-detect at runtime" on the wire. We don't enforce a pick.
//
// One-click presets (Linux x86_64, Windows x64, macOS Apple Silicon)
// used to live here as a "Quick start" row sourced from a static
// table; they now live in the wizard's first step (PickPresetStep)
// as persisted, project-scoped presets the operator owns. This step
// stays focused on the manual override.
export default function OSStep({
    platforms,
    osList,
    value,
    onChange,
}: Props) {
    return (
        <div className="space-y-3">
            <div style={{ fontSize: 13, color: palette.textSecondary }}>
                Pick the target operating system, or skip to let the install
                script auto-detect at runtime.
            </div>
            <ToggleGroup
                type="single"
                variant="outline"
                size="sm"
                value={value}
                onValueChange={onChange}
                disabled={platforms.status !== "ready"}
                className="flex-wrap justify-start"
                data-testid="enroll-wizard-os"
            >
                {osList.map((os) => (
                    <ToggleGroupItem key={os} value={os}>
                        {osLabel(os)}
                    </ToggleGroupItem>
                ))}
            </ToggleGroup>
            <PlatformsHint platforms={platforms} />
        </div>
    );
}

// PlatformsHint surfaces the four loading / ready / empty / error
// states the manifest fetch can land in. Lives next to OSStep
// because that's the only step that reads platform state directly.
function PlatformsHint({ platforms }: { platforms: PlatformsState }) {
    let body: React.ReactNode;
    if (platforms.status === "loading") {
        body = (
            <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
                <Loader2 className="size-3 animate-spin" /> Loading platforms…
            </span>
        );
    } else if (platforms.status === "empty") {
        body = `No agent binaries on channel "${platforms.channel}" yet — the install command still works (auto-detect at runtime).`;
    } else if (platforms.status === "error") {
        body = `Couldn't load platforms: ${platforms.message}. The install command still works (auto-detect at runtime).`;
    } else {
        body =
            "Leave unselected for the install one-liner to auto-detect the target's OS, or pick one to start narrowing.";
    }
    return <div style={{ fontSize: 11, color: palette.textMuted }}>{body}</div>;
}
