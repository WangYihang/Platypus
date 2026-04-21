import { palette, radius } from "../layout/theme";

interface Props {
    size?: number;
}

// Brand is the small white-on-black "P" square used as Platypus's mark
// at the top of the sidebar. Squared off — distinguishes it from a user
// avatar (which is a circle).
export default function Brand({ size = 28 }: Props) {
    return (
        <div
            aria-label="Platypus"
            style={{
                width: size,
                height: size,
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                background: palette.textPrimary,
                color: palette.accentFg,
                borderRadius: radius.md,
                fontWeight: 700,
                fontSize: Math.round(size * 0.5),
                letterSpacing: -0.5,
                lineHeight: 1,
                flexShrink: 0,
            }}
        >
            P
        </div>
    );
}
