import { ReactElement, useEffect, useState } from "react";
import { ResponsiveContainer } from "recharts";

interface Props {
    children: ReactElement;
    height?: number | string;
    minHeight?: number;
}

/**
 * ChartContainer is a wrapper around Recharts' ResponsiveContainer that
 * resolves the "The width(-1) and height(-1) of chart should be greater than 0"
 * warning.
 *
 * It prevents rendering the chart until the component has mounted and
 * the browser has had a chance to perform the initial layout, ensuring
 * that width and height are correctly measured by ResizeObserver.
 */
export default function ChartContainer({ children, height = "100%", minHeight = 0 }: Props) {
    const [isReady, setIsMounted] = useState(false);

    useEffect(() => {
        setIsMounted(true);
    }, []);

    if (!isReady) {
        return <div style={{ width: "100%", height, minHeight }} />;
    }

    return (
        <ResponsiveContainer width="100%" height={height} debounce={50}>
            {children}
        </ResponsiveContainer>
    );
}
