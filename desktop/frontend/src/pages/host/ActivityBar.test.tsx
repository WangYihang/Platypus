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

    // ---------------- "new" indicator ----------------

    it("renders a new-dot on plugin entries in newPluginIDs", () => {
        render(
            <ActivityBar
                active="files"
                onSelect={() => {}}
                pluginEntries={ENTRIES}
                newPluginIDs={new Set(["com.platypus.sys-pkg-linux"])}
            />,
        );
        // sys-pkg-linux is new → dot rendered.
        expect(
            screen.getByTestId(
                "host-activity-new-com.platypus.sys-pkg-linux",
            ),
        ).toBeInTheDocument();
        // sys-systemd-linux isn't in the new set → no dot.
        expect(
            screen.queryByTestId(
                "host-activity-new-com.platypus.sys-systemd-linux",
            ),
        ).toBeNull();
    });

    it("data-new attribute marks the button + tooltip says '(new)'", () => {
        render(
            <ActivityBar
                active="files"
                onSelect={() => {}}
                pluginEntries={ENTRIES}
                newPluginIDs={new Set(["com.platypus.sys-pkg-linux"])}
            />,
        );
        const btn = screen.getByTestId(
            "host-activity-plugin:com.platypus.sys-pkg-linux",
        );
        expect(btn.getAttribute("data-new")).toBe("true");
        expect(btn.getAttribute("title")).toMatch(/\(new\)/);
    });

    it("no new dots when newPluginIDs is undefined or empty", () => {
        render(
            <ActivityBar
                active="files"
                onSelect={() => {}}
                pluginEntries={ENTRIES}
            />,
        );
        expect(
            screen.queryByTestId(/host-activity-new-/),
        ).toBeNull();
    });

    // ---------------- alwaysVisible dimming (Q1) ----------------

    it("alwaysVisible entry with required plugin missing renders dimmed + needs-install dot", () => {
        const ENTRY_WITH_REQ: PluginUIEntry = {
            pluginID: "com.platypus.sys-files-read",
            requiredPluginIDs: [
                "com.platypus.sys-files-read",
                "com.platypus.sys-files-write",
            ],
            alwaysVisible: true,
            title: "Files",
            icon: Cog,
            component: NoopComp,
        };
        render(
            <ActivityBar
                active="files"
                onSelect={() => {}}
                pluginEntries={[ENTRY_WITH_REQ]}
                installedPluginIDs={new Set(["com.platypus.sys-files-read"])}
            />,
        );
        const btn = screen.getByTestId(
            "host-activity-plugin:com.platypus.sys-files-read",
        );
        expect(btn.getAttribute("data-needs-install")).toBe("true");
        expect(btn.getAttribute("title")).toMatch(/needs plugin/);
        expect(
            screen.getByTestId(
                "host-activity-needs-install-com.platypus.sys-files-read",
            ),
        ).toBeInTheDocument();
    });

    it("alwaysVisible entry with all required plugins installed: no dimming, no needs-install dot", () => {
        const ENTRY_WITH_REQ: PluginUIEntry = {
            pluginID: "com.platypus.sys-files-read",
            requiredPluginIDs: [
                "com.platypus.sys-files-read",
                "com.platypus.sys-files-write",
            ],
            alwaysVisible: true,
            title: "Files",
            icon: Cog,
            component: NoopComp,
        };
        render(
            <ActivityBar
                active="files"
                onSelect={() => {}}
                pluginEntries={[ENTRY_WITH_REQ]}
                installedPluginIDs={
                    new Set([
                        "com.platypus.sys-files-read",
                        "com.platypus.sys-files-write",
                    ])
                }
            />,
        );
        const btn = screen.getByTestId(
            "host-activity-plugin:com.platypus.sys-files-read",
        );
        expect(btn.getAttribute("data-needs-install")).toBeNull();
    });

    it("entry.activityKey overrides the default plugin: prefix in URL routing", () => {
        // Use a custom slug that doesn't collide with the hardcoded
        // ACTIVITIES (the migration to one-uniform-registry happens
        // in a follow-up commit). The point of the test is the
        // override mechanism, not the specific slug value.
        const ENTRY_LEGACY: PluginUIEntry = {
            pluginID: "com.example.legacy",
            activityKey: "legacy-custom-slug",
            alwaysVisible: true,
            title: "Legacy",
            icon: Cog,
            component: NoopComp,
        };
        render(
            <ActivityBar
                active="legacy-custom-slug"
                onSelect={() => {}}
                pluginEntries={[ENTRY_LEGACY]}
            />,
        );
        // URL slug uses the override, not plugin:<id>.
        expect(
            screen.getByTestId("host-activity-legacy-custom-slug"),
        ).toBeInTheDocument();
        expect(
            screen.queryByTestId("host-activity-plugin:com.example.legacy"),
        ).toBeNull();
    });
});
