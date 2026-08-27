import { useQuery } from "@tanstack/react-query";

import { type Host, listHosts } from "../api";
import { qk } from "../queryKeys";

/**
 * The project's host list.
 *
 * This exact query — `qk.hosts(id)` paired with `() => listHosts(id)` —
 * was written out in seven components, and two more (CommandPalette,
 * TerminalDrawer) hand-rolled it as an effect with a cancelled flag and
 * a catch that swallowed the failure into an empty list. Nine call
 * sites, one fetch.
 *
 * Repeating a query declaration is not free: the key and the function
 * have to agree for the cache to work at all, and nothing checks that
 * they do. A component that keys on qk.hosts(id) but fetches something
 * slightly different, or vice versa, gets a cache that silently serves
 * the wrong rows.
 */
export function useProjectHosts(
    projectID: string | undefined,
    opts: { enabled?: boolean; refetchInterval?: number } = {},
) {
    const { enabled = true, refetchInterval } = opts;
    return useQuery({
        // The key still says "" when there is no project so the shape
        // stays stable, but `enabled` keeps the fetch from running.
        queryKey: qk.hosts(projectID ?? ""),
        queryFn: () => listHosts(projectID as string),
        enabled: enabled && !!projectID,
        refetchInterval,
    });
}

/** The list, defaulted — for the many call sites that only want rows. */
export function useProjectHostList(
    projectID: string | undefined,
    opts: { enabled?: boolean; refetchInterval?: number } = {},
): Host[] {
    return useProjectHosts(projectID, opts).data ?? [];
}
