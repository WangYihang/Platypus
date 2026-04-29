import Mono from "../../../../components/Mono";
import { palette, space } from "../../../../layout/theme";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";

import { archLabel, osLabel } from "../../../enrollment/platforms";

interface Props {
    os: string;
    archList: string[];
    value: string;
    onChange: (next: string) => void;
}

// ArchStep is step 2 of the EnrollAgentWizard. When OS was skipped,
// there's nothing to filter the arch list by — so we render an
// explanatory note instead of an empty toggle row, which would read
// as "broken UI". The operator can either go back and pick an OS or
// move on (both fields stay empty, install script auto-detects).
export default function ArchStep({ os, archList, value, onChange }: Props) {
    if (!os) {
        return (
            <div
                style={{
                    fontSize: 13,
                    color: palette.textSecondary,
                    padding: `${space[3]}px 0`,
                }}
            >
                No OS selected — the install script will auto-detect both OS and
                architecture at runtime. You can move on, or go back to pick an OS
                first.
            </div>
        );
    }
    return (
        <div className="space-y-3">
            <div style={{ fontSize: 13, color: palette.textSecondary }}>
                Pick an architecture for <Mono>{osLabel(os)}</Mono>. Skipping
                leaves the arch unbound; the install script auto-detects.
            </div>
            <ToggleGroup
                type="single"
                variant="outline"
                size="sm"
                value={value}
                onValueChange={onChange}
                className="flex-wrap justify-start"
                data-testid="enroll-wizard-arch"
            >
                {archList.map((a) => (
                    <ToggleGroupItem key={a} value={a}>
                        {archLabel(a)}
                    </ToggleGroupItem>
                ))}
            </ToggleGroup>
        </div>
    );
}
