import { palette, radius, space } from "../../../layout/theme";
import { Skeleton } from "@/components/ui/skeleton";

// CardGridSkeleton renders six placeholder tiles while the initial
// hosts query is in flight. Six is enough to cover the common
// 3-column layouts at typical viewport widths without flashing a
// near-empty grid.
export default function CardGridSkeleton() {
    const rows = Array.from({ length: 6 });
    return (
        <div
            style={{
                display: "grid",
                gridTemplateColumns: "repeat(auto-fill, minmax(280px, 1fr))",
                gap: space[3],
            }}
        >
            {rows.map((_, i) => (
                <div
                    key={i}
                    data-testid="hosts-card-skeleton"
                    style={{
                        background: palette.surface,
                        border: `1px solid ${palette.border}`,
                        borderRadius: radius.md,
                        padding: space[4],
                        display: "flex",
                        flexDirection: "column",
                        gap: space[3],
                    }}
                >
                    <Skeleton className="h-4 w-3/4" />
                    <Skeleton className="h-3 w-1/2" />
                    <Skeleton className="h-3 w-2/3" />
                    <Skeleton className="h-3 w-1/3" />
                </div>
            ))}
        </div>
    );
}
