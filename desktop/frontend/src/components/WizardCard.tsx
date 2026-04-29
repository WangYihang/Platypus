import { ReactNode } from "react";

import { palette, space } from "../layout/theme";

// WizardCard is the full-page centered card used by Onboarding and
// Login. The mockups keep both flows on a centered viewport with a
// hairline-bordered surface; this primitive packages that frame so
// the two pages render with identical chrome and a future
// pre-deploy splash / EULA / setup-secret rotation flow can drop
// into the same shape.
//
// Consumers render their own header + body inside `children`. The
// frame doesn't impose any internal layout — it just centers a
// fixed-width column on a dark canvas.

interface Props {
    width?: number; // default 440 px
    children: ReactNode;
}

export default function WizardCard({ width = 440, children }: Props) {
    return (
        <div
            style={{
                minHeight: "100vh",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                padding: space[6],
                background: palette.main,
                color: palette.textPrimary,
            }}
        >
            <div style={{ width, maxWidth: "100%" }}>{children}</div>
        </div>
    );
}
