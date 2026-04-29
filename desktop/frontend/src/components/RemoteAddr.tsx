import { useQuery } from "@tanstack/react-query";

import { lookupIpInfo, RemoteIpInfo } from "../lib/api";
import { qk } from "../lib/queryKeys";
import Mono from "./Mono";

// IpInfo mirrors the Go-side ipinfo.Info struct serialised by the
// API. All geo fields are optional — they only get populated for
// public IPv4 addresses that ip2region recognises.
export type IpInfo = RemoteIpInfo;

// Country-name → ISO emoji flag for the names ip2region emits. The
// dataset uses Chinese country names, so the keys here are the raw
// strings that come out of the search result. Anything not in this
// map renders without a flag — that's intentional, an unknown
// country shouldn't display a wrong one.
const FLAG_BY_COUNTRY: Record<string, string> = {
    "中国": "🇨🇳",
    "香港": "🇭🇰",
    "澳门": "🇲🇴",
    "台湾": "🇹🇼",
    "美国": "🇺🇸",
    "日本": "🇯🇵",
    "韩国": "🇰🇷",
    "新加坡": "🇸🇬",
    "英国": "🇬🇧",
    "法国": "🇫🇷",
    "德国": "🇩🇪",
    "俄罗斯": "🇷🇺",
    "加拿大": "🇨🇦",
    "澳大利亚": "🇦🇺",
    "印度": "🇮🇳",
    "巴西": "🇧🇷",
    "荷兰": "🇳🇱",
    "意大利": "🇮🇹",
    "西班牙": "🇪🇸",
    "瑞士": "🇨🇭",
    "瑞典": "🇸🇪",
    "马来西亚": "🇲🇾",
    "泰国": "🇹🇭",
    "越南": "🇻🇳",
    "印度尼西亚": "🇮🇩",
    "菲律宾": "🇵🇭",
    "土耳其": "🇹🇷",
    "南非": "🇿🇦",
    "阿联酋": "🇦🇪",
    "以色列": "🇮🇱",
};

// Pretty-print location parts skipping empties and dropping the
// repeated city when province == city (common for CN municipalities
// like "北京市 / 北京市"; ip2region collapses those by convention but
// the data isn't always consistent).
function formatLocation(info: IpInfo): string {
    const parts: string[] = [];
    if (info.country) parts.push(info.country);
    if (info.province && info.province !== info.country) parts.push(info.province);
    if (info.city && info.city !== info.province) parts.push(info.city);
    return parts.join(" · ");
}

// Build the hover tooltip from whatever fields are populated. Always
// shows the raw IP first so the operator can copy it without
// expanding anything.
function buildTooltip(addr: string, info?: IpInfo): string {
    const lines: string[] = [addr];
    if (!info) return lines.join("\n");

    if (info.is_loopback) lines.push("Loopback (本机)");
    else if (info.is_private) lines.push("Private / reserved (内网)");

    if (info.version) lines.push(`IPv${info.version}`);

    const loc = formatLocation(info);
    if (loc) lines.push(loc);
    if (info.isp) lines.push(`ISP: ${info.isp}`);

    return lines.join("\n");
}

interface Props {
    addr: string;
    info?: IpInfo | null;
    monoSize?: number;
    // When true and `info` is not provided, fall back to a server
    // lookup via /api/v1/ipinfo. List endpoints already inline
    // *_info on each row (cheap), so this is reserved for one-off
    // sites like InfoTab.public_ip where there's no enclosing list
    // payload.
    fetchInfo?: boolean;
}

// RemoteAddr renders an IP address with optional geo / class
// enrichment. The bare address is always shown in monospace so
// table layouts stay aligned; the flag, badge, and location text
// hang off the side and degrade to nothing when unknown.
//
// Hover surfaces the full breakdown via a native title attribute —
// matches the rest of the codebase's tooltip pattern (StatusBar,
// RoleHelpIcon) instead of dragging in a popover lib.
export default function RemoteAddr({ addr, info, monoSize = 12, fetchInfo }: Props) {
    // Hooks must run unconditionally — gate on `enabled` instead of
    // an early return so the rules-of-hooks linter is happy when the
    // empty-addr branch below short-circuits.
    const lookup = useQuery({
        queryKey: qk.ipInfo(addr),
        queryFn: () => lookupIpInfo(addr),
        enabled: Boolean(fetchInfo && !info && addr),
        staleTime: 5 * 60_000,
        gcTime: 30 * 60_000,
    });

    if (!addr) return <>—</>;

    const resolved = info ?? lookup.data ?? undefined;
    const tooltip = buildTooltip(addr, resolved);
    const flag = resolved?.country ? FLAG_BY_COUNTRY[resolved.country] : undefined;
    const location = resolved ? formatLocation(resolved) : "";

    let badge: string | null = null;
    if (resolved?.is_loopback) badge = "loopback";
    else if (resolved?.is_private) badge = "private";

    return (
        <span
            title={tooltip}
            style={{ display: "inline-flex", alignItems: "center", gap: 6, flexWrap: "wrap" }}
        >
            {flag && <span aria-hidden style={{ fontSize: monoSize + 2 }}>{flag}</span>}
            <Mono size={monoSize}>{addr}</Mono>
            {badge && (
                <span
                    style={{
                        fontSize: monoSize - 1,
                        padding: "1px 6px",
                        borderRadius: 999,
                        background: "var(--color-bg-subtle, #f1f1f1)",
                        color: "var(--color-text-secondary, #666)",
                    }}
                >
                    {badge}
                </span>
            )}
            {!badge && location && (
                <span
                    style={{
                        fontSize: monoSize - 1,
                        color: "var(--color-text-secondary, #666)",
                    }}
                >
                    {location}
                </span>
            )}
        </span>
    );
}
