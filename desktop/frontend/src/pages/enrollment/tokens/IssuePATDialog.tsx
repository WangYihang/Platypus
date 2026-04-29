import { Loader2 } from "lucide-react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";

import {
    IssueEnrollmentTokenResponse,
    issueEnrollmentToken,
} from "../../../lib/api";
import { humanizeError } from "../../../lib/humanizeError";
import { formatSeconds } from "../../../lib/time";
import { Button } from "@/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";
import {
    Form,
    FormControl,
    FormDescription,
    FormField,
    FormItem,
    FormLabel,
    FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";

import { PATFormValues, patSchema } from "../schemas";

interface Props {
    open: boolean;
    onOpenChange: (o: boolean) => void;
    onIssued: (r: IssueEnrollmentTokenResponse) => void;
    projectID: string;
}

// IssuePATDialog issues a raw enrollment token for CI / scripted
// flows. Most operators should prefer the install-command surface
// (IssueInstallDialog or the EnrollAgentWizard); this exists for the
// uncommon case where the agent host can't run a curl|sh shape.
export default function IssuePATDialog({
    open,
    onOpenChange,
    onIssued,
    projectID,
}: Props) {
    const form = useForm<PATFormValues>({
        resolver: zodResolver(patSchema),
        // Same rationale as IssueInstallDialog — flag bad numeric
        // values on blur instead of waiting for submit.
        mode: "onBlur",
        defaultValues: { description: "", binding_machine_id: "" },
    });

    async function submit(v: PATFormValues) {
        try {
            const r = await issueEnrollmentToken(projectID, v);
            onIssued(r);
            form.reset({ description: "", binding_machine_id: "" });
        } catch (e) {
            toast.error(`Couldn't issue enrollment token: ${humanizeError(e)}`);
        }
    }

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="sm:max-w-[480px]">
                <DialogHeader>
                    <DialogTitle>Issue an enrollment token</DialogTitle>
                    <DialogDescription>
                        Raw token an agent can submit at /enroll. Use this when you
                        need the bare credential for a CI / scripted flow that can't
                        run the install command shape.
                    </DialogDescription>
                </DialogHeader>
                <Form {...form}>
                    <form onSubmit={form.handleSubmit(submit)} className="space-y-4">
                        <FormField
                            control={form.control}
                            name="description"
                            render={({ field }) => (
                                <FormItem>
                                    <FormLabel>Description</FormLabel>
                                    <FormControl>
                                        <Input
                                            autoFocus
                                            placeholder="Deploy for web-01"
                                            {...field}
                                        />
                                    </FormControl>
                                    <FormDescription>
                                        Free-form note shown in the list.
                                    </FormDescription>
                                    <FormMessage />
                                </FormItem>
                            )}
                        />
                        <FormField
                            control={form.control}
                            name="ttl_seconds"
                            render={({ field }) => (
                                <FormItem>
                                    <FormLabel>Expires in (seconds)</FormLabel>
                                    <FormControl>
                                        <Input
                                            type="number"
                                            inputMode="numeric"
                                            placeholder="3600 (= 1h)"
                                            value={field.value ?? ""}
                                            onChange={(e) =>
                                                field.onChange(
                                                    e.target.value === ""
                                                        ? undefined
                                                        : Number(e.target.value),
                                                )
                                            }
                                            onBlur={field.onBlur}
                                            name={field.name}
                                            ref={field.ref}
                                        />
                                    </FormControl>
                                    <FormDescription>
                                        How long the enrollment token stays valid.{" "}
                                        {typeof field.value === "number" && field.value > 0
                                            ? `= ${formatSeconds(field.value)}`
                                            : "Default 3600 (= 1h)."}
                                    </FormDescription>
                                    <FormMessage />
                                </FormItem>
                            )}
                        />
                        <FormField
                            control={form.control}
                            name="max_uses"
                            render={({ field }) => (
                                <FormItem>
                                    <FormLabel>Max uses</FormLabel>
                                    <FormControl>
                                        <Input
                                            type="number"
                                            inputMode="numeric"
                                            placeholder="1"
                                            value={field.value ?? ""}
                                            onChange={(e) =>
                                                field.onChange(
                                                    e.target.value === ""
                                                        ? undefined
                                                        : Number(e.target.value),
                                                )
                                            }
                                            onBlur={field.onBlur}
                                            name={field.name}
                                            ref={field.ref}
                                        />
                                    </FormControl>
                                    <FormDescription>Default 1 (single-use).</FormDescription>
                                    <FormMessage />
                                </FormItem>
                            )}
                        />
                        <FormField
                            control={form.control}
                            name="binding_machine_id"
                            render={({ field }) => (
                                <FormItem>
                                    <FormLabel>Restrict to machine</FormLabel>
                                    <FormControl>
                                        <Input
                                            placeholder="machine-id (optional)"
                                            {...field}
                                        />
                                    </FormControl>
                                    <FormDescription>
                                        Optional. Paste the host's
                                        <code> /etc/machine-id</code> contents to lock
                                        this enrollment token to that one host —
                                        useful for long-lived tokens, since a stolen
                                        token cannot be replayed elsewhere. Leave
                                        empty for short-lived tokens you don't plan
                                        to reuse.
                                    </FormDescription>
                                    <FormMessage />
                                </FormItem>
                            )}
                        />
                        <DialogFooter>
                            <Button
                                type="button"
                                variant="outline"
                                onClick={() => onOpenChange(false)}
                            >
                                Cancel
                            </Button>
                            <Button type="submit" disabled={form.formState.isSubmitting}>
                                {form.formState.isSubmitting && (
                                    <Loader2 className="size-3.5 animate-spin" />
                                )}
                                Issue
                            </Button>
                        </DialogFooter>
                    </form>
                </Form>
            </DialogContent>
        </Dialog>
    );
}
