import { useMatch } from "react-router-dom";

import { HOST_SUBVIEWS } from "@/lib/hostRoutes";

// useRouteHostId reads the :hostId segment of a host-detail URL without
// depending on where the caller sits in the route tree.
//
// useParams only exposes segments matched at or above the calling
// component, which is why the terminal drawer used to work only while
// it was mounted inside HostsShell. The drawer now lives in ShellChrome
// so its <Terminal> children — and their WebSockets — survive
// navigating off /hosts; from up there useParams sees no :hostId at
// all. useMatch resolves the pattern against the whole location, so the
// answer is the same at any depth.
//
// Returns undefined on every route that is not a host detail page,
// which is exactly the signal the drawer uses to hide itself.
export function useRouteHostId(): string | undefined {
    const withTab = useMatch("/projects/:projectSlug/hosts/:hostId/:tab");
    // /hosts/:hostId redirects to /files, but the redirect renders for
    // a frame and topology/index must not match — both are handled by
    // the explicit patterns rather than a wildcard.
    const bare = useMatch("/projects/:projectSlug/hosts/:hostId");

    const hostId = withTab?.params.hostId ?? bare?.params.hostId;

    if (!hostId || RESERVED_HOST_SEGMENTS.has(hostId)) return undefined;
    return hostId;
}

// The fleet sub-views match the bare /hosts/:hostId pattern above, so
// without this each would be reported as a host whose shells could
// never exist. Derived from the shared list rather than restated, so
// adding a sub-view cannot forget this file.
const RESERVED_HOST_SEGMENTS: ReadonlySet<string> = new Set(HOST_SUBVIEWS);
