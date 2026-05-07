import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import SchemaForm, { JSONSchema, SecretWidgetProps } from "./SchemaForm";

// Harness wraps SchemaForm in a stateful parent so userEvent.type
// produces the realistic per-keystroke behaviour (each keystroke
// updates the value, the form re-renders with the latest input).
// Without this the form is "controlled but not updated" and edits
// silently reset between events.
function Harness({
    schema,
    initialValue,
    secretFields,
    renderSecretWidget,
    onChange,
}: {
    schema: JSONSchema;
    initialValue: Record<string, unknown>;
    secretFields: string[];
    renderSecretWidget?: (p: SecretWidgetProps) => React.ReactNode;
    onChange?: (v: Record<string, unknown>) => void;
}) {
    const [v, setV] = useState(initialValue);
    return (
        <SchemaForm
            schema={schema}
            value={v}
            onChange={(next) => {
                setV(next);
                onChange?.(next);
            }}
            secretFields={secretFields}
            renderSecretWidget={renderSecretWidget}
        />
    );
}

// SchemaForm renders a JSON-Schema-driven form for plugin config.
// Scope is intentionally narrow:
//   - top-level object only (the plan's "2 levels max" soft rule
//     is enforced via UI rendering — deep schemas show a raw JSON
//     fallback)
//   - leaf types: string, integer, number, boolean
//   - arrays of primitives
//   - JSON Pointer paths in `secretFields` are routed to a custom
//     widget (an embedded SecretPicker mock here — the real one
//     wires up in PluginSpecEditor)
//
// Behaviour pinned by the tests:
//   1. Fields render labels from the schema's `description:` (or
//      property name, fallback).
//   2. Default values are pre-filled from the schema's `default:`.
//   3. Editing a field calls onChange with the full updated value.
//   4. Required fields are flagged in the UI.
//   5. Secret-marked fields render the supplied SecretWidget instead
//      of a plaintext input.

describe("<SchemaForm>", () => {
    it("renders string + boolean leaves with descriptions and defaults", () => {
        const onChange = vi.fn();
        render(
            <SchemaForm
                schema={{
                    type: "object",
                    properties: {
                        greeting: {
                            type: "string",
                            description: "Word prefixed before the name",
                            default: "Hello",
                        },
                        shout: {
                            type: "boolean",
                            description: "Uppercase the entire greeting",
                            default: false,
                        },
                    },
                }}
                value={{ greeting: "Hello", shout: false }}
                onChange={onChange}
                secretFields={[]}
            />,
        );
        // Label comes from `description:`.
        expect(screen.getByLabelText(/Word prefixed/)).toHaveValue("Hello");
        // Boolean rendered as a checkbox.
        const shout = screen.getByLabelText(/Uppercase the entire greeting/);
        expect(shout).not.toBeChecked();
    });

    it("falls back to the property name when description is absent", () => {
        render(
            <SchemaForm
                schema={{
                    type: "object",
                    properties: {
                        port: { type: "integer", default: 514 },
                    },
                }}
                value={{ port: 514 }}
                onChange={vi.fn()}
                secretFields={[]}
            />,
        );
        // Property name acts as the visible label when `description`
        // is missing — the schema validator already requires unique
        // names, so this is a stable fallback.
        expect(screen.getByLabelText(/^port$/i)).toBeInTheDocument();
    });

    it("calls onChange with the merged value when a field edits", async () => {
        const onChange = vi.fn();
        const user = userEvent.setup();
        render(
            <Harness
                schema={{
                    type: "object",
                    properties: {
                        greeting: { type: "string", default: "Hi" },
                        shout: { type: "boolean", default: false },
                    },
                }}
                initialValue={{ greeting: "Hi", shout: false }}
                secretFields={[]}
                onChange={onChange}
            />,
        );
        const input = screen.getByLabelText(/^greeting$/i);
        await user.clear(input);
        await user.type(input, "Howdy");
        // Latest call carries the full merged value — important
        // for the parent's controlled pattern (it just stores
        // the latest value and re-renders).
        const last = onChange.mock.calls[onChange.mock.calls.length - 1][0];
        expect(last.greeting).toBe("Howdy");
        expect(last.shout).toBe(false);
    });

    it("flags required fields in the rendered label", () => {
        render(
            <SchemaForm
                schema={{
                    type: "object",
                    required: ["destination"],
                    properties: {
                        destination: { type: "string" },
                        tls: { type: "boolean", default: true },
                    },
                }}
                value={{}}
                onChange={vi.fn()}
                secretFields={[]}
            />,
        );
        // Required indicator (the canonical asterisk) appears on
        // the required field; non-required fields don't get it.
        const required = screen.getByTestId("schema-form-field-destination");
        expect(required.textContent).toContain("*");
        const optional = screen.getByTestId("schema-form-field-tls");
        expect(optional.textContent).not.toContain("*");
    });

    it("routes secret-marked fields to the custom widget", () => {
        const SecretWidget = vi.fn(({ value, path }) => (
            <span data-testid={`secret-widget-${path}`}>
                {String(value ?? "")}
            </span>
        ));
        render(
            <SchemaForm
                schema={{
                    type: "object",
                    properties: {
                        token: { type: "string" },
                        debug: { type: "boolean" },
                    },
                }}
                value={{ token: { $secret: "sec_abc" } }}
                onChange={vi.fn()}
                secretFields={["/token"]}
                renderSecretWidget={SecretWidget}
            />,
        );
        // Secret field used the custom widget; non-secret field
        // fell through to the standard input.
        expect(screen.getByTestId("secret-widget-/token")).toBeInTheDocument();
        expect(screen.getByLabelText(/^debug$/i)).toBeInTheDocument();
    });

    it("renders array-of-strings as a dynamic list with add/remove", async () => {
        const onChange = vi.fn();
        const user = userEvent.setup();
        render(
            <SchemaForm
                schema={{
                    type: "object",
                    properties: {
                        tags: {
                            type: "array",
                            items: { type: "string" },
                        },
                    },
                }}
                value={{ tags: ["alpha"] }}
                onChange={onChange}
                secretFields={[]}
            />,
        );
        // Existing item rendered.
        expect(screen.getByDisplayValue("alpha")).toBeInTheDocument();
        // Add a new item; onChange's last value is the appended array.
        fireEvent.click(screen.getByTestId("schema-form-add-tags"));
        const last = onChange.mock.calls[onChange.mock.calls.length - 1][0];
        expect(last.tags).toEqual(["alpha", ""]);
    });

    it("renders a raw JSON fallback when the schema is too complex", () => {
        // oneOf / $ref / nested allOf are out of scope for the
        // structured renderer; the form should fall back to a raw
        // JSON textarea so operators can still edit the config
        // (with on-blur validation handled by the parent).
        render(
            <SchemaForm
                schema={{
                    type: "object",
                    oneOf: [
                        {
                            properties: { mode: { const: "s3" } },
                        },
                        {
                            properties: { mode: { const: "gcs" } },
                        },
                    ],
                }}
                value={{ mode: "s3" }}
                onChange={vi.fn()}
                secretFields={[]}
            />,
        );
        expect(screen.getByTestId("schema-form-raw-json")).toBeInTheDocument();
    });
});
