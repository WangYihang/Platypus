import { Loader2 } from "lucide-react";

import { palette, space } from "../../../layout/theme";
import { Button } from "@/components/ui/button";

import { Step } from "./steps";

interface Props {
    step: Step;
    submitting: boolean;
    canNext: boolean;
    canGenerate: boolean;
    isFirst: boolean;
    onBack: () => void;
    onNext: () => void;
    onGenerate: () => void;
    onFinish: () => void;
}

// WizardFooter renders the per-step action row at the bottom of the
// EnrollAgentWizard. The dialog's own X button already handles
// dismissal, so the footer carries only the navigation actions.
export default function WizardFooter({
    step,
    submitting,
    canNext,
    canGenerate,
    isFirst,
    onBack,
    onNext,
    onGenerate,
    onFinish,
}: Props) {
    if (step === "run") {
        return (
            <div
                style={{
                    display: "flex",
                    justifyContent: "flex-end",
                    paddingTop: space[2],
                    borderTop: `1px solid ${palette.border}`,
                }}
            >
                <Button onClick={onFinish} data-testid="enroll-wizard-finish">
                    Done — show me Fleet
                </Button>
            </div>
        );
    }
    return (
        <div
            style={{
                display: "flex",
                justifyContent: "space-between",
                alignItems: "center",
                paddingTop: space[2],
                borderTop: `1px solid ${palette.border}`,
            }}
        >
            <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={onBack}
                disabled={isFirst}
            >
                Back
            </Button>
            <div style={{ display: "flex", gap: space[2] }}>
                {step === "review" ? (
                    <Button
                        type="button"
                        size="sm"
                        disabled={!canGenerate || submitting}
                        onClick={onGenerate}
                        data-testid="enroll-wizard-generate"
                    >
                        {submitting && <Loader2 className="size-3.5 animate-spin" />}
                        Generate
                    </Button>
                ) : (
                    <Button
                        type="button"
                        size="sm"
                        disabled={!canNext}
                        onClick={onNext}
                        data-testid="enroll-wizard-next"
                    >
                        Next
                    </Button>
                )}
            </div>
        </div>
    );
}
