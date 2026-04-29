import type { CSSProperties, ReactNode } from "react";

import { font } from "../layout/theme";

interface Props {
    // Optional so this component composes with i18next's <Trans
    // components={{ mono: <Mono /> }} />, which injects children at
    // render time. Direct callers always pass them explicitly.
    children?: ReactNode;
    size?: number;
    color?: string;
    style?: CSSProperties;
}

// Mono is a monospace span that uses Geist Mono. Use for IDs, ports,
// host:port endpoints, file paths, fingerprints, and other "code-like"
// values so they line up visually across rows.
export default function Mono({ children, size = 12, color, style }: Props) {
    return (
        <span
            style={{
                fontFamily: font.mono,
                fontSize: size,
                color,
                ...style,
            }}
        >
            {children}
        </span>
    );
}
