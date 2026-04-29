import { Loader2 } from "lucide-react";

import { palette } from "../../../../layout/theme";
import { Button } from "@/components/ui/button";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

import {
    QuickPreset,
    availablePresets,
    osLabel,
} from "../../../enrollment/platforms";
import { PlatformsState } from "../steps";

interface Props {
    platforms: PlatformsState;
    osList: string[];
    value: string;
    onChange: (next: string) => void;
    onPickPreset: (preset: QuickPreset) => void;
}

// OSStep is step 1 of the EnrollAgentWizard. ToggleGroup is `single`
// so leaving it deselected — by clicking the active item again or by
// never picking — yields an empty `value`, which the caller maps to
// "auto-detect at runtime" on the wire. We don't enforce a pick.
//
// On top of the manual OS picker we surface a row of "quick start"
// presets (Linux x86_64, Windows x64, macOS Apple Silicon, …) — one
// click sets both OS and arch and the wizard jumps straight to the
// Connect step. Presets are filtered against the live manifest so we
// never offer a one-click that 404s on download.
export default function OSStep({
    platforms,
    osList,
    value,
    onChange,
    onPickPreset,
}: Props) {
    const presets =
        platforms.status === "ready" ? availablePresets(platforms.platforms) : [];
    return (
        <div className="space-y-3">
            {presets.length > 0 && (
                <div className="space-y-2">
                    <div
                        style={{
                            fontSize: 12,
                            color: palette.textMuted,
                            textTransform: "uppercase",
                            letterSpacing: 0.4,
                        }}
                    >
                        Quick start
                    </div>
                    <div className="flex flex-wrap gap-2">
                        {presets.map((p) => (
                            <Button
                                key={p.id}
                                variant="outline"
                                size="sm"
                                onClick={() => onPickPreset(p)}
                                data-testid={`enroll-wizard-preset-${p.id}`}
                            >
                                {p.label}
                            </Button>
                        ))}
                    </div>
                </div>
            )}
            <div style={{ fontSize: 13, color: palette.textSecondary }}>
                Or pick the target operating system manually. Skip if you'd
                rather have the install script auto-detect at runtime.
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
