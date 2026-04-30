import { Input } from "@/components/ui/input";
import { palette } from "../../../../layout/theme";

interface Props {
    description: string;
    onChange: (v: string) => void;
}

export default function DescriptionStep({ description, onChange }: Props) {
    return (
        <div className="space-y-3" data-testid="enroll-wizard-description">
            <div style={{ fontSize: 13, color: palette.textSecondary }}>
                Optional note shown in Audit and install artifact records.
            </div>
            <Input
                autoFocus
                placeholder="Deploy for web-01"
                value={description}
                onChange={(e) => onChange(e.target.value)}
            />
        </div>
    );
}
