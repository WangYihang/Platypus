import { Input } from "@/components/ui/input";
import { palette } from "../../../../layout/theme";

interface Props {
    patMaxUses: number | undefined;
    onChange: (v: number | undefined) => void;
}

export default function PATMaxUsesStep({ patMaxUses, onChange }: Props) {
    return (
        <div className="space-y-3" data-testid="enroll-wizard-pat-max-uses">
            <div style={{ fontSize: 13, color: palette.textSecondary }}>
                Set max redemptions for the PAT minted from this install link.
            </div>
            <Input
                type="number"
                inputMode="numeric"
                placeholder="1"
                value={patMaxUses ?? ""}
                onChange={(e) =>
                    onChange(e.target.value === "" ? undefined : Number(e.target.value))
                }
            />
            <div style={{ fontSize: 11, color: palette.textMuted }}>
                Default is 1 redemption.
            </div>
        </div>
    );
}
