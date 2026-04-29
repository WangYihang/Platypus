import { UseFormReturn } from "react-hook-form";

import {
    FormDescription,
    FormItem,
    FormLabel,
    FormMessage,
} from "@/components/ui/form";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

import {
    ARCH_ORDER,
    OS_ORDER,
    PlatformsState,
    preferredOrder,
} from "./platforms";
import { InstallFormValues } from "./schemas";

interface Props {
    form: UseFormReturn<InstallFormValues>;
    platforms: PlatformsState;
}

// PlatformPickerField is the "Target platform" form control on
// IssueInstallDialog. Two cascading ToggleGroups: pick an OS, then
// the matching archs unfold; leaving both unselected means
// "auto-detect at runtime", which preserves the install one-liner's
// "blank target_os + blank target_arch → self-detect" wire contract.
//
// ToggleGroup is used over Radix Select because Select forbids the
// empty-string value, and we need empty as a first-class option.
//
// Note: the four-step EnrollAgentWizard intentionally does NOT reuse
// this component — it splits OS and arch onto separate steps so each
// pick gets its own visual focus. The two flows share the
// `PlatformsState` enum and the `OS_ORDER` / `ARCH_ORDER` priority
// tables instead.
export default function PlatformPickerField({ form, platforms }: Props) {
    const targetOS = form.watch("target_os") ?? "";
    const targetArch = form.watch("target_arch") ?? "";

    // Index live platforms by OS so the arch row only shows architectures
    // that actually have a published binary on the selected OS.
    const archsByOS = new Map<string, string[]>();
    if (platforms.status === "ready") {
        const tmp = new Map<string, Set<string>>();
        for (const p of platforms.platforms) {
            if (!tmp.has(p.os)) tmp.set(p.os, new Set());
            tmp.get(p.os)!.add(p.arch);
        }
        for (const [os, set] of tmp) {
            archsByOS.set(os, [...set].sort(preferredOrder(ARCH_ORDER)));
        }
    }
    const osList = [...archsByOS.keys()].sort(preferredOrder(OS_ORDER));
    const archList = targetOS ? (archsByOS.get(targetOS) ?? []) : [];

    function pickOS(next: string) {
        // Radix emits "" when the user deselects the active item by
        // clicking it again — let that propagate as "back to
        // auto-detect" rather than an "OS but no arch" half state.
        form.setValue("target_os", next);
        // Switching OS invalidates whatever arch was picked — even if
        // the new OS happens to have the same arch name, forcing a
        // re-pick keeps the cascade honest.
        form.setValue("target_arch", "");
    }

    function pickArch(next: string) {
        form.setValue("target_arch", next);
    }

    const description = describePlatformsState(platforms, targetOS, targetArch);

    return (
        <FormItem>
            <FormLabel>Target platform</FormLabel>
            <div className="space-y-3">
                <ToggleGroup
                    type="single"
                    variant="outline"
                    size="sm"
                    value={targetOS}
                    onValueChange={pickOS}
                    disabled={platforms.status !== "ready"}
                    className="flex-wrap justify-start"
                >
                    {osList.map((os) => (
                        <ToggleGroupItem key={os} value={os}>
                            {os}
                        </ToggleGroupItem>
                    ))}
                </ToggleGroup>
                {targetOS && archList.length > 0 && (
                    <ToggleGroup
                        type="single"
                        variant="outline"
                        size="sm"
                        value={targetArch}
                        onValueChange={pickArch}
                        className="flex-wrap justify-start"
                    >
                        {archList.map((arch) => (
                            <ToggleGroupItem key={arch} value={arch}>
                                {arch}
                            </ToggleGroupItem>
                        ))}
                    </ToggleGroup>
                )}
            </div>
            <FormDescription>{description}</FormDescription>
            <FormMessage />
        </FormItem>
    );
}

function describePlatformsState(
    platforms: PlatformsState,
    targetOS: string,
    targetArch: string,
): string {
    if (platforms.status === "loading") return "Loading platforms…";
    if (platforms.status === "empty") {
        return `No agent binaries on channel "${platforms.channel}" yet — run the agent-publisher sidecar (or seed MinIO) to populate this picker. The install command still works (auto-detect at runtime).`;
    }
    if (platforms.status === "error") {
        return `Couldn't load platforms: ${platforms.message}. The install command still works (auto-detect at runtime).`;
    }
    if (targetOS && targetArch) return `Pinned to ${targetOS}/${targetArch}.`;
    if (targetOS) {
        return "Pick an architecture, or leave the OS unselected to auto-detect at runtime.";
    }
    return "Leave both unselected for the install one-liner to auto-detect, or pick an OS to start narrowing.";
}
