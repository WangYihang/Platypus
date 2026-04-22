import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

// cn joins conditional className fragments and collapses Tailwind
// conflicts so the last matching utility wins — the canonical
// shadcn/ui helper. Lives at @/lib/cn because shadcn's generator
// writes imports of this exact path.
export function cn(...inputs: ClassValue[]): string {
    return twMerge(clsx(inputs));
}
