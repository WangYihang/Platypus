import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import SecretPicker from "./SecretPicker";
import { ProjectSecretRedacted } from "../lib/api";

// SecretPicker contract:
//   1. Renders the operator's saved-secret list as a dropdown so
//      plugin config fields marked sensitive can resolve to a
//      reference instead of plaintext.
//   2. Picking a secret yields its id ("sec_<hex>") to the caller —
//      the form layer wraps it as {"$secret":"sec_<hex>"} before
//      sending to the server.
//   3. "Create new" is a discoverable inline path so operators
//      don't have to leave the wizard to set up a one-off secret.
//   4. The currently-selected secret is rendered by NAME (not id)
//      so the form stays scannable; the id is only the wire form.
//   5. Revoked secrets do NOT appear in the picker — referencing
//      a revoked secret would produce a stale config that
//      installs-as-fail.

const fixtures: ProjectSecretRedacted[] = [
    {
        secret_id: "sec_aaa",
        project_id: "p1",
        name: "datadog_api_key",
        created_at: "2026-05-01T00:00:00Z",
        revoked: false,
    },
    {
        secret_id: "sec_bbb",
        project_id: "p1",
        name: "old_revoked_key",
        created_at: "2025-12-01T00:00:00Z",
        revoked: true,
        revoked_at: "2026-04-01T00:00:00Z",
    },
];

describe("<SecretPicker>", () => {
    it("lists active secrets by name when opened", async () => {
        const user = userEvent.setup();
        render(
            <SecretPicker
                projectID="p1"
                secrets={fixtures}
                value={undefined}
                onSelect={vi.fn()}
                onCreate={vi.fn()}
            />,
        );
        await user.click(screen.getByTestId("secret-picker"));
        // Active secret rendered inside the opened dropdown.
        expect(
            await screen.findByTestId("secret-picker-option-sec_aaa"),
        ).toBeInTheDocument();
        // Revoked secret is hidden — referencing one would fail the
        // server-side resolver. Better to surface as absent.
        expect(
            screen.queryByTestId("secret-picker-option-sec_bbb"),
        ).not.toBeInTheDocument();
    });

    it("calls onSelect with the secret_id when one is picked", async () => {
        const onSelect = vi.fn();
        const user = userEvent.setup();
        render(
            <SecretPicker
                projectID="p1"
                secrets={fixtures}
                value={undefined}
                onSelect={onSelect}
                onCreate={vi.fn()}
            />,
        );
        // Click the trigger to expose the option list, then click
        // the "datadog_api_key" option.
        await user.click(screen.getByTestId("secret-picker"));
        await user.click(
            await screen.findByTestId("secret-picker-option-sec_aaa"),
        );
        await waitFor(() => {
            expect(onSelect).toHaveBeenCalledWith("sec_aaa");
        });
    });

    it("renders the selected secret name when value is set", () => {
        render(
            <SecretPicker
                projectID="p1"
                secrets={fixtures}
                value="sec_aaa"
                onSelect={vi.fn()}
                onCreate={vi.fn()}
            />,
        );
        // Trigger label shows the human-readable name even when
        // closed — the operator scanning the form sees what's
        // selected without re-opening the dropdown.
        expect(
            screen.getByTestId("secret-picker"),
        ).toHaveTextContent(/datadog_api_key/i);
    });

    it("inline-create yields a new secret_id via onCreate", async () => {
        // onCreate captures (name, value) → returns the new id
        // (mimicking the API call). The picker doesn't itself talk
        // to the server — keeps it focused on UI.
        const onCreate = vi.fn().mockResolvedValue({
            secret_id: "sec_new",
            project_id: "p1",
            name: "newkey",
            created_at: "2026-05-07T00:00:00Z",
            revoked: false,
        });
        const onSelect = vi.fn();
        const user = userEvent.setup();
        render(
            <SecretPicker
                projectID="p1"
                secrets={fixtures}
                value={undefined}
                onSelect={onSelect}
                onCreate={onCreate}
            />,
        );
        await user.click(screen.getByTestId("secret-picker"));
        await user.click(await screen.findByTestId("secret-picker-create"));

        const nameInput = await screen.findByTestId("secret-picker-new-name");
        const valueInput = screen.getByTestId("secret-picker-new-value");
        await user.type(nameInput, "newkey");
        await user.type(valueInput, "supersecret");
        fireEvent.click(screen.getByTestId("secret-picker-new-confirm"));

        await waitFor(() => {
            expect(onCreate).toHaveBeenCalledWith({
                name: "newkey",
                value: "supersecret",
            });
        });
        await waitFor(() => {
            expect(onSelect).toHaveBeenCalledWith("sec_new");
        });
    });

    it("shows a clear-selection control when value is set", async () => {
        const onSelect = vi.fn();
        const user = userEvent.setup();
        render(
            <SecretPicker
                projectID="p1"
                secrets={fixtures}
                value="sec_aaa"
                onSelect={onSelect}
                onCreate={vi.fn()}
            />,
        );
        await user.click(screen.getByTestId("secret-picker-clear"));
        // Clearing yields an empty selection — the form layer
        // unsets the field's secret-ref and lets the operator
        // type plaintext (or skip the field).
        expect(onSelect).toHaveBeenCalledWith("");
    });
});
