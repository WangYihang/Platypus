import { ReactNode } from "react";

import { Label } from "@/components/ui/label";

import { palette, space } from "../layout/theme";

// SettingRow is the canonical "label + description on the left,
// control on the right" row used by every settings surface
// (Preferences, Account → Identity, AdminSettings, the future
// onboarding fine-grained step). The shape is pinned here so a
// future density tweak (label size, gap) lands in one component.
//
// The row draws a hairline bottom border so a stack of rows reads
// as a list without an explicit `<ul>`. Pass `borderless` when the
// last row in a section shouldn't draw the divider.
interface Props {
    label: ReactNode;
    description?: ReactNode;
    children: ReactNode;
    htmlFor?: string;
    borderless?: boolean;
}

export default function SettingRow({
    label,
    description,
    children,
    htmlFor,
    borderless = false,
}: Props) {
    return (
        <div
            style={{
                display: "flex",
                alignItems: "flex-start",
                justifyContent: "space-between",
                gap: space[4],
                padding: `${space[3]}px 0`,
                borderBottom: borderless
                    ? undefined
                    : `1px solid ${palette.border}`,
            }}
        >
            <div style={{ flex: 1, minWidth: 0 }}>
                <Label
                    htmlFor={htmlFor}
                    style={{
                        display: "block",
                        marginBottom: 4,
                        color: palette.textPrimary,
                        fontSize: 13,
                        fontWeight: 500,
                    }}
                >
                    {label}
                </Label>
                {description && (
                    <p
                        style={{
                            fontSize: 12,
                            color: palette.textMuted,
                            lineHeight: 1.5,
                            margin: 0,
                        }}
                    >
                        {description}
                    </p>
                )}
            </div>
            <div style={{ flexShrink: 0 }}>{children}</div>
        </div>
    );
}
