import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import PluginSpecEditor, { PluginSpecDraft } from "./PluginSpecEditor";
import { MarketplacePlugin, ProjectSecretRedacted } from "../lib/api";

// PluginSpecEditor combines:
//   1. capability checkboxes (declared by the manifest, opt-in
//      grants — operator-authority-wins)
//   2. schema-driven config form (delegates to SchemaForm)
//   3. SecretPicker for secret-marked fields
//
// The contract is:
//   - The editor is controlled (value + onChange).
//   - capability ticks update spec.granted_capabilities.
//   - config edits update spec.config_overrides as a delta.
//   - When a field is in the manifest's secret_fields, the picker
//     yields a SecretRef shape ({"$secret": "sec_..."}) into the
//     overrides.
//   - Plugins without a config block render only the capability
//     section — the form gracefully no-ops when schema is absent.

function harness({
    plugin,
    secrets,
    initial,
    onCreateSecret,
}: {
    plugin: MarketplacePlugin;
    secrets: ProjectSecretRedacted[];
    initial: PluginSpecDraft;
    onCreateSecret?: () => Promise<ProjectSecretRedacted>;
}) {
    const onChange = vi.fn();
    function Wrapper() {
        const [v, setV] = useState<PluginSpecDraft>(initial);
        return (
            <PluginSpecEditor
                projectID="p1"
                plugin={plugin}
                secrets={secrets}
                value={v}
                onChange={(next) => {
                    setV(next);
                    onChange(next);
                }}
                onCreateSecret={onCreateSecret ?? (async () => ({
                    secret_id: "sec_new",
                    project_id: "p1",
                    name: "x",
                    created_at: "",
                    revoked: false,
                }))}
            />
        );
    }
    render(<Wrapper />);
    return { onChange };
}

const baseManifest: MarketplacePlugin = {
    plugin_id: "com.example.syslog",
    version: "1.0.0",
    name: "Syslog",
    author: "Test",
    license: "Apache-2.0",
    homepage: "",
    description: "",
    latest_version: "1.0.0",
    publisher_key_id: "k",
    wasm_url: "",
    signature_url: "",
    wasm_sha256_hex: "",
    capabilities: ["net.dial", "log"],
    fetched_at_unix: 0,
    config_schema: {
        type: "object",
        properties: {
            destination: {
                type: "string",
                description: "Syslog destination URI",
            },
            auth_token: {
                type: "string",
                description: "Bearer token",
            },
            tls: {
                type: "boolean",
                default: true,
            },
        },
    },
    config_secret_fields: ["/auth_token"],
    config_schema_version: 1,
};

describe("<PluginSpecEditor>", () => {
    it("renders capability checkboxes for the manifest's declared set", () => {
        harness({
            plugin: baseManifest,
            secrets: [],
            initial: { plugin_id: "com.example.syslog" },
        });
        // Each declared family rendered as a row; granted defaults
        // to "off" — operators must explicitly grant.
        expect(screen.getByLabelText(/net\.dial/i)).not.toBeChecked();
        expect(screen.getByLabelText(/^log$/i)).not.toBeChecked();
    });

    it("ticking a capability adds it to granted_capabilities", async () => {
        const user = userEvent.setup();
        const { onChange } = harness({
            plugin: baseManifest,
            secrets: [],
            initial: { plugin_id: "com.example.syslog" },
        });
        await user.click(screen.getByLabelText(/net\.dial/i));
        const last = onChange.mock.calls[onChange.mock.calls.length - 1][0];
        expect(last.granted_capabilities).toEqual(["net.dial"]);
    });

    it("renders a schema-driven config form when the manifest has one", () => {
        harness({
            plugin: baseManifest,
            secrets: [],
            initial: { plugin_id: "com.example.syslog" },
        });
        // Standard fields use plaintext inputs; secret-marked
        // field renders the SecretPicker (the "Select a secret"
        // button trigger).
        expect(screen.getByLabelText(/Syslog destination URI/i)).toBeInTheDocument();
        expect(screen.getByTestId("secret-picker")).toBeInTheDocument();
    });

    it("plugins without config_schema show only the capability section", () => {
        const noConfig: MarketplacePlugin = {
            ...baseManifest,
            config_schema: undefined,
            config_secret_fields: undefined,
            config_schema_version: undefined,
        };
        harness({
            plugin: noConfig,
            secrets: [],
            initial: { plugin_id: "com.example.syslog" },
        });
        // Capability section still renders.
        expect(screen.getByLabelText(/net\.dial/i)).toBeInTheDocument();
        // No config form fields.
        expect(
            screen.queryByLabelText(/Syslog destination/i),
        ).not.toBeInTheDocument();
    });

    it("picking a secret writes a SecretRef into config_overrides", async () => {
        const secrets: ProjectSecretRedacted[] = [
            {
                secret_id: "sec_dd",
                project_id: "p1",
                name: "datadog_api_key",
                created_at: "",
                revoked: false,
            },
        ];
        const user = userEvent.setup();
        const { onChange } = harness({
            plugin: baseManifest,
            secrets,
            initial: { plugin_id: "com.example.syslog" },
        });
        await user.click(screen.getByTestId("secret-picker"));
        await user.click(
            await screen.findByTestId("secret-picker-option-sec_dd"),
        );
        const last = onChange.mock.calls[onChange.mock.calls.length - 1][0];
        // SecretRef shape — the wire form the resolver expects.
        expect(last.config_overrides).toEqual({
            auth_token: { $secret: "sec_dd" },
        });
        // schema_version is pinned from the manifest so the
        // server-side validator can refuse stale configs.
        expect(last.schema_version).toBe(1);
    });
});
