import { useState } from "react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { cn } from "@/lib/cn";

interface Props {
    label: string;
    options: string[];
    selected: string[];
    onChange: (next: string[]) => void;
}

// Minimal multi-select built from Popover + Checkbox. Used for the
// ~10-option category filter; pulling in cmdk + Command just for that
// would be overkill.
export default function MultiSelectPopover({
    label,
    options,
    selected,
    onChange,
}: Props) {
    const [open, setOpen] = useState(false);
    const summary =
        selected.length === 0
            ? label
            : selected.length === 1
              ? selected[0]
              : `${label} · ${selected.length}`;

    function toggle(opt: string, next: boolean) {
        if (next) onChange([...selected, opt]);
        else onChange(selected.filter((x) => x !== opt));
    }

    return (
        <Popover open={open} onOpenChange={setOpen}>
            <PopoverTrigger asChild>
                <Button
                    variant="outline"
                    size="sm"
                    className={cn("min-w-[160px] justify-between", {
                        "text-text-muted": selected.length === 0,
                    })}
                >
                    {summary}
                </Button>
            </PopoverTrigger>
            <PopoverContent align="start" className="w-[220px] p-1">
                {options.map((opt) => {
                    const checked = selected.includes(opt);
                    return (
                        <label
                            key={opt}
                            className="flex cursor-pointer items-center gap-2 rounded-sm px-2 py-1.5 text-sm hover:bg-accent"
                        >
                            <Checkbox
                                checked={checked}
                                onCheckedChange={(v) => toggle(opt, v === true)}
                            />
                            <span>{opt}</span>
                        </label>
                    );
                })}
                {selected.length > 0 && (
                    <>
                        <div className="my-1 h-px bg-border" />
                        <button
                            type="button"
                            onClick={() => onChange([])}
                            className="flex w-full items-center justify-center rounded-sm px-2 py-1.5 text-xs text-text-muted hover:bg-accent hover:text-text-primary"
                        >
                            Clear
                        </button>
                    </>
                )}
            </PopoverContent>
        </Popover>
    );
}
