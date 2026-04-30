import { Input } from "@/components/ui/input";
import { palette } from "../../../../layout/theme";
import { formatSeconds } from "../../../../lib/time";

interface Props {
    ttlSeconds: number | undefined;
    onChange: (v: number | undefined) => void;
}

export default function TTLStep({ ttlSeconds, onChange }: Props) {
    return (
        <div className="space-y-3" data-testid="enroll-wizard-ttl">
            <div style={{ fontSize: 13, color: palette.textSecondary }}>
                Set how long this one-shot install URL remains valid.
            </div>
            <Input
                type="number"
                inputMode="numeric"
                placeholder="300 (= 5m)"
                value={ttlSeconds ?? ""}
                onChange={(e) =>
                    onChange(e.target.value === "" ? undefined : Number(e.target.value))
                }
            />
            <div style={{ fontSize: 11, color: palette.textMuted }}>
                {typeof ttlSeconds === "number" && ttlSeconds > 0
                    ? `Current: ${formatSeconds(ttlSeconds)}`
                    : "Default 300 seconds (5 minutes)."}
            </div>
        </div>
    );
}
