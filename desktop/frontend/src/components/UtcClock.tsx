import { useEffect, useState } from "react";

import Mono from "./Mono";
import { palette } from "../layout/theme";

// UtcClock renders an HH:MM:SS clock in UTC, ticking at 1 Hz. The
// mockup pins it in the StatusBar's right zone — having UTC on
// screen everywhere keeps incident timelines (logs, audit rows, etc.
// recorded in UTC) easy to correlate without doing the timezone
// math in the operator's head.
//
// The internal interval is suspended to a re-render every second
// rather than every animation frame; the value formats stably to
// the second, finer-grained ticks would just be wasted work.
function pad2(n: number): string {
    return n < 10 ? `0${n}` : String(n);
}

export default function UtcClock() {
    const [now, setNow] = useState(() => new Date());
    useEffect(() => {
        const id = window.setInterval(() => setNow(new Date()), 1000);
        return () => window.clearInterval(id);
    }, []);
    const hh = pad2(now.getUTCHours());
    const mm = pad2(now.getUTCMinutes());
    const ss = pad2(now.getUTCSeconds());
    return (
        <span
            data-testid="status-bar-utc-clock"
            title={`UTC ${now.toUTCString()}`}
            style={{ display: "inline-flex", alignItems: "center", gap: 4 }}
        >
            <span style={{ color: palette.textMuted }}>UTC</span>
            <Mono size={11} color={palette.textPrimary}>{`${hh}:${mm}:${ss}`}</Mono>
        </span>
    );
}
