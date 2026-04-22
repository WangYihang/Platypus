import { useCallback, useEffect, useState } from "react";
import { Loader2, Plus, RotateCw } from "lucide-react";
import { toast } from "sonner";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import Card from "../../components/Card";
import EmptyState from "../../components/EmptyState";
import Mono from "../../components/Mono";
import StatusPill from "../../components/StatusPill";
import { CreateTunnel, ListTunnels } from "../../../wailsjs/go/app/App";
import type { api } from "../../../wailsjs/go/models";
import { palette, space } from "../../layout/theme";

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
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "@/components/ui/select";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";

const MODES = ["pull", "push", "dynamic", "internet"] as const;
type Mode = (typeof MODES)[number];

const MODE_DESC: Record<Mode, string> = {
    pull: "Inbound: agent listens on dst, forwards to src on operator side",
    push: "Outbound: operator listens on src, agent dials dst on its own network",
    dynamic: "Agent runs a SOCKS5 server on a free port (request from agent)",
    internet: "Server runs SOCKS5 at src and proxies via the agent to dst",
};

const tunnelSchema = z.object({
    mode: z.enum(MODES),
    srcAddress: z.string(),
    dstAddress: z.string(),
});
type TunnelFormValues = z.infer<typeof tunnelSchema>;

interface Props {
    sessionHash: string;
}

// TunnelsTab is the per-session tunnel manager embedded in HostView.
// sessionHash flows in as a prop from the HostView chip row.
export default function TunnelsTab({ sessionHash }: Props) {
    const [tunnels, setTunnels] = useState<api.TunnelInfo[]>([]);
    const [modalOpen, setModalOpen] = useState(false);

    const form = useForm<TunnelFormValues>({
        resolver: zodResolver(tunnelSchema),
        defaultValues: { mode: "dynamic", srcAddress: "", dstAddress: "" },
    });

    const mode = form.watch("mode");

    const refresh = useCallback(async () => {
        try {
            setTunnels(await ListTunnels(sessionHash));
        } catch (err) {
            toast.error(`refresh: ${String(err)}`);
        }
    }, [sessionHash]);

    useEffect(() => {
        refresh();
    }, [refresh]);

    async function handleCreate(v: TunnelFormValues) {
        try {
            await CreateTunnel(sessionHash, v.mode, v.srcAddress, v.dstAddress);
            toast.success(`${v.mode} tunnel created`);
            setModalOpen(false);
            form.reset({ mode: "dynamic", srcAddress: "", dstAddress: "" });
            refresh();
        } catch (err) {
            toast.error(`create: ${String(err)}`);
        }
    }

    return (
        <div style={{ display: "flex", flexDirection: "column", gap: space[3] }}>
            <div
                style={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                }}
            >
                <h3
                    style={{
                        margin: 0,
                        color: palette.textPrimary,
                        fontSize: 14,
                        fontWeight: 600,
                    }}
                >
                    Active tunnels
                </h3>
                <div className="flex gap-2">
                    <Button size="sm" variant="outline" onClick={refresh}>
                        <RotateCw className="size-3.5" />
                        Refresh
                    </Button>
                    <Button size="sm" onClick={() => setModalOpen(true)}>
                        <Plus className="size-3.5" />
                        New tunnel
                    </Button>
                </div>
            </div>

            <Card padding={0}>
                {tunnels.length === 0 ? (
                    <EmptyState
                        title="No active tunnels"
                        description="Open a tunnel on this session via New tunnel."
                    />
                ) : (
                    <Table>
                        <TableHeader>
                            <TableRow>
                                <TableHead className="w-[120px]">Type</TableHead>
                                <TableHead>Address</TableHead>
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {tunnels.map((t) => (
                                <TableRow key={`${t.type}:${t.address}`}>
                                    <TableCell>
                                        <StatusPill tone="info">{t.type}</StatusPill>
                                    </TableCell>
                                    <TableCell>
                                        <Mono>{t.address}</Mono>
                                    </TableCell>
                                </TableRow>
                            ))}
                        </TableBody>
                    </Table>
                )}
            </Card>

            <Dialog open={modalOpen} onOpenChange={setModalOpen}>
                <DialogContent className="sm:max-w-[480px]">
                    <DialogHeader>
                        <DialogTitle>New tunnel</DialogTitle>
                        <DialogDescription>{MODE_DESC[mode as Mode]}</DialogDescription>
                    </DialogHeader>
                    <Form {...form}>
                        <form
                            onSubmit={form.handleSubmit(handleCreate)}
                            className="space-y-4"
                        >
                            <FormField
                                control={form.control}
                                name="mode"
                                render={({ field }) => (
                                    <FormItem>
                                        <FormLabel>Mode</FormLabel>
                                        <Select
                                            value={field.value}
                                            onValueChange={field.onChange}
                                        >
                                            <FormControl>
                                                <SelectTrigger>
                                                    <SelectValue />
                                                </SelectTrigger>
                                            </FormControl>
                                            <SelectContent>
                                                {MODES.map((m) => (
                                                    <SelectItem key={m} value={m}>
                                                        {m}
                                                    </SelectItem>
                                                ))}
                                            </SelectContent>
                                        </Select>
                                        <FormMessage />
                                    </FormItem>
                                )}
                            />
                            <FormField
                                control={form.control}
                                name="srcAddress"
                                render={({ field }) => (
                                    <FormItem>
                                        <FormLabel>src_address</FormLabel>
                                        <FormControl>
                                            <Input placeholder="0.0.0.0:1080" {...field} />
                                        </FormControl>
                                        <FormDescription>
                                            dynamic mode: ignored — server picks a free port
                                        </FormDescription>
                                        <FormMessage />
                                    </FormItem>
                                )}
                            />
                            <FormField
                                control={form.control}
                                name="dstAddress"
                                render={({ field }) => (
                                    <FormItem>
                                        <FormLabel>dst_address</FormLabel>
                                        <FormControl>
                                            <Input placeholder="127.0.0.1:80" {...field} />
                                        </FormControl>
                                        <FormDescription>
                                            dynamic: ignored. internet: target IP:port the SOCKS5
                                            server proxies to.
                                        </FormDescription>
                                        <FormMessage />
                                    </FormItem>
                                )}
                            />
                            <DialogFooter>
                                <Button
                                    type="button"
                                    variant="outline"
                                    onClick={() => setModalOpen(false)}
                                >
                                    Cancel
                                </Button>
                                <Button type="submit" disabled={form.formState.isSubmitting}>
                                    {form.formState.isSubmitting && (
                                        <Loader2 className="size-3.5 animate-spin" />
                                    )}
                                    Create
                                </Button>
                            </DialogFooter>
                        </form>
                    </Form>
                </DialogContent>
            </Dialog>
        </div>
    );
}
