import { Copy } from "lucide-react";
import { toast } from "sonner";

import DataList from "../../components/DataList";
import Mono from "../../components/Mono";
import { palette, space } from "../../layout/theme";
import { type IssueAccountPATResponse } from "../../lib/api";
import { fromNow } from "../../lib/time";
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
    result: IssueAccountPATResponse | null;
    onClose: () => void;
}

export default function IssuedAccountPATDialog({ result, onClose }: Props) {
    async function copy() {
        if (!result) return;
        try {
            await navigator.clipboard.writeText(result.token);
            toast.success("Token copied");
        } catch {
            toast.error("Copy failed — select manually");
        }
    }

    return (
        <Dialog open={result !== null} onOpenChange={(o) => !o && onClose()}>
            <DialogContent className="sm:max-w-[560px]">
                <DialogHeader>
                    <DialogTitle>Token issued</DialogTitle>
                    <DialogDescription>
                        Copy this token now. After you close this dialog it can never
                        be retrieved again — issue a new one if you lose it.
                    </DialogDescription>
                </DialogHeader>
                {result && (
                    <div className="space-y-3">
                        <div
                            style={{
                                fontFamily: "var(--font-mono)",
                                fontSize: 12,
                                background: palette.surface,
                                border: `1px solid ${palette.border}`,
                                padding: `${space[3]}px ${space[4]}px`,
                                borderRadius: 6,
                                wordBreak: "break-all",
                            }}
                        >
                            {result.token}
                        </div>
                        <DataList
                            items={[
                                { label: "name", value: result.name },
                                {
                                    label: "scopes",
                                    value: <Mono size={12}>{result.scopes.join(" ")}</Mono>,
                                },
                                { label: "expires", value: fromNow(result.expires_at) },
                            ]}
                        />
                    </div>
                )}
                <DialogFooter>
                    <Button variant="outline" onClick={copy}>
                        <Copy className="size-3.5" />
                        Copy
                    </Button>
                    <Button onClick={onClose}>Done</Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}
