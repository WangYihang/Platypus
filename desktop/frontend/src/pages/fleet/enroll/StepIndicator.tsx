import { palette, space } from "../../../layout/theme";
import { STEPS, STEP_LABEL, Step } from "./steps";

// StepIndicator renders the linear progress strip at the top of the
// wizard. The flow is 11 steps long, which doesn't fit on a single
// row in a 640px dialog — so we render a compact "step N / total —
// label" header plus a thin progress bar. Per-step data attributes
// stay on the bar so existing tests can still scope by step name.
export default function StepIndicator({ current }: { current: Step }) {
    const currentIdx = STEPS.indexOf(current);
    return (
        <div
            data-testid="enroll-wizard-steps"
            style={{
                paddingBottom: space[2],
                borderBottom: `1px solid ${palette.border}`,
            }}
        >
            <div
                style={{
                    display: "flex",
                    alignItems: "baseline",
                    gap: 6,
                    fontSize: 12,
                    color: palette.textMuted,
                    marginBottom: 6,
                }}
            >
                <span style={{ color: palette.textPrimary, fontWeight: 600 }}>
                    {STEP_LABEL[current]}
                </span>
                <span>
                    Step {currentIdx + 1} of {STEPS.length}
                </span>
            </div>
            <div
                style={{
                    display: "flex",
                    gap: 3,
                    height: 4,
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
                            title={STEP_LABEL[s]}
                            style={{
                                flex: 1,
                                borderRadius: 2,
                                background:
                                    active || done
                                        ? palette.accent
                                        : palette.border,
                                opacity: active ? 1 : done ? 0.85 : 0.5,
                            }}
                        />
                    );
                })}
            </div>
        </div>
    );
}
