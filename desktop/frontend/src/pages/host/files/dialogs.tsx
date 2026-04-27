import { useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";
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
    const [submitting, setSubmitting] = useState(false);

    useEffect(() => {
        if (open) setName("");
    }, [open]);

    async function handleSubmit(e: React.FormEvent) {
        e.preventDefault();
        if (!name.trim()) return;
        setSubmitting(true);
        try {
            await onConfirm(name.trim());
            onOpenChange(false);
        } finally {
            setSubmitting(false);
        }
    }

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent>
                <form onSubmit={handleSubmit}>
                    <DialogHeader>
                        <DialogTitle>New file</DialogTitle>
                        <DialogDescription>
                            Create an empty file inside {parentPath}. You can
                            edit it after it's created.
                        </DialogDescription>
                    </DialogHeader>
                    <div className="space-y-2 py-2">
                        <Label htmlFor="new-file-name">File name</Label>
                        <Input
                            id="new-file-name"
                            autoFocus
                            value={name}
                            onChange={(e) => setName(e.target.value)}
                            placeholder="notes.txt"
                        />
                    </div>
                    <DialogFooter>
                        <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                            Cancel
                        </Button>
                        <Button type="submit" disabled={submitting || !name.trim()}>
                            Create
                        </Button>
                    </DialogFooter>
                </form>
            </DialogContent>
        </Dialog>
    );
}

export function NewFolderDialog({ open, onOpenChange, parentPath, onConfirm }: NewFolderProps) {
    const [name, setName] = useState("");
    const [submitting, setSubmitting] = useState(false);

    useEffect(() => {
        if (open) setName("");
    }, [open]);

    async function handleSubmit(e: React.FormEvent) {
        e.preventDefault();
        if (!name.trim()) return;
        setSubmitting(true);
        try {
            await onConfirm(name.trim());
            onOpenChange(false);
        } finally {
            setSubmitting(false);
        }
    }

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent>
                <form onSubmit={handleSubmit}>
                    <DialogHeader>
                        <DialogTitle>New folder</DialogTitle>
                        <DialogDescription>Create a directory inside {parentPath}.</DialogDescription>
                    </DialogHeader>
                    <div className="space-y-2 py-2">
                        <Label htmlFor="new-folder-name">Folder name</Label>
                        <Input
                            id="new-folder-name"
                            autoFocus
                            value={name}
                            onChange={(e) => setName(e.target.value)}
                            placeholder="scripts"
                        />
                    </div>
                    <DialogFooter>
                        <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                            Cancel
                        </Button>
                        <Button type="submit" disabled={submitting || !name.trim()}>
                            Create
                        </Button>
                    </DialogFooter>
                </form>
            </DialogContent>
        </Dialog>
    );
}

interface RenameProps extends BaseProps {
    entry: FileEntryDTO | null;
    onConfirm: (newName: string) => Promise<void>;
}

export function RenameDialog({ open, onOpenChange, entry, onConfirm }: RenameProps) {
    const [name, setName] = useState("");
    const [submitting, setSubmitting] = useState(false);

    useEffect(() => {
        if (open && entry) setName(entry.name);
    }, [open, entry]);

    async function handleSubmit(e: React.FormEvent) {
        e.preventDefault();
        if (!name.trim() || name === entry?.name) return;
        setSubmitting(true);
        try {
            await onConfirm(name.trim());
            onOpenChange(false);
        } finally {
            setSubmitting(false);
        }
    }

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent>
                <form onSubmit={handleSubmit}>
                    <DialogHeader>
                        <DialogTitle>Rename</DialogTitle>
                        <DialogDescription>
                            Rename {entry?.name ?? "—"} (cross-filesystem moves may fail with EXDEV).
                        </DialogDescription>
                    </DialogHeader>
                    <div className="space-y-2 py-2">
                        <Label htmlFor="rename-input">New name</Label>
                        <Input
                            id="rename-input"
                            autoFocus
                            value={name}
                            onChange={(e) => setName(e.target.value)}
                        />
                    </div>
                    <DialogFooter>
                        <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                            Cancel
                        </Button>
                        <Button type="submit" disabled={submitting || !name.trim() || name === entry?.name}>
                            Rename
                        </Button>
                    </DialogFooter>
                </form>
            </DialogContent>
        </Dialog>
    );
}

interface ChmodProps extends BaseProps {
    entry: FileEntryDTO | null;
    onConfirm: (octalMode: number) => Promise<void>;
}

export function ChmodDialog({ open, onOpenChange, entry, onConfirm }: ChmodProps) {
    const [mode, setMode] = useState("644");
    const [submitting, setSubmitting] = useState(false);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        if (open && entry) {
            setMode(formatModeOctal(entry.mode));
            setError(null);
        }
    }, [open, entry]);

    async function handleSubmit(e: React.FormEvent) {
        e.preventDefault();
        const parsed = parseInt(mode, 8);
        if (!/^[0-7]{3,4}$/.test(mode) || Number.isNaN(parsed)) {
            setError("Mode must be 3 or 4 octal digits (e.g. 644 or 0755)");
            return;
        }
        setError(null);
        setSubmitting(true);
        try {
            await onConfirm(parsed);
            onOpenChange(false);
        } finally {
            setSubmitting(false);
        }
    }

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent>
                <form onSubmit={handleSubmit}>
                    <DialogHeader>
                        <DialogTitle>Change mode</DialogTitle>
                        <DialogDescription>
                            Set octal permissions for {entry?.name ?? "—"} (e.g. 644 for rw-r--r--).
                            On Windows only the owner-write bit is meaningful.
                        </DialogDescription>
                    </DialogHeader>
                    <div className="space-y-2 py-2">
                        <Label htmlFor="chmod-input">Octal mode</Label>
                        <Input
                            id="chmod-input"
                            autoFocus
                            value={mode}
                            onChange={(e) => setMode(e.target.value)}
                            placeholder="644"
                        />
                        {error && <p className="text-sm text-red-500">{error}</p>}
                    </div>
                    <DialogFooter>
                        <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                            Cancel
                        </Button>
                        <Button type="submit" disabled={submitting || !mode}>
                            Apply
                        </Button>
                    </DialogFooter>
                </form>
            </DialogContent>
        </Dialog>
    );
}

interface DeleteConfirmProps extends BaseProps {
    entries: FileEntryDTO[];
    onConfirm: () => Promise<void>;
}

export function DeleteConfirmDialog({ open, onOpenChange, entries, onConfirm }: DeleteConfirmProps) {
    const [submitting, setSubmitting] = useState(false);
    const hasDir = entries.some((e) => e.isDir);

    async function handleConfirm() {
        setSubmitting(true);
        try {
            await onConfirm();
            onOpenChange(false);
        } finally {
            setSubmitting(false);
        }
    }

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent>
                <DialogHeader>
                    <DialogTitle>
                        Delete {entries.length === 1 ? entries[0].name : `${entries.length} items`}?
                    </DialogTitle>
                    <DialogDescription>
                        {hasDir
                            ? "Directories will be removed recursively. This cannot be undone."
                            : "This cannot be undone."}
                    </DialogDescription>
                </DialogHeader>
                <DialogFooter>
                    <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                        Cancel
                    </Button>
                    <Button
                        type="button"
                        variant="destructive"
                        onClick={handleConfirm}
                        disabled={submitting}
                    >
                        Delete
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}
