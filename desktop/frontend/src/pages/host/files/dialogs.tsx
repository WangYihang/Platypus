import { useEffect, useState } from "react";

import FormDialog from "../../../components/FormDialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

import { formatModeOctal } from "./paths";
import type { FileEntryDTO } from "../../../platform/App.web";

interface BaseProps {
    open: boolean;
    onOpenChange: (open: boolean) => void;
}

interface NewFolderProps extends BaseProps {
    parentPath: string;
    onConfirm: (folderName: string) => Promise<void>;
}

interface NewFileProps extends BaseProps {
    parentPath: string;
    onConfirm: (fileName: string) => Promise<void>;
}

export function NewFileDialog({ open, onOpenChange, parentPath, onConfirm }: NewFileProps) {
    const [name, setName] = useState("");

    useEffect(() => {
        if (open) setName("");
    }, [open]);

    return (
        <FormDialog
            open={open}
            onOpenChange={onOpenChange}
            title="New file"
            description={`Create an empty file inside ${parentPath}. You can edit it after it's created.`}
            submitLabel="Create"
            submitDisabled={!name.trim()}
            onSubmit={async () => {
                await onConfirm(name.trim());
                onOpenChange(false);
            }}
        >
            <Label htmlFor="new-file-name">File name</Label>
            <Input
                id="new-file-name"
                autoFocus
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="notes.txt"
            />
        </FormDialog>
    );
}

export function NewFolderDialog({ open, onOpenChange, parentPath, onConfirm }: NewFolderProps) {
    const [name, setName] = useState("");

    useEffect(() => {
        if (open) setName("");
    }, [open]);

    return (
        <FormDialog
            open={open}
            onOpenChange={onOpenChange}
            title="New folder"
            description={`Create a directory inside ${parentPath}.`}
            submitLabel="Create"
            submitDisabled={!name.trim()}
            onSubmit={async () => {
                await onConfirm(name.trim());
                onOpenChange(false);
            }}
        >
            <Label htmlFor="new-folder-name">Folder name</Label>
            <Input
                id="new-folder-name"
                autoFocus
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="scripts"
            />
        </FormDialog>
    );
}

interface RenameProps extends BaseProps {
    entry: FileEntryDTO | null;
    onConfirm: (newName: string) => Promise<void>;
}

export function RenameDialog({ open, onOpenChange, entry, onConfirm }: RenameProps) {
    const [name, setName] = useState("");

    useEffect(() => {
        if (open && entry) setName(entry.name);
    }, [open, entry]);

    return (
        <FormDialog
            open={open}
            onOpenChange={onOpenChange}
            title="Rename"
            description={`Rename ${entry?.name ?? "—"} (cross-filesystem moves may fail with EXDEV).`}
            submitLabel="Rename"
            submitDisabled={!name.trim() || name === entry?.name}
            onSubmit={async () => {
                await onConfirm(name.trim());
                onOpenChange(false);
            }}
        >
            <Label htmlFor="rename-input">New name</Label>
            <Input
                id="rename-input"
                autoFocus
                value={name}
                onChange={(e) => setName(e.target.value)}
            />
        </FormDialog>
    );
}

interface ChmodProps extends BaseProps {
    entry: FileEntryDTO | null;
    onConfirm: (octalMode: number) => Promise<void>;
}

export function ChmodDialog({ open, onOpenChange, entry, onConfirm }: ChmodProps) {
    const [mode, setMode] = useState("644");
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        if (open && entry) {
            setMode(formatModeOctal(entry.mode));
            setError(null);
        }
    }, [open, entry]);

    return (
        <FormDialog
            open={open}
            onOpenChange={onOpenChange}
            title="Change mode"
            description={`Set octal permissions for ${
                entry?.name ?? "—"
            } (e.g. 644 for rw-r--r--). On Windows only the owner-write bit is meaningful.`}
            submitLabel="Apply"
            submitDisabled={!mode}
            onSubmit={async () => {
                const parsed = parseInt(mode, 8);
                if (!/^[0-7]{3,4}$/.test(mode) || Number.isNaN(parsed)) {
                    setError("Mode must be 3 or 4 octal digits (e.g. 644 or 0755)");
                    // Throw so FormDialog's `submitting` flag clears via the
                    // try/finally without proceeding past onConfirm.
                    throw new Error("invalid mode");
                }
                setError(null);
                await onConfirm(parsed);
                onOpenChange(false);
            }}
        >
            <Label htmlFor="chmod-input">Octal mode</Label>
            <Input
                id="chmod-input"
                autoFocus
                value={mode}
                onChange={(e) => setMode(e.target.value)}
                placeholder="644"
            />
            {error && <p className="text-sm text-red-500">{error}</p>}
        </FormDialog>
    );
}

interface DeleteConfirmProps extends BaseProps {
    entries: FileEntryDTO[];
    onConfirm: () => Promise<void>;
}

export function DeleteConfirmDialog({ open, onOpenChange, entries, onConfirm }: DeleteConfirmProps) {
    const hasDir = entries.some((e) => e.isDir);

    return (
        <FormDialog
            open={open}
            onOpenChange={onOpenChange}
            title={`Delete ${
                entries.length === 1 ? entries[0].name : `${entries.length} items`
            }?`}
            description={
                hasDir
                    ? "Directories will be removed recursively. This cannot be undone."
                    : "This cannot be undone."
            }
            submitLabel="Delete"
            destructive
            onSubmit={async () => {
                await onConfirm();
                onOpenChange(false);
            }}
        >
            {/* Confirm dialog has no body fields. The empty children
                slot collapses to an empty space-y-2 wrapper. */}
            <></>
        </FormDialog>
    );
}
