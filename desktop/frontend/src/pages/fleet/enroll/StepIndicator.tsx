import { Check } from "lucide-react";

import { palette, space } from "../../../layout/theme";
import { STEPS, STEP_LABEL, Step } from "./steps";

// StepIndicator renders the linear progress strip at the top of the
// wizard. To keep dense 10-step flows readable in a narrow dialog, it
// only shows labels for the active step and its immediate neighbours.
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
                const showLabel = Math.abs(i - currentIdx) <= 1 || done;
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
                        {showLabel ? (
                            <span
                                style={{
                                    color: active
                                        ? palette.textPrimary
                                        : palette.textMuted,
                                    fontWeight: active ? 600 : 500,
                                }}
                            >
                                {STEP_LABEL[s]}
                            </span>
                        ) : null}
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
