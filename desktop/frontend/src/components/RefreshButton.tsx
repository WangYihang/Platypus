import { Loader2, RotateCw } from "lucide-react";
import { ComponentProps } from "react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/cn";

interface Props extends Omit<ComponentProps<typeof Button>, "size" | "variant" | "children"> {
    loading?: boolean;
    label?: string;
    size?: ComponentProps<typeof Button>["size"];
    variant?: ComponentProps<typeof Button>["variant"];
    iconOnly?: boolean;
}

// RefreshButton is the canonical Loader2-while-loading / RotateCw-otherwise
// button used on every list / detail page. Before this primitive, twelve
// call sites repeated the same `{loading ? <Loader2 /> : <RotateCw />}`
// ternary inside a Button shell — see `git log --grep "RefreshButton"`
// for the swap-in commit. Keep the icon size / variant defaults aligned
// with what those call sites used so the visual diff after migration is
// nil; deviating call sites can override via the standard Button props.
export default function RefreshButton({
    loading = false,
    onClick,
    label = "Refresh",
    size = "sm",
    variant = "outline",
    iconOnly = false,
    disabled,
    className,
    ...rest
}: Props) {
    const Icon = loading ? Loader2 : RotateCw;
    return (
        <Button
            type="button"
            size={iconOnly ? (size === "sm" ? "icon-sm" : "icon") : size}
            variant={variant}
            disabled={disabled || loading}
            onClick={onClick}
            aria-label={iconOnly ? (rest["aria-label"] ?? label) : rest["aria-label"]}
            title={rest.title ?? label}
            className={cn(className)}
            {...rest}
        >
            <Icon className={cn("size-3.5", loading && "animate-spin")} />
            {!iconOnly && label}
        </Button>
    );
}
