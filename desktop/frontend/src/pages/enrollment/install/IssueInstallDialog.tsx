import { useEffect, useState } from "react";
import { Loader2 } from "lucide-react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";

import Mono from "../../../components/Mono";
import {
    IssueInstallResponse,
    getServerInfo,
    issueInstallArtifact,
    listInstallPlatforms,
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

import PlatformPickerField from "../PlatformPickerField";
import { PlatformsState } from "../platforms";
import { InstallFormValues, installSchema } from "../schemas";

interface Props {
    open: boolean;
    onOpenChange: (o: boolean) => void;
    onIssued: (r: IssueInstallResponse) => void;
    projectID: string;
}

// IssueInstallDialog is the management-page form for generating a new
// install command. Distinct from the four-step EnrollAgentWizard on
// FleetPage — that flow is for day-to-day onboarding; this dialog is
// for power users on the management surface (Audit → Enrollment) who
// want every option visible at once.
export default function IssueInstallDialog({
    open,
    onOpenChange,
    onIssued,
    projectID,
}: Props) {
    const form = useForm<InstallFormValues>({
        resolver: zodResolver(installSchema),
        // onBlur revalidation surfaces "must be a positive integer"
        // immediately after the user leaves the TTL field instead of
        // waiting for submit — matters most for the numeric inputs
        // where a typo is silently accepted until dismissal.
        mode: "onBlur",
        defaultValues: {
            server_endpoint: "",
            target_os: "",
            target_arch: "",
            auto_approve: false,
        },
    });

    // Live (os, arch) list from the active channel's manifest.
    const [platforms, setPlatforms] = useState<PlatformsState>({
        status: "loading",
    });

    // Pre-fill server_endpoint with the server's public_addr and load
    // the platform list whenever the dialog opens. Both are best-effort
    // — failures fall back to a usable empty state.
    useEffect(() => {
        if (!open) return;
        getServerInfo()
            .then((info) => {
                if (info.public_addr) {
                    form.setValue("server_endpoint", info.public_addr);
                }
            })
            .catch(() => {
                /* best-effort — the field stays blank if /info fails */
            });
        setPlatforms({ status: "loading" });
        listInstallPlatforms()
            .then((r) => {
                if (r.platforms.length === 0) {
                    setPlatforms({ status: "empty", channel: r.channel });
                } else {
                    setPlatforms({
                        status: "ready",
                        platforms: r.platforms,
                        channel: r.channel,
                    });
                }
            })
            .catch((e) => {
                setPlatforms({ status: "error", message: humanizeError(e) });
            });
    }, [open, form]);

    async function submit(v: InstallFormValues) {
        try {
            const r = await issueInstallArtifact(projectID, v);
            onIssued(r);
            form.reset({ server_endpoint: "", target_os: "", target_arch: "" });
        } catch (e) {
            toast.error(`Couldn't generate install command: ${humanizeError(e)}`);
        }
    }

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="sm:max-w-[480px]">
                <DialogHeader>
                    <DialogTitle>Generate install command</DialogTitle>
                    <DialogDescription>
                        One-shot `curl ... | sh` bootstrap that issues a single-use
                        enrollment token on first fetch.
                    </DialogDescription>
                </DialogHeader>
                <Form {...form}>
                    <form onSubmit={form.handleSubmit(submit)} className="space-y-4">
                        <FormField
                            control={form.control}
                            name="server_endpoint"
                            render={({ field }) => (
                                <FormItem>
                                    <FormLabel>Agent should dial</FormLabel>
                                    <FormControl>
                                        <Input
                                            autoFocus
                                            placeholder="203.0.113.5:13337"
                                            {...field}
                                        />
                                    </FormControl>
                                    <FormDescription>
                                        host:port agents dial; defaults to this server's
                                        unified ingress address
                                    </FormDescription>
                                    <FormMessage />
                                </FormItem>
                            )}
                        />
                        <PlatformPickerField form={form} platforms={platforms} />
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
                                            placeholder="300 (= 5m)"
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
                                        How long the install URL stays valid.{" "}
                                        {typeof field.value === "number" && field.value > 0
                                            ? `= ${formatSeconds(field.value)}`
                                            : "Default 300 (= 5m)."}
                                    </FormDescription>
                                    <FormMessage />
                                </FormItem>
                            )}
                        />
                        <FormField
                            control={form.control}
                            name="auto_approve"
                            render={({ field }) => (
                                <FormItem className="flex flex-row items-start space-x-3 space-y-0 rounded-md border p-3">
                                    <FormControl>
                                        <input
                                            type="checkbox"
                                            checked={!!field.value}
                                            onChange={(e) => field.onChange(e.target.checked)}
                                            onBlur={field.onBlur}
                                            ref={field.ref}
                                            name={field.name}
                                            style={{ marginTop: 4 }}
                                        />
                                    </FormControl>
                                    <div className="space-y-1 leading-none">
                                        <FormLabel className="cursor-pointer">
                                            Skip admin approval (automation)
                                        </FormLabel>
                                        <FormDescription>
                                            When off (default), the host that uses this
                                            install link lands in <Mono>pending</Mono> and
                                            an admin has to click Approve on Fleet →
                                            Approvals before the agent can open a link.
                                            Turn on for unattended flows (Ansible, CI,
                                            cloud-init) where there's no human-in-the-loop
                                            step.
                                        </FormDescription>
                                    </div>
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
                                Generate
                            </Button>
                        </DialogFooter>
                    </form>
                </Form>
            </DialogContent>
        </Dialog>
    );
}
