import { Loader2 } from "lucide-react";

import { palette, space } from "../../../layout/theme";
import { Button } from "@/components/ui/button";

import { Step } from "./steps";

interface Props {
    step: Step;
    submitting: boolean;
    canSubmitConnect: boolean;
    onBack: () => void;
    onNext: () => void;
    onSubmit: () => void;
    onCancel: () => void;
    onFinish: () => void;
}

// WizardFooter renders the per-step action row at the bottom of the
// EnrollAgentWizard. The shape varies enough between the linear
// steps (back / next / cancel) and the terminal "run" step (single
// "Done" button) that branching here is clearer than papering over
// the difference with a uniform layout.
export default function WizardFooter({
    step,
    submitting,
    canSubmitConnect,
    onBack,
    onNext,
    onSubmit,
    onCancel,
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
                disabled={step === "os"}
            >
                Back
            </Button>
            <div style={{ display: "flex", gap: space[2] }}>
                <Button type="button" variant="outline" size="sm" onClick={onCancel}>
                    Cancel
                </Button>
                {step === "connect" ? (
                    <Button
                        type="button"
                        size="sm"
                        disabled={!canSubmitConnect || submitting}
                        onClick={onSubmit}
                        data-testid="enroll-wizard-submit"
                    >
                        {submitting && <Loader2 className="size-3.5 animate-spin" />}
                        Generate
                    </Button>
                ) : (
                    <Button
                        type="button"
                        size="sm"
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
