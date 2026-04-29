import { Check } from "lucide-react";

import { palette, space } from "../../../layout/theme";
import { STEPS, STEP_LABEL, Step } from "./steps";

// StepIndicator renders the four-pill progress strip at the top of
// the wizard (OS → Arch → Connect → Run). Pills behind `current` are
// rendered as "done" (filled circle with a check), `current` is
// "active" (filled circle with the step number), the rest are
// "pending" (hairline circle). Pure presentation — no state of its
// own.
export default function StepIndicator({ current }: { current: Step }) {
    const currentIdx = STEPS.indexOf(current);
    return (
        <div
            data-testid="enroll-wizard-steps"
            style={{
                display: "flex",
                alignItems: "center",
                gap: space[2],
                fontSize: 11,
                color: palette.textMuted,
                paddingBottom: space[2],
                borderBottom: `1px solid ${palette.border}`,
            }}
        >
            {STEPS.map((s, i) => {
                const done = i < currentIdx;
                const active = i === currentIdx;
                return (
                    <span
                        key={s}
                        data-step={s}
                        data-active={active ? "true" : "false"}
                        data-done={done ? "true" : "false"}
                        style={{
                            display: "inline-flex",
                            alignItems: "center",
                            gap: 6,
                        }}
                    >
                        <span
                            style={{
                                display: "inline-flex",
                                alignItems: "center",
                                justifyContent: "center",
                                width: 18,
                                height: 18,
                                borderRadius: 999,
                                fontSize: 10,
                                fontWeight: 600,
                                color:
                                    active || done
                                        ? palette.accentFg
                                        : palette.textMuted,
                                background:
                                    active || done ? palette.accent : "transparent",
                                border: `1px solid ${
                                    active || done ? palette.accent : palette.border
                                }`,
                            }}
                        >
                            {done ? <Check className="size-3" /> : i + 1}
                        </span>
                        <span
                            style={{
                                color: active ? palette.textPrimary : palette.textMuted,
                                fontWeight: active ? 600 : 500,
                            }}
                        >
                            {STEP_LABEL[s]}
                        </span>
                        {i < STEPS.length - 1 && (
                            <span style={{ color: palette.border, marginLeft: 2 }}>
                                →
                            </span>
                        )}
                    </span>
                );
            })}
        </div>
    );
}
