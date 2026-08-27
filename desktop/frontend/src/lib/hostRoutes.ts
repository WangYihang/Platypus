// HOST_SUBVIEWS are the static route segments that sit alongside
// /hosts/:hostId — fleet views that are not a specific host.
//
// They live here, in one place, because three unrelated files have to
// agree on them and nothing else forces them to:
//
//   · routes.tsx        mounts each as a child of the hosts route
//   · HostsShell        renders the List / Topology / Archived switcher
//   · useRouteHostId    must NOT report them as host ids
//
// The third is the one that bites. /hosts/topology matches the
// /hosts/:hostId pattern, so a missing entry means the terminal drawer
// scopes its shells to a "host" called topology or archived — a host
// that can never exist, on a page with no terminals.
export const HOST_SUBVIEWS = ["topology", "archived"] as const;

export type HostSubview = (typeof HOST_SUBVIEWS)[number];
