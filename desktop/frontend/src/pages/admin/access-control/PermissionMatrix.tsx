import { useMemo } from "react";

import Mono from "../../../components/Mono";
import { palette, space } from "../../../layout/theme";
import { type RBACPermission } from "../../../lib/api";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";

interface Props {
    permissions: RBACPermission[];
    selected: Set<string>;
    onToggle: (slug: string) => void;
}

// Resource-grouped checkbox grid for picking a role's permission set.
// Used by both Create and Edit role dialogs.
export default function PermissionMatrix({ permissions, selected, onToggle }: Props) {
    const grouped = useMemo(() => {
        const m = new Map<string, RBACPermission[]>();
        for (const p of permissions) {
            const list = m.get(p.resource) ?? [];
            list.push(p);
            m.set(p.resource, list);
        }
        return Array.from(m.entries()).sort((a, b) => a[0].localeCompare(b[0]));
    }, [permissions]);

    return (
        <div className="space-y-3">
            <Label>Permissions</Label>
            {grouped.map(([resource, perms]) => (
                <div key={resource}>
                    <div
                        style={{
                            fontSize: 12,
                            color: palette.textMuted,
                            marginBottom: space[1],
                            textTransform: "uppercase",
                            letterSpacing: 0.5,
                        }}
                    >
                        {resource}
                    </div>
                    <div
                        style={{
                            display: "grid",
                            gridTemplateColumns: "repeat(2, minmax(0, 1fr))",
                            gap: space[2],
                        }}
                    >
                        {perms.map((p) => (
                            <label
                                key={p.slug}
                                htmlFor={`perm-${p.slug}`}
                                style={{
                                    display: "flex",
                                    alignItems: "flex-start",
                                    gap: space[2],
                                    fontSize: 13,
                                    cursor: "pointer",
                                }}
                            >
                                <Checkbox
                                    id={`perm-${p.slug}`}
                                    checked={selected.has(p.slug)}
                                    onCheckedChange={() => onToggle(p.slug)}
                                />
                                <div>
                                    <Mono size={12}>{p.slug}</Mono>
                                    <div
                                        style={{
                                            color: palette.textMuted,
                                            fontSize: 11,
                                            marginTop: 2,
                                        }}
                                    >
                                        {p.description}
                                    </div>
                                </div>
                            </label>
                        ))}
                    </div>
                </div>
            ))}
        </div>
    );
}
