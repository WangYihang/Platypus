import { CheckCircle2 } from "lucide-react";

import { palette, radius, space } from "../../layout/theme";
import { normaliseURL } from "../../lib/servers";
import { Label } from "@/components/ui/label";

// Shared bits used by multiple onboarding steps.

export function Field({
    label,
    helper,
    children,
}: {
    label: string;
    // Optional guidance below the input. Used by the bootstrap-secret
    // field to point operators at <data-dir>/bootstrap.secret since the
    // secret no longer appears in stdout (security audit M2).
    helper?: React.ReactNode;
    children: React.ReactNode;
}) {
    return (
        <div>
            <Label
                style={{
                    fontSize: 12,
                    color: palette.textSecondary,
                    marginBottom: 4,
                    display: "inline-block",
                }}
            >
                {label}
            </Label>
            {children}
            {helper && (
                <div
                    style={{
                        marginTop: 6,
                        fontSize: 12,
                        color: palette.textMuted,
                        lineHeight: 1.5,
                    }}
                >
                    {helper}
                </div>
            )}
        </div>
    );
}

export function ConfirmedBanner({ url, hint }: { url: string; hint: string }) {
    return (
        <div
            style={{
                display: "flex",
                alignItems: "center",
                gap: space[2],
                padding: space[3],
                background: palette.surface,
                border: `1px solid ${palette.border}`,
                borderRadius: radius.md,
                color: palette.textSecondary,
                fontSize: 12,
            }}
        >
            <CheckCircle2 className="size-4" style={{ color: palette.info }} />
            <div style={{ flex: 1, minWidth: 0 }}>
                <div
                    style={{
                        fontFamily: "var(--font-geist-mono)",
                        color: palette.textPrimary,
                        whiteSpace: "nowrap",
                        overflow: "hidden",
                        textOverflow: "ellipsis",
                    }}
                >
                    {normaliseURL(url)}
                </div>
                <div>{hint}</div>
            </div>
        </div>
    );
}

export function ProgressPill({ step, total }: { step: number; total: number }) {
    return (
        <div
            style={{
                display: "flex",
                alignItems: "center",
                gap: space[2],
                justifyContent: "center",
                marginBottom: space[4],
                color: palette.textMuted,
                fontSize: 12,
                fontFamily: "var(--font-geist-mono)",
            }}
        >
            <span>
                Step {step} / {total}
            </span>
            <div style={{ display: "flex", gap: 4 }}>
                {Array.from({ length: total }).map((_, i) => (
                    <span
                        key={i}
                        style={{
                            width: 24,
                            height: 3,
                            borderRadius: 2,
                            background: i < step ? palette.textPrimary : palette.border,
                        }}
                    />
                ))}
            </div>
        </div>
    );
}
