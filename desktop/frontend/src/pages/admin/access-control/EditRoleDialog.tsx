import { useEffect, useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";

import { humanizeError } from "../../../lib/humanizeError";
import {
    type RBACPermission,
    type RBACRole,
    updateRBACRole,
} from "../../../lib/api";
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
import { Textarea } from "@/components/ui/textarea";

import PermissionMatrix from "./PermissionMatrix";

interface Props {
    role: RBACRole | null;
    onOpenChange: (o: boolean) => void;
    permissions: RBACPermission[];
}

export default function EditRoleDialog({ role, onOpenChange, permissions }: Props) {
    const [name, setName] = useState("");
    const [description, setDescription] = useState("");
    const [selected, setSelected] = useState<Set<string>>(new Set());
    const [submitting, setSubmitting] = useState(false);

    useEffect(() => {
        if (role) {
            setName(role.name);
            setDescription(role.description ?? "");
            setSelected(new Set(role.permissions));
            setSubmitting(false);
        }
    }, [role]);

    if (!role) return null;

    async function submit(e: React.FormEvent) {
        e.preventDefault();
        if (!role) return;
        setSubmitting(true);
        try {
            await updateRBACRole(role.slug, {
                name: role.is_builtin ? undefined : name,
                description,
                permissions: Array.from(selected),
            });
            toast.success("Role updated");
            onOpenChange(false);
        } catch (e) {
            toast.error(`Couldn't update: ${humanizeError(e)}`);
        } finally {
            setSubmitting(false);
        }
    }

    return (
        <Dialog open onOpenChange={onOpenChange}>
            <DialogContent className="sm:max-w-[640px] max-h-[90vh] overflow-auto">
                <DialogHeader>
                    <DialogTitle>
                        {role.is_builtin ? "Edit builtin role" : "Edit role"}: {role.name}
                    </DialogTitle>
                    <DialogDescription>
                        {role.is_builtin
                            ? "Builtin role names and slot affinity are locked. The permission set can still be tuned."
                            : "Name and permission set are editable."}
                    </DialogDescription>
                </DialogHeader>
                <form onSubmit={submit} className="space-y-4">
                    <div className="space-y-1">
                        <Label htmlFor="edit-role-name">Name</Label>
                        <Input
                            id="edit-role-name"
                            value={name}
                            onChange={(e) => setName(e.target.value)}
                            disabled={role.is_builtin}
                        />
                    </div>
                    <div className="space-y-1">
                        <Label htmlFor="edit-role-desc">Description</Label>
                        <Textarea
                            id="edit-role-desc"
                            rows={2}
                            value={description}
                            onChange={(e) => setDescription(e.target.value)}
                        />
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
                        <Button type="submit" disabled={submitting}>
                            {submitting && <Loader2 className="size-3.5 animate-spin" />}
                            Save
                        </Button>
                    </DialogFooter>
                </form>
            </DialogContent>
        </Dialog>
    );
}
