// Spec for the plugin-entry section of ActivityBar. The hardcoded
// first-party section is well-exercised by HostView's existing
// integration tests; here we just verify that:
//   - pluginEntries renders one icon per entry, after a divider
//   - clicking a plugin icon fires onSelect with the prefixed activity key
//   - the active-plugin styling is applied to the matching button
//   - empty / undefined pluginEntries → no divider, no extra icons

import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { Cog, Database } from "lucide-react";

import ActivityBar from "./ActivityBar";
import {
    pluginActivityKey,
    type PluginUIEntry,
} from "./plugins/registry";

const NoopComp = () => null;

const ENTRIES: PluginUIEntry[] = [
    {
        pluginID: "com.platypus.sys-systemd-linux",
        title: "Services",
        icon: Cog,
        component: NoopComp,
    },
    {
        pluginID: "com.platypus.sys-pkg-linux",
        title: "Packages",
        icon: Database,
        component: NoopComp,
    },
];

describe("<ActivityBar pluginEntries>", () => {
    it("renders no divider + no plugin icons when pluginEntries is empty", () => {
        render(<ActivityBar active="files" onSelect={() => {}} />);
        expect(
            screen.queryByTestId("host-activity-plugin-divider"),
        ).toBeNull();
        expect(
            screen.queryByTestId(/host-activity-plugin:/),
        ).toBeNull();
    });

    it("renders a divider + one button per plugin entry", () => {
        render(
            <ActivityBar
                active="files"
                onSelect={() => {}}
                pluginEntries={ENTRIES}
            />,
        );
        expect(
            screen.getByTestId("host-activity-plugin-divider"),
        ).toBeInTheDocument();
        expect(
            screen.getByTestId(
                "host-activity-plugin:com.platypus.sys-systemd-linux",
            ),
        ).toBeInTheDocument();
        expect(
            screen.getByTestId(
                "host-activity-plugin:com.platypus.sys-pkg-linux",
            ),
        ).toBeInTheDocument();
    });

    it("clicking a plugin icon fires onSelect with the prefixed key", () => {
        const onSelect = vi.fn();
        render(
            <ActivityBar
                active="files"
                onSelect={onSelect}
                pluginEntries={ENTRIES}
            />,
        );
        fireEvent.click(
            screen.getByTestId(
                "host-activity-plugin:com.platypus.sys-systemd-linux",
            ),
        );
        expect(onSelect).toHaveBeenCalledWith(
            pluginActivityKey("com.platypus.sys-systemd-linux"),
        );
    });

    it("marks the matching button as active via data-active", () => {
        render(
            <ActivityBar
                active={pluginActivityKey("com.platypus.sys-pkg-linux")}
                onSelect={() => {}}
                pluginEntries={ENTRIES}
            />,
        );
        const active = screen.getByTestId(
            "host-activity-plugin:com.platypus.sys-pkg-linux",
        );
        const inactive = screen.getByTestId(
            "host-activity-plugin:com.platypus.sys-systemd-linux",
        );
        expect(active.getAttribute("data-active")).toBe("true");
        expect(inactive.getAttribute("data-active")).toBeNull();
    });
});
