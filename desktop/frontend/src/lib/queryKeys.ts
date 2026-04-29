// queryKeys is the centralised registry of every react-query
// `queryKey` shape we use. Pages compose keys from `qk.X(args)` so
// invalidations after mutations (`queryClient.invalidateQueries({
// queryKey: qk.X(args) })`) match exactly the same tuple shape that
// the original `useQuery` produced.
//
// Why a flat object instead of nested factories? It keeps the
// keyspace discoverable in a single grep, and the `as const` tuples
// give react-query proper structural-equality matching without
// hand-rolling a comparator.

import type { ListActivitiesOpts } from "./api";

export const qk = {
    // --- Hosts / sessions / processes -----------------------------
    hosts: (projectId: string) =>
        ["hosts", projectId] as const,
    host: (projectId: string, hostId: string) =>
        ["host", projectId, hostId] as const,
    hostSysInfo: (projectId: string, hostId: string) =>
        ["hostSysInfo", projectId, hostId] as const,
    hostSessions: (projectId: string, hostId: string) =>
        ["hostSessions", projectId, hostId] as const,
    hostProcesses: (projectId: string, hostId: string) =>
        ["hostProcesses", projectId, hostId] as const,
    pendingHosts: (projectId: string) =>
        ["pendingHosts", projectId] as const,
    pendingHostsCount: (projectId: string) =>
        ["pendingHostsCount", projectId] as const,

    // --- Project-scoped lists -------------------------------------
    activities: (projectId: string, opts: ListActivitiesOpts) =>
        ["activities", projectId, opts] as const,
    members: (projectId: string) => ["members", projectId] as const,
    enrollment: (projectId: string) =>
        ["enrollment", projectId] as const,

    // --- Server / project lookups --------------------------------
    projects: () => ["projects"] as const,
    project: (slug: string) => ["project", slug] as const,
    serverInfo: () => ["serverInfo"] as const,

    // --- Admin (server-wide) -------------------------------------
    adminUsers: () => ["adminUsers"] as const,
    adminRoles: () => ["adminRoles"] as const,
    adminPermissions: () => ["adminPermissions"] as const,
    adminSettings: () => ["adminSettings"] as const,

    // --- Account (current user) ----------------------------------
    accountTokens: () => ["accountTokens"] as const,
} as const;
