import { ReactNode, useMemo } from "react";
import { Plus, Trash2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import { Textarea } from "@/components/ui/textarea";
import { palette, space } from "../layout/theme";

// JSON-Schema-driven form for plugin config. The renderer is small
// on purpose: we cover the schema features the canonical
// CONFIG_AUTHORING.md guide steers plugin authors toward —
// top-level object, primitive leaves (string/number/integer/bool),
// arrays of primitives, and "secret-marked" fields (rendered via a
// caller-supplied widget). Schemas that reach for higher-order
// composition (oneOf / $ref / conditional required) drop into a
// raw-JSON textarea fallback so the operator can still edit; the
// caller is expected to ajv-validate on save.

export type SchemaValue = unknown;

export interface JSONSchema {
    type?: string | string[];
    properties?: Record<string, JSONSchema>;
    required?: string[];
    description?: string;
    default?: SchemaValue;
    items?: JSONSchema;
    enum?: SchemaValue[];
    minimum?: number;
    maximum?: number;
    minLength?: number;
    maxLength?: number;
    // Anything below makes the schema too complex for the
    // structured renderer; we fall back to raw JSON.
    oneOf?: JSONSchema[];
    anyOf?: JSONSchema[];
    allOf?: JSONSchema[];
    $ref?: string;
    [key: string]: unknown;
}

export interface SecretWidgetProps {
    /** JSON Pointer-style path of the field, e.g. "/api/token". */
    path: string;
    /** Current value at this path, typed as either a SecretRef
     *  ({"$secret":"sec_..."}) or a plaintext value the operator
     *  is mid-typing. */
    value: SchemaValue;
    onChange: (next: SchemaValue) => void;
}

interface Props {
    schema: JSONSchema;
    value: Record<string, SchemaValue>;
    onChange: (next: Record<string, SchemaValue>) => void;
    /** JSON Pointer paths the manifest marked sensitive. Each
     *  matching field renders via renderSecretWidget instead of
     *  the standard input. */
    secretFields: string[];
    /** Optional override for secret-marked fields. Default: a
     *  read-only display showing the current value (callers
     *  typically pass <SecretPicker /> here). */
    renderSecretWidget?: (props: SecretWidgetProps) => ReactNode;
}

// SchemaForm is the entry point. Decides between the structured
// renderer and the raw-JSON fallback based on schema complexity,
// then walks the property tree.
export default function SchemaForm({
    schema,
    value,
    onChange,
    secretFields,
    renderSecretWidget,
}: Props) {
    const isComplex = useMemo(() => isSchemaComplex(schema), [schema]);
    if (isComplex) {
        return <RawJSONFallback value={value} onChange={onChange} />;
    }
    return (
        <div className="space-y-3">
            {Object.entries(schema.properties ?? {}).map(([key, prop]) => {
                const path = "/" + escapePtrToken(key);
                const isRequired = (schema.required ?? []).includes(key);
                const isSecret = secretFields.includes(path);
                const fieldValue = (value ?? {})[key];
                return (
                    <div
                        key={key}
                        data-testid={`schema-form-field-${key}`}
                        className="space-y-1"
                    >
                        <FieldLabel
                            htmlFor={`schema-form-input-${key}`}
                            label={prop.description ?? key}
                            required={isRequired}
                        />
                        {isSecret && renderSecretWidget ? (
                            renderSecretWidget({
                                path,
                                value: fieldValue,
                                onChange: (v) =>
                                    onChange({ ...value, [key]: v }),
                            })
                        ) : (
                            <LeafField
                                id={`schema-form-input-${key}`}
                                schema={prop}
                                value={fieldValue}
                                onChange={(v) =>
                                    onChange({ ...value, [key]: v })
                                }
                                fieldKey={key}
                            />
                        )}
                    </div>
                );
            })}
        </div>
    );
}

// FieldLabel renders the human-facing label plus the required
// asterisk. Wrapping it in a labelled <Label> via htmlFor pairs
// keeps screen readers happy.
function FieldLabel({
    htmlFor,
    label,
    required,
}: {
    htmlFor: string;
    label: string;
    required: boolean;
}) {
    return (
        <Label
            htmlFor={htmlFor}
            style={{
                fontSize: 12,
                color: palette.textPrimary,
                display: "inline-flex",
                alignItems: "center",
                gap: 4,
            }}
        >
            {label}
            {required && (
                <span
                    style={{ color: palette.danger }}
                    aria-label="required"
                >
                    *
                </span>
            )}
        </Label>
    );
}

// LeafField dispatches to the right primitive renderer based on
// the schema type. Unrecognised types fall back to a plain string
// input — operators rarely hit this, and the parent's ajv
// validator catches type mismatches on save.
function LeafField({
    id,
    schema,
    value,
    onChange,
    fieldKey,
}: {
    id: string;
    schema: JSONSchema;
    value: SchemaValue;
    onChange: (v: SchemaValue) => void;
    fieldKey: string;
}) {
    const typ = Array.isArray(schema.type) ? schema.type[0] : schema.type;

    if (typ === "boolean") {
        return (
            <Checkbox
                id={id}
                checked={Boolean(value ?? schema.default ?? false)}
                onCheckedChange={(c) => onChange(Boolean(c))}
            />
        );
    }
    if (typ === "integer" || typ === "number") {
        return (
            <Input
                id={id}
                type="number"
                value={
                    value === undefined || value === null
                        ? ""
                        : String(value)
                }
                onChange={(e) => {
                    const raw = e.target.value;
                    if (raw === "") {
                        onChange(undefined);
                        return;
                    }
                    const n = typ === "integer" ? parseInt(raw, 10) : parseFloat(raw);
                    if (!Number.isNaN(n)) onChange(n);
                }}
                min={schema.minimum}
                max={schema.maximum}
            />
        );
    }
    if (typ === "array") {
        return (
            <ArrayField
                schema={schema}
                value={Array.isArray(value) ? value : []}
                onChange={onChange}
                fieldKey={fieldKey}
            />
        );
    }
    // Default: string. enum becomes a select.
    if (Array.isArray(schema.enum) && schema.enum.length > 0) {
        return (
            <select
                id={id}
                value={String(value ?? schema.default ?? "")}
                onChange={(e) => onChange(e.target.value)}
                className="border rounded px-2 py-1 text-sm bg-surface"
            >
                {schema.enum.map((opt) => (
                    <option key={String(opt)} value={String(opt)}>
                        {String(opt)}
                    </option>
                ))}
            </select>
        );
    }
    return (
        <Input
            id={id}
            type="text"
            value={typeof value === "string" ? value : ""}
            onChange={(e) => onChange(e.target.value)}
            minLength={schema.minLength}
            maxLength={schema.maxLength}
        />
    );
}

// ArrayField renders an array of primitives as a dynamic list with
// add / remove buttons. Arrays of objects are out of scope for the
// structured renderer (they'd need the 2-level rule waiver) and
// drop into the raw-JSON fallback at the parent level.
function ArrayField({
    schema,
    value,
    onChange,
    fieldKey,
}: {
    schema: JSONSchema;
    value: SchemaValue[];
    onChange: (v: SchemaValue[]) => void;
    fieldKey: string;
}) {
    const itemSchema = schema.items ?? { type: "string" };
    return (
        <div className="space-y-1">
            {value.map((item, i) => (
                <div
                    key={i}
                    style={{
                        display: "flex",
                        gap: space[1],
                        alignItems: "center",
                    }}
                >
                    <Input
                        type="text"
                        value={String(item ?? "")}
                        onChange={(e) => {
                            const next = [...value];
                            next[i] = coerceLeaf(itemSchema, e.target.value);
                            onChange(next);
                        }}
                    />
                    <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => {
                            const next = value.filter((_, j) => j !== i);
                            onChange(next);
                        }}
                        title="Remove"
                    >
                        <Trash2 className="size-3.5" />
                    </Button>
                </div>
            ))}
            <Button
                variant="outline"
                size="sm"
                onClick={() => onChange([...value, defaultLeaf(itemSchema)])}
                data-testid={`schema-form-add-${fieldKey}`}
            >
                <Plus className="size-3.5" />
                Add
            </Button>
        </div>
    );
}

// RawJSONFallback is shown whenever the schema's complexity
// outruns the structured renderer. Operators get a textarea;
// the parent's save flow validates on submit.
function RawJSONFallback({
    value,
    onChange,
}: {
    value: Record<string, SchemaValue>;
    onChange: (v: Record<string, SchemaValue>) => void;
}) {
    return (
        <Textarea
            data-testid="schema-form-raw-json"
            rows={8}
            value={JSON.stringify(value ?? {}, null, 2)}
            onChange={(e) => {
                try {
                    onChange(JSON.parse(e.target.value));
                } catch {
                    // Defer the surface error to the parent's
                    // save flow — keeping the textarea editable
                    // is more important than mid-keystroke
                    // validation.
                }
            }}
        />
    );
}

// isSchemaComplex returns true when the schema reaches for
// composition keywords the structured renderer can't express.
// Used to switch into the raw-JSON fallback.
function isSchemaComplex(s: JSONSchema): boolean {
    if (s.oneOf || s.anyOf || s.allOf || s.$ref) return true;
    // Nested object types are rendered top-level only; deeper
    // nesting falls through. Authors who hit this should follow
    // the 2-level rule in CONFIG_AUTHORING.md.
    for (const v of Object.values(s.properties ?? {})) {
        if (v.type === "object" && v.properties) return true;
        if (
            v.type === "array" &&
            v.items?.type === "object" &&
            v.items.properties
        ) {
            return true;
        }
    }
    return false;
}

// coerceLeaf parses a string from a primitive-leaf input into the
// schema's expected type. Falls back to the raw string for
// unrecognised types — better than surfacing a NaN.
function coerceLeaf(s: JSONSchema, raw: string): SchemaValue {
    const typ = Array.isArray(s.type) ? s.type[0] : s.type;
    if (typ === "integer") {
        const n = parseInt(raw, 10);
        return Number.isNaN(n) ? raw : n;
    }
    if (typ === "number") {
        const n = parseFloat(raw);
        return Number.isNaN(n) ? raw : n;
    }
    if (typ === "boolean") {
        return raw === "true";
    }
    return raw;
}

function defaultLeaf(s: JSONSchema): SchemaValue {
    const typ = Array.isArray(s.type) ? s.type[0] : s.type;
    if (typ === "integer" || typ === "number") return 0;
    if (typ === "boolean") return false;
    return "";
}

function escapePtrToken(s: string): string {
    return s.replace(/~/g, "~0").replace(/\//g, "~1");
}
