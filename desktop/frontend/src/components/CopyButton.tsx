import { Copy } from "lucide-react";
import { toast } from "sonner";

import { humanizeError } from "../lib/humanizeError";
import { Button } from "@/components/ui/button";

interface Props {
    text: string;
    label?: string;
    successMessage?: string;
    className?: string;
    size?: "sm" | "default";
    variant?: "outline" | "default" | "ghost";
}

// CopyButton wraps navigator.clipboard.writeText with a labelled
// affordance and a success toast. Use anywhere we surface a long
// command, secret, or URL the operator is going to paste somewhere.
// Failures (rare — usually a permissions issue in a non-secure
// context) surface as toasts too so the user isn't left guessing.
export default function CopyButton({
    text,
    label = "Copy",
    successMessage = "Copied to clipboard",
    className,
    size = "sm",
    variant = "outline",
}: Props) {
    async function onClick() {
        try {
            await navigator.clipboard.writeText(text);
            toast.success(successMessage);
        } catch (e) {
            toast.error(`Couldn't copy: ${humanizeError(e)}`);
        }
    }

    return (
        <Button
            type="button"
            size={size}
            variant={variant}
            onClick={onClick}
            className={className}
        >
            <Copy className="size-3.5" />
            {label}
        </Button>
    );
}
