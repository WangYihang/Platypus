import { useEffect, useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";

import { humanizeError } from "../../../lib/humanizeError";
import {
    type CreateRBACRoleRequest,
    type RBACPermission,
    createRBACRole,
} from "../../../lib/api";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
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
import { Textarea } from "@/components/ui/textarea";

import PermissionMatrix from "./PermissionMatrix";

interface Props {
    open: boolean;
    onOpenChange: (o: boolean) => void;
    permissions: RBACPermission[];
}

export default function CreateRoleDialog({ open, onOpenChange, permissions }: Props) {
    const [slug, setSlug] = useState("");
    const [name, setName] = useState("");
    const [description, setDescription] = useState("");
    const [isGlobal, setIsGlobal] = useState(true);
    const [isProject, setIsProject] = useState(true);
    const [selected, setSelected] = useState<Set<string>>(new Set());
    const [submitting, setSubmitting] = useState(false);

    useEffect(() => {
        if (open) {
            setSlug("");
            setName("");
            setDescription("");
            setIsGlobal(true);
            setIsProject(true);
            setSelected(new Set());
            setSubmitting(false);
        }
    }, [open]);

    async function submit(e: React.FormEvent) {
        e.preventDefault();
        if (!slug.trim() || !name.trim()) return;
        if (!isGlobal && !isProject) return;
        setSubmitting(true);
        const req: CreateRBACRoleRequest = {
            slug: slug.trim(),
            name: name.trim(),
            description: description.trim() || undefined,
            is_global: isGlobal,
            is_project: isProject,
            permissions: Array.from(selected),
        };
        try {
            await createRBACRole(req);
            toast.success("Role created");
            onOpenChange(false);
        } catch (e) {
            toast.error(`Couldn't create: ${humanizeError(e)}`);
        } finally {
            setSubmitting(false);
        }
    }

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="sm:max-w-[640px] max-h-[90vh] overflow-auto">
                <DialogHeader>
                    <DialogTitle>New role</DialogTitle>
                    <DialogDescription>
                        Slugs are stable identifiers stored in users.role and
                        project_members.role. Use lowercase letters and hyphens.
                    </DialogDescription>
                </DialogHeader>
                <form onSubmit={submit} className="space-y-4">
                    <div className="grid grid-cols-2 gap-3">
                        <div className="space-y-1">
                            <Label htmlFor="role-slug">Slug</Label>
                            <Input
                                id="role-slug"
                                value={slug}
                                onChange={(e) => setSlug(e.target.value)}
                                placeholder="oncall"
                                autoFocus
                            />
                        </div>
                        <div className="space-y-1">
                            <Label htmlFor="role-name">Name</Label>
                            <Input
                                id="role-name"
                                value={name}
                                onChange={(e) => setName(e.target.value)}
                                placeholder="On-call Operator"
                            />
                        </div>
                    </div>
                    <div className="space-y-1">
                        <Label htmlFor="role-desc">Description (optional)</Label>
                        <Textarea
                            id="role-desc"
                            rows={2}
                            value={description}
                            onChange={(e) => setDescription(e.target.value)}
                        />
                    </div>
                    <div className="flex gap-4">
                        <label className="inline-flex items-center gap-2 text-sm">
                            <Checkbox
                                checked={isGlobal}
                                onCheckedChange={(v) => setIsGlobal(Boolean(v))}
                            />
                            Assignable globally
                        </label>
                        <label className="inline-flex items-center gap-2 text-sm">
                            <Checkbox
                                checked={isProject}
                                onCheckedChange={(v) => setIsProject(Boolean(v))}
                            />
                            Assignable per-project
                        </label>
                    </div>
                    <PermissionMatrix
                        permissions={permissions}
                        selected={selected}
                        onToggle={(slug) => {
                            const next = new Set(selected);
                            if (next.has(slug)) next.delete(slug);
                            else next.add(slug);
                            setSelected(next);
                        }}
                    />
                    <DialogFooter>
                        <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                            Cancel
                        </Button>
                        <Button
                            type="submit"
                            disabled={
                                submitting ||
                                !slug.trim() ||
                                !name.trim() ||
                                (!isGlobal && !isProject)
                            }
                        >
                            {submitting && <Loader2 className="size-3.5 animate-spin" />}
                            Create role
                        </Button>
                    </DialogFooter>
                </form>
            </DialogContent>
        </Dialog>
    );
}
