import Mono from "../../components/Mono";
import { palette, space } from "../../layout/theme";
import { Checkbox } from "@/components/ui/checkbox";

interface Props {
    label: string;
    options: readonly string[];
    selected: Set<string>;
    onToggle: (s: string) => void;
}

export default function ScopeGroup({ label, options, selected, onToggle }: Props) {
    return (
        <div>
            <div
                style={{
                    fontSize: 11,
                    color: palette.textMuted,
                    marginBottom: space[1],
                    textTransform: "uppercase",
                    letterSpacing: 0.5,
                }}
            >
                {label}
            </div>
            <div
                style={{
                    display: "grid",
                    gridTemplateColumns: "repeat(2, minmax(0, 1fr))",
                    gap: space[2],
                }}
            >
                {options.map((s) => (
                    <label
                        key={s}
                        htmlFor={`scope-${s}`}
                        style={{
                            display: "inline-flex",
                            alignItems: "center",
                            gap: space[2],
                            fontSize: 13,
                            color: palette.textPrimary,
                            cursor: "pointer",
                        }}
                    >
                        <Checkbox
                            id={`scope-${s}`}
                            checked={selected.has(s)}
                            onCheckedChange={() => onToggle(s)}
                        />
                        <Mono size={12}>{s}</Mono>
                    </label>
                ))}
            </div>
        </div>
    );
}
