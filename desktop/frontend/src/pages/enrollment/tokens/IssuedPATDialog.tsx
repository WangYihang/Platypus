import { Copy } from "lucide-react";
import { toast } from "sonner";

import { IssueEnrollmentTokenResponse } from "../../../lib/api";
import { Button } from "@/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";

interface Props {
    result: IssueEnrollmentTokenResponse | null;
    onClose: () => void;
}

// IssuedPATDialog renders the freshly-issued enrollment token in a
// single copyable block. Plain-text exposure is one-shot — once the
// dialog closes, the server discards the secret and there's no way
// to retrieve it. The Copy button is the path most operators take.
export default function IssuedPATDialog({ result, onClose }: Props) {
    async function copy() {
        if (!result) return;
        await navigator.clipboard.writeText(result.token);
        toast.success("Copied to clipboard");
    }

    return (
        <Dialog open={result !== null} onOpenChange={(o) => !o && onClose()}>
            <DialogContent className="sm:max-w-[640px]">
                <DialogHeader>
                    <DialogTitle>Enrollment token issued</DialogTitle>
                    <DialogDescription>
                        This is the only time the token is shown. Copy it now — the
                        server cannot show it again.
                    </DialogDescription>
                </DialogHeader>
                <pre className="rounded border border-border bg-surface p-3 font-mono text-xs break-all whitespace-pre-wrap">
                    {result?.token}
                </pre>
                <DialogFooter>
                    <Button variant="outline" onClick={copy}>
                        <Copy className="size-3.5" />
                        Copy token
                    </Button>
                    <Button onClick={onClose}>Done</Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}
