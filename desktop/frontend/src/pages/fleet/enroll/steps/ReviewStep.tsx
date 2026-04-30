import { Button } from "@/components/ui/button";
import { palette } from "../../../../layout/theme";
import { Step } from "../steps";

interface Props {
    summary: Array<{ label: string; value: string; editStep: Step }>;
    onEdit: (step: Step) => void;
}

export default function ReviewStep({ summary, onEdit }: Props) {
    return (
        <div className="space-y-3" data-testid="enroll-wizard-review">
            <div style={{ fontSize: 13, color: palette.textSecondary }}>
                Review configuration before generating one-shot commands.
            </div>
            <div className="space-y-2">
                {summary.map((item) => (
                    <div
                        key={item.label}
                        className="flex items-center justify-between rounded border border-border bg-surface p-2"
                    >
                        <div style={{ fontSize: 12 }}>
                            <div style={{ color: palette.textMuted }}>{item.label}</div>
                            <div style={{ color: palette.textPrimary }}>{item.value}</div>
                        </div>
                        <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => onEdit(item.editStep)}
                        >
                            Edit
                        </Button>
                    </div>
                ))}
            </div>
        </div>
    );
}
