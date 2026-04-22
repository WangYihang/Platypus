import {
    CircleCheckIcon,
    InfoIcon,
    Loader2Icon,
    OctagonXIcon,
    TriangleAlertIcon,
} from "lucide-react";
import { Toaster as Sonner, type ToasterProps } from "sonner";

// Dark-only Toaster. The stock shadcn generator uses next-themes; our
// app is locked to dark mode (color-scheme: dark on :root), so we skip
// that dependency and pin theme="dark" directly. Tokens still come
// through the CSS vars set in style.css.
const Toaster = ({ ...props }: ToasterProps) => {
    return (
        <Sonner
            theme="dark"
            className="toaster group"
            icons={{
                success: <CircleCheckIcon className="size-4" />,
                info: <InfoIcon className="size-4" />,
                warning: <TriangleAlertIcon className="size-4" />,
                error: <OctagonXIcon className="size-4" />,
                loading: <Loader2Icon className="size-4 animate-spin" />,
            }}
            style={
                {
                    "--normal-bg": "var(--popover)",
                    "--normal-text": "var(--popover-foreground)",
                    "--normal-border": "var(--border)",
                    "--border-radius": "var(--radius)",
                } as React.CSSProperties
            }
            {...props}
        />
    );
};

export { Toaster };
