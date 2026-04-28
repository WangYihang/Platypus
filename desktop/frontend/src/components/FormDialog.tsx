import { ReactNode, useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";

interface FormDialogProps {
    open: boolean;
    onOpenChange: (open: boolean) => void;
    title: ReactNode;
    description?: ReactNode;
    submitLabel?: string;
    cancelLabel?: string;
    // When true the submit button takes a destructive tone. AlertDialog
    // remains the right primitive for "are you sure" confirmations
    // (different focus/aria semantics) — this flag is for forms that
    // submit a value but happen to perform a destructive operation.
    destructive?: boolean;
    // Optional disabler — pass a derived `!form.isValid` to keep the
    // submit button greyed until the form is ready. The dialog
    // additionally disables the submit while a submission is in
    // flight.
    submitDisabled?: boolean;
    // Called with no args; should return a Promise that resolves
    // when the operation is complete. The dialog manages the
    // "submitting…" state and closes on resolve.
    onSubmit: () => Promise<void> | void;
    // The form body. The dialog handles everything around it
    // (header, footer, submit/cancel buttons, escape/backdrop). The
    // body is wrapped in a <form> so an Enter on the input still
    // submits.
    children: ReactNode;
}

// FormDialog packages the surrounding chrome that every domain
// dialog (NewFile, NewFolder, Rename, Chmod, AddServer login,
// ProjectSidebar create-project, …) used to spell out by hand.
// What's left in each consumer is the actual form fields plus an
// `onSubmit` callback — typically 30-60 LOC down from 80-120.
//
// Why not a render-prop with `<FormDialog>{(form) => …}</FormDialog>`
// and built-in react-hook-form? The dialogs span a wide range of
// validation styles (zod schemas, raw refs, single-input forms with
// trim-then-non-empty checks). Forcing one path adds friction; this
// shape just owns the chrome and lets each dialog keep its own
// state.
export default function FormDialog({
    open,
    onOpenChange,
    title,
    description,
    submitLabel = "Save",
    cancelLabel = "Cancel",
    destructive = false,
    submitDisabled = false,
    onSubmit,
    children,
}: FormDialogProps) {
    const [submitting, setSubmitting] = useState(false);

    // Reset the in-flight flag whenever the dialog re-opens. Without
    // this a previous submission that errored mid-flight could leave
    // submitting=true, locking out the next attempt.
    useEffect(() => {
        if (open) setSubmitting(false);
    }, [open]);

    async function handleSubmit(e: React.FormEvent) {
        e.preventDefault();
        if (submitting || submitDisabled) return;
        setSubmitting(true);
        try {
            await onSubmit();
        } finally {
            setSubmitting(false);
        }
    }

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent>
                <form onSubmit={handleSubmit}>
                    <DialogHeader>
                        <DialogTitle>{title}</DialogTitle>
                        {description ? (
                            <DialogDescription>{description}</DialogDescription>
                        ) : null}
                    </DialogHeader>
                    <div className="space-y-2 py-2">{children}</div>
                    <DialogFooter>
                        <Button
                            type="button"
                            variant="outline"
                            onClick={() => onOpenChange(false)}
                        >
                            {cancelLabel}
                        </Button>
                        <Button
                            type="submit"
                            variant={destructive ? "destructive" : "default"}
                            disabled={submitting || submitDisabled}
                        >
                            {submitLabel}
                        </Button>
                    </DialogFooter>
                </form>
            </DialogContent>
        </Dialog>
    );
}
