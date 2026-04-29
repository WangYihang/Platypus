// PAT scopes are fetched from /api/v1/account/permissions — the caller's
// effective role permissions, post-RBAC. Group by the resource prefix in
// the slug (hosts:* / files:* / etc.) so a long list stays readable.
export function groupScopesByResource(scopes: string[]): Array<[string, string[]]> {
    const m = new Map<string, string[]>();
    for (const s of scopes) {
        const colon = s.indexOf(":");
        const resource = colon > 0 ? s.slice(0, colon) : "other";
        const list = m.get(resource) ?? [];
        list.push(s);
        m.set(resource, list);
    }
    return Array.from(m.entries()).sort((a, b) => a[0].localeCompare(b[0]));
}
