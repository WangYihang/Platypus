import type { ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";

import { lookupIpInfo, RemoteIpInfo } from "../lib/api";
import { qk } from "../lib/queryKeys";
import Mono from "./Mono";
import { Tooltip, TooltipContent, TooltipTrigger } from "./ui/tooltip";

// IpInfo mirrors the Go-side ipinfo.Info struct serialised by the
// API. All geo fields are optional — they only get populated for
// public addresses that ip2region recognises.
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
// table layouts stay aligned; the flag, badge, and inline location
// text hang off the side and degrade to nothing when unknown.
//
// Hover surfaces the full breakdown via a Radix tooltip — a styled
// card with sectioned IP / family / class / location / ISP rows
// instead of the browser's native single-line title attribute. The
// global TooltipProvider in main.tsx sets the open delay (200ms).
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
    const flag = resolved?.country ? FLAG_BY_COUNTRY[resolved.country] : undefined;
    const location = resolved ? formatLocation(resolved) : "";

    let badge: string | null = null;
    if (resolved?.is_loopback) badge = "loopback";
    else if (resolved?.is_private) badge = "private";

    const trigger = (
        <span
            // Inline-flex so the trigger keeps Mono baseline alignment
            // inside table cells and DataList rows; the Radix tooltip
            // primitive forwards refs through `asChild` so the span is
            // the actual hover target.
            style={{
                display: "inline-flex",
                alignItems: "center",
                gap: 6,
                flexWrap: "wrap",
                cursor: "default",
            }}
        >
            {flag && (
                <span aria-hidden style={{ fontSize: monoSize + 2 }}>
                    {flag}
                </span>
            )}
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

    return (
        <Tooltip>
            <TooltipTrigger asChild>{trigger}</TooltipTrigger>
            <TooltipContent
                side="top"
                align="start"
                // Override the default tooltip styling: this content
                // is a multi-line card, not a one-liner label.
                className="max-w-[320px] rounded-md border bg-popover px-3 py-2 text-popover-foreground shadow-md"
            >
                <RemoteAddrTooltipBody addr={addr} info={resolved} flag={flag} />
            </TooltipContent>
        </Tooltip>
    );
}

function RemoteAddrTooltipBody({
    addr,
    info,
    flag,
}: {
    addr: string;
    info?: IpInfo;
    flag?: string;
}) {
    const location = info ? formatLocation(info) : "";
    const classLabel = info?.is_loopback
        ? "Loopback (本机)"
        : info?.is_private
          ? "Private / reserved (内网)"
          : "";
    return (
        <div className="flex flex-col gap-1.5 text-xs">
            <div className="flex items-center gap-1.5 font-mono text-[12px] break-all">
                {flag && <span aria-hidden>{flag}</span>}
                <span>{addr}</span>
            </div>
            {(info?.version || classLabel) && (
                <div className="flex flex-wrap items-center gap-1.5">
                    {info?.version && <Pill>IPv{info.version}</Pill>}
                    {classLabel && <Pill>{classLabel}</Pill>}
                </div>
            )}
            {(location || info?.isp) && (
                <div className="flex flex-col gap-0.5 text-text-secondary">
                    {location && <div>{location}</div>}
                    {info?.isp && (
                        <div>
                            <span className="text-text-muted">ISP </span>
                            {info.isp}
                        </div>
                    )}
                </div>
            )}
            {!info && (
                <div className="text-text-muted">No enrichment data available</div>
            )}
        </div>
    );
}

function Pill({ children }: { children: ReactNode }) {
    return (
        <span className="rounded-full bg-muted px-1.5 py-0.5 text-[11px] text-muted-foreground">
            {children}
        </span>
    );
}
