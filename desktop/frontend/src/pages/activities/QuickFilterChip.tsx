import { palette, radius, space } from "../../layout/theme";

interface Props {
    label: string;
    title?: string;
    onClick: () => void;
    disabled?: boolean;
    variant?: "outline" | "ghost";
}

// Pill-styled button local to ActivitiesPage; uses StatusPill-like
// styling so the chip row reads as a coherent group instead of generic
// stacked Buttons.
export default function QuickFilterChip({
    label,
    title,
    onClick,
    disabled,
    variant = "outline",
}: Props) {
    const ghost = variant === "ghost";
    return (
        <button
            type="button"
            onClick={onClick}
            disabled={disabled}
            title={title}
            style={{
                display: "inline-flex",
                alignItems: "center",
                padding: `2px ${space[3]}px`,
                fontSize: 12,
                fontWeight: 500,
                lineHeight: 1.6,
                color: ghost ? palette.textMuted : palette.textSecondary,
                border: ghost ? "1px solid transparent" : `1px solid ${palette.border}`,
                background: "transparent",
                borderRadius: radius.pill,
                cursor: disabled ? "not-allowed" : "pointer",
                opacity: disabled ? 0.5 : 1,
                whiteSpace: "nowrap",
            }}
        >
            {label}
        </button>
    );
}
