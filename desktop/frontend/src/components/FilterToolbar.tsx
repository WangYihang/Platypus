import { ReactNode } from "react";
import { Search } from "lucide-react";

import { Input } from "@/components/ui/input";
import { palette, space } from "../layout/theme";

import RefreshButton from "./RefreshButton";
import Toolbar from "./Toolbar";

interface SearchSpec {
    value: string;
    onChange: (next: string) => void;
    placeholder?: string;
    /** Optional minWidth override for the input (default 220 px). */
    minWidth?: number;
}

interface Props {
    /** Domain-specific filter chips / dropdowns / toggles. Rendered
     *  on the left, after the search input if one is provided. */
    filters?: ReactNode;
    /** Optional search input. Renders the Search icon + Input pair
     *  every list page used to spell out by hand. */
    search?: SearchSpec;
    /** Result count / pagination summary text rendered before the
     *  right-aligned actions cluster. */
    count?: ReactNode;
    onRefresh?: () => void;
    refreshLoading?: boolean;
    /** Extra right-aligned actions (e.g. New … buttons) rendered
     *  after the refresh button. */
    actions?: ReactNode;
}

// FilterToolbar is the standardised "search + filters · count +
// refresh + actions" row that 8 pages used to spell out individually.
// It composes on top of the existing `<Toolbar>` chrome (hairline,
// padding, left/right split); FilterToolbar handles the inside layout
// and the Search/RefreshButton primitives.
//
// What goes where:
//   left:  search input · filter chips (in this order)
//   right: count · refresh button · custom actions
//
// Pages that need a different shape (e.g. EnrollmentPage's filter
// ToggleGroup as a primary control) keep using `<Toolbar>` directly;
// FilterToolbar covers the common case.
export default function FilterToolbar({
    filters,
    search,
    count,
    onRefresh,
    refreshLoading,
    actions,
}: Props) {
    return (
        <Toolbar
            left={
                <>
                    {search && (
                        <div
                            style={{
                                display: "flex",
                                alignItems: "center",
                                gap: space[2],
                                flex: "1 1 auto",
                                minWidth: 0,
                                maxWidth: 480,
                            }}
                        >
                            <Search
                                className="size-3.5"
                                style={{ color: palette.textMuted, flexShrink: 0 }}
                            />
                            <Input
                                value={search.value}
                                onChange={(e) => search.onChange(e.target.value)}
                                placeholder={search.placeholder ?? "Search…"}
                                className="h-8"
                                style={{ minWidth: search.minWidth ?? 220 }}
                            />
                        </div>
                    )}
                    {filters}
                </>
            }
            right={
                <>
                    {count !== undefined && count !== null ? (
                        <span className="pl-text-secondary">{count}</span>
                    ) : null}
                    {onRefresh && (
                        <RefreshButton loading={refreshLoading} onClick={onRefresh} />
                    )}
                    {actions}
                </>
            }
        />
    );
}
