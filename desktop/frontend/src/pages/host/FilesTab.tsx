import { useState } from "react";
import { Download, Loader2, Upload } from "lucide-react";
import { toast } from "sonner";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

import Card from "../../components/Card";
import DataList from "../../components/DataList";
import Mono from "../../components/Mono";
import {
    DownloadFile,
    FileSize,
    PickFileToUpload,
    PickSaveLocation,
    UploadFile,
} from "../../../wailsjs/go/app/App";
import { basename, humanize } from "../../lib/format";
import { palette, space } from "../../layout/theme";

import { Button } from "@/components/ui/button";
import {
    Form,
    FormControl,
    FormField,
    FormItem,
    FormLabel,
    FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";

const fileFormSchema = z.object({
    remotePath: z.string().min(1, "remote path is required"),
});
type FileFormValues = z.infer<typeof fileFormSchema>;

interface Props {
    sessionHash: string;
}

// FilesTab is the per-session file-transfer panel embedded in HostView.
// Path-based for now (no directory listing): the user types an exact
// remote path, then Get Size / Download / Upload act on it.
export default function FilesTab({ sessionHash }: Props) {
    const [size, setSize] = useState<number | null>(null);
    const [lastPath, setLastPath] = useState<string | null>(null);
    const [busy, setBusy] = useState<"" | "size" | "download" | "upload">("");

    const form = useForm<FileFormValues>({
        resolver: zodResolver(fileFormSchema),
        defaultValues: { remotePath: "" },
    });

    async function refreshSize() {
        const v = await form.trigger();
        if (!v) return;
        const values = form.getValues();
        setBusy("size");
        try {
            const n = await FileSize(sessionHash, values.remotePath);
            setSize(n);
            setLastPath(values.remotePath);
        } catch (err) {
            toast.error(`size: ${String(err)}`);
            setSize(null);
        } finally {
            setBusy("");
        }
    }

    async function download() {
        const valid = await form.trigger();
        if (!valid) return;
        const values = form.getValues();
        const dst = await PickSaveLocation("Save to", basename(values.remotePath));
        if (!dst) return;
        setBusy("download");
        try {
            await DownloadFile(sessionHash, values.remotePath, dst);
            toast.success(`Saved to ${dst}`);
        } catch (err) {
            toast.error(`download: ${String(err)}`);
        } finally {
            setBusy("");
        }
    }

    async function upload() {
        const valid = await form.trigger();
        if (!valid) return;
        const values = form.getValues();
        const src = await PickFileToUpload("Choose local file");
        if (!src) return;
        setBusy("upload");
        try {
            await UploadFile(sessionHash, values.remotePath, src);
            toast.success(`Uploaded ${src} → ${values.remotePath}`);
        } catch (err) {
            toast.error(`upload: ${String(err)}`);
        } finally {
            setBusy("");
        }
    }

    return (
        <div
            style={{
                maxWidth: 720,
                display: "flex",
                flexDirection: "column",
                gap: space[4],
            }}
        >
            <Card header="Transfer">
                <p
                    style={{
                        margin: `0 0 ${space[4]}px`,
                        color: palette.textSecondary,
                        fontSize: 13,
                        lineHeight: 1.5,
                    }}
                >
                    Path-based for now — provide an exact remote path, then Get Size / Download /
                    Upload.
                </p>
                <Form {...form}>
                    {/* Buttons intentionally trigger separate workflows (size / download /
                        upload) rather than a single submit; RHF trigger()+getValues is
                        cleaner than wrapping each in its own handleSubmit. */}
                    <form className="space-y-4" onSubmit={(e) => e.preventDefault()}>
                        <FormField
                            control={form.control}
                            name="remotePath"
                            render={({ field }) => (
                                <FormItem>
                                    <FormLabel>Remote path</FormLabel>
                                    <FormControl>
                                        <Input
                                            placeholder="/etc/hostname"
                                            autoFocus
                                            {...field}
                                        />
                                    </FormControl>
                                    <FormMessage />
                                </FormItem>
                            )}
                        />
                        <div className="flex gap-2">
                            <Button
                                type="button"
                                variant="outline"
                                onClick={refreshSize}
                                disabled={busy === "size"}
                            >
                                {busy === "size" && (
                                    <Loader2 className="size-3.5 animate-spin" />
                                )}
                                Get size
                            </Button>
                            <Button
                                type="button"
                                variant="outline"
                                onClick={download}
                                disabled={busy === "download"}
                            >
                                {busy === "download" ? (
                                    <Loader2 className="size-3.5 animate-spin" />
                                ) : (
                                    <Download className="size-3.5" />
                                )}
                                Download
                            </Button>
                            <Button
                                type="button"
                                onClick={upload}
                                disabled={busy === "upload"}
                            >
                                {busy === "upload" ? (
                                    <Loader2 className="size-3.5 animate-spin" />
                                ) : (
                                    <Upload className="size-3.5" />
                                )}
                                Upload
                            </Button>
                        </div>
                    </form>
                </Form>
            </Card>

            {size !== null && lastPath && (
                <Card header="Size">
                    <DataList
                        items={[
                            { label: "path", value: <Mono>{lastPath}</Mono> },
                            { label: "bytes", value: <Mono>{size}</Mono> },
                            { label: "human", value: humanize(size) },
                        ]}
                    />
                </Card>
            )}
        </div>
    );
}
