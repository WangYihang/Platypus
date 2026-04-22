import { useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import Card from "../components/Card";
import EmptyState from "../components/EmptyState";
import Mono from "../components/Mono";
import PageHeader from "../components/PageHeader";
import StatusPill from "../components/StatusPill";
import { font, palette, space } from "../layout/theme";
import { DispatchResult, dispatchCommand } from "../lib/api";

import { Button } from "@/components/ui/button";
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
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";

const dispatchSchema = z.object({
    command: z.string().min(1, "command is required"),
    timeout: z.number().int().min(1, "min 1s").max(120, "max 120s"),
});
type DispatchFormValues = z.infer<typeof dispatchSchema>;

interface Props {
    projectID: string;
    projectName: string;
}

// DispatchPanel is the in-line "run a command on every flagged live
// session" view. Selecting Dispatch in the sidebar opens it as the
// main-panel content (no modal) so results stay visible across runs.
export default function DispatchPanel({ projectID, projectName }: Props) {
    const [busy, setBusy] = useState(false);
    const [results, setResults] = useState<DispatchResult[] | null>(null);

    const form = useForm<DispatchFormValues>({
        resolver: zodResolver(dispatchSchema),
        defaultValues: { command: "id", timeout: 3 },
    });

    async function run(v: DispatchFormValues) {
        setBusy(true);
        try {
            setResults(await dispatchCommand(projectID, v.command, v.timeout));
        } catch (e) {
            toast.error(`dispatch: ${String(e)}`);
        } finally {
            setBusy(false);
        }
    }

    return (
        <div style={{ display: "flex", flexDirection: "column", height: "100%" }}>
            <PageHeader
                title="Dispatch"
                subtitle={`Run a command on every flagged live session in ${projectName}`}
            />
            <div style={{ flex: 1, overflow: "auto", padding: space[8] }}>
                <div className="flex flex-col gap-4">
                    <Card header="Run command" style={{ maxWidth: 720 }}>
                        <p
                            style={{
                                margin: `0 0 ${space[4]}px`,
                                color: palette.textSecondary,
                                fontSize: 13,
                                lineHeight: 1.5,
                            }}
                        >
                            Sessions are targeted by their <Mono>group_dispatch</Mono> flag. Flip
                            a session's flag from its HostView Sessions tab to include it. Only
                            sessions that are <em>live</em> and <em>flagged</em> will receive
                            the command.
                        </p>
                        <Form {...form}>
                            <form onSubmit={form.handleSubmit(run)} className="space-y-4">
                                <FormField
                                    control={form.control}
                                    name="command"
                                    render={({ field }) => (
                                        <FormItem>
                                            <FormLabel>Command</FormLabel>
                                            <FormControl>
                                                <Textarea
                                                    rows={2}
                                                    placeholder="id"
                                                    className="font-mono"
                                                    {...field}
                                                />
                                            </FormControl>
                                            <FormMessage />
                                        </FormItem>
                                    )}
                                />
                                <FormField
                                    control={form.control}
                                    name="timeout"
                                    render={({ field }) => (
                                        <FormItem>
                                            <FormLabel>Per-session timeout (seconds)</FormLabel>
                                            <FormControl>
                                                <Input
                                                    type="number"
                                                    inputMode="numeric"
                                                    min={1}
                                                    max={120}
                                                    className="max-w-[160px]"
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
                                                Each session gets its own timeout — slow boxes
                                                don't block the rest.
                                            </FormDescription>
                                            <FormMessage />
                                        </FormItem>
                                    )}
                                />
                                <div className="flex gap-2">
                                    <Button type="submit" disabled={busy}>
                                        {busy && <Loader2 className="size-3.5 animate-spin" />}
                                        Run
                                    </Button>
                                    <Button
                                        type="button"
                                        variant="outline"
                                        onClick={() => setResults(null)}
                                        disabled={!results}
                                    >
                                        Clear results
                                    </Button>
                                </div>
                            </form>
                        </Form>
                    </Card>

                    {results !== null && (
                        <Card
                            header={
                                <span>
                                    Results{" "}
                                    <Mono color={palette.textSecondary}>
                                        ({results.length})
                                    </Mono>
                                </span>
                            }
                            padding={0}
                        >
                            {results.length === 0 ? (
                                <EmptyState
                                    title="No flagged live sessions"
                                    description="Flip a session's group_dispatch flag from its host's Sessions tab to include it here."
                                />
                            ) : (
                                <Table>
                                    <TableHeader>
                                        <TableRow>
                                            <TableHead className="w-[180px]">Session</TableHead>
                                            <TableHead className="w-[120px]">Host</TableHead>
                                            <TableHead>Output</TableHead>
                                        </TableRow>
                                    </TableHeader>
                                    <TableBody>
                                        {results.map((r) => (
                                            <TableRow key={r.session_hash}>
                                                <TableCell>
                                                    <Mono>{`${r.session_hash.slice(0, 16)}…`}</Mono>
                                                </TableCell>
                                                <TableCell>
                                                    <Mono>{`${r.host_id.slice(0, 8)}…`}</Mono>
                                                </TableCell>
                                                <TableCell>
                                                    {r.error ? (
                                                        <StatusPill tone="danger">
                                                            {r.error}
                                                        </StatusPill>
                                                    ) : (
                                                        <pre
                                                            style={{
                                                                margin: 0,
                                                                whiteSpace: "pre-wrap",
                                                                fontFamily: font.mono,
                                                                fontSize: 12,
                                                                color: palette.textPrimary,
                                                            }}
                                                        >
                                                            {r.output || (
                                                                <span className="text-text-muted">
                                                                    (empty)
                                                                </span>
                                                            )}
                                                        </pre>
                                                    )}
                                                </TableCell>
                                            </TableRow>
                                        ))}
                                    </TableBody>
                                </Table>
                            )}
                        </Card>
                    )}
                </div>
            </div>
        </div>
    );
}
