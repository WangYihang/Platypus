# Authoring plugin configuration

This guide is for **plugin authors** who want their plugin to take
deployment-time configuration — connection endpoints, thresholds,
credentials, allowlists. The shape is declared in `plugin.yaml` and
the operator-facing UI auto-renders a form against it.

If your plugin takes all its parameters at RPC-call time and has no
"installed-once-then-runs" knobs, you can skip this entire document.
The wizard will simply install your plugin without a config form,
and your `OnInstall` handler receives an empty config blob.

## TL;DR

```yaml
# plugin.yaml
config:
  schema_version: 1
  schema:
    type: object
    required: [destination]
    properties:
      destination:
        type: string
        format: uri
        description: "syslog target, e.g. udp://10.0.0.1:514"
      tls: {type: boolean, default: true}
      auth_token:
        type: string
        description: "API key for the destination"
  secret_fields:
    - "/auth_token"
  defaults:
    tls: true
```

That's it. Operators who install your plugin see a form with fields
"destination", "tls" (toggle), "auth_token" (rendered as a "saved
secret" picker because it's marked secret). Your plugin's
`OnInstall(config)` hook receives a JSON blob shaped exactly like
your schema.

## The schema is a JSON Schema

We use **JSON Schema draft 2020-12** verbatim — no DSL, no custom
keywords. Anything documented at <https://json-schema.org/> works.
The Go side validates with
[santhosh-tekuri/jsonschema](https://github.com/santhosh-tekuri/jsonschema);
the FE side validates with [ajv](https://ajv.js.org/) and renders
forms with [react-jsonschema-form](https://rjsf-team.github.io/react-jsonschema-form/).
Both implementations are off-the-shelf — your schema works without
any Platypus-specific tweaking.

Reach for standard schema features:

- **Types**: `string`, `number`, `integer`, `boolean`, `object`,
  `array`.
- **Constraints**: `minLength`, `maxLength`, `minimum`, `maximum`,
  `pattern`, `enum`, `minItems`, `uniqueItems`.
- **Format hints**: `format: uri`, `format: email`,
  `format: ipv4`, `format: hostname`. The FE renders specialised
  inputs for these.
- **Composition**: `oneOf`, `anyOf`, `allOf`, `if/then/else`,
  `$ref`, `$defs`. The FE renders these as discriminated unions.

## Nested vs flat: nested wins

The schema **must be a top-level object**. Group related fields
into nested objects rather than dot-flattening them into the top
level:

```yaml
# YES — nested grouping
schema:
  type: object
  properties:
    database:
      type: object
      required: [host]
      properties:
        host: {type: string}
        port: {type: integer, default: 5432, minimum: 1, maximum: 65535}
        tls:  {type: boolean, default: true}
    retry:
      type: object
      properties:
        max_attempts: {type: integer, default: 3, minimum: 1}
        backoff_ms:   {type: integer, default: 250}
```

```yaml
# NO — dot-flattened keys
schema:
  type: object
  properties:
    database.host: {type: string}    # invalid JSON Schema; loses
    database.port: {type: integer}   # arrays-of-objects entirely
```

Why nested:

1. **Arrays of objects work cleanly** — the canonical shape for
   "configure N destinations" / "watch N paths". Flat KV would
   force `destinations.0.url`, `destinations.0.tls`,
   `destinations.1.url`, …, with no way to add or remove an entry
   without renumbering.
2. **Standard JSON Schema features (oneOf, $ref, conditional
   `required`) require structure.** Flat encodings can't express
   most of them.
3. **Plugin-side decoding stays idiomatic.** Your handler receives
   `{database: {host: "x", port: 5432}}`, decodes into
   `type Config struct { Database DatabaseConfig }`, done.
4. **The FE renders nested objects as collapsible sections** by
   default, so depth doesn't hurt operator UX. Use `description:`
   liberally — it shows as helper text.

**Soft rule**: keep nesting to **2 levels max** for normal config.
If a third level genuinely helps organisation, use it; if it's
just buckets-inside-buckets, flatten.

## Defaults

JSON Schema's `default:` keyword is honoured. Operators who don't
override a field get the default. **Stored configs are deltas over
defaults** — when you bump a default in a new plugin version,
existing deployments pick up the new default automatically.

For multi-field defaults that don't fit cleanly under per-property
`default:`, use the top-level `defaults:` block:

```yaml
config:
  defaults:
    retry:
      max_attempts: 5
      backoff_ms: 500
```

The two are merged; `defaults:` takes priority where both apply.

## Secrets

Mark sensitive fields with **JSON Pointer** paths in
`secret_fields`:

```yaml
config:
  schema:
    type: object
    properties:
      api:
        type: object
        properties:
          base_url: {type: string, format: uri}
          token:    {type: string, description: "API bearer token"}
  secret_fields:
    - "/api/token"
```

Three things happen automatically when a field is in
`secret_fields`:

1. **The UI renders a "saved secret" picker** for that field instead
   of a plaintext input. Operators choose from project-scoped
   `ProjectSecret` rows they've previously created (or create a new
   one inline).
2. **Storage stores a reference**, not the plaintext. The saved
   PluginSpec carries `{"$secret":"sec_xyz"}` instead of the actual
   token; the value is sealed under the project KEK in the
   `project_secrets` table.
3. **The agent receives the resolved plaintext** — substitution
   happens server-side at install time. Your plugin SDK call
   `OnInstall(config)` gets a config that matches your schema
   exactly, with the token field as a plain string. The agent
   never sees secret IDs and never needs a "fetch this secret" RPC.

JSON Pointer rules:

- `"/foo"` is the top-level field `foo`.
- `"/foo/bar"` is `bar` inside `foo`.
- `"/items/0/key"` works for arrays — but most secret-marker
  patterns target *every* element of an array. Use the array path
  itself (`"/destinations"`); the resolver treats every leaf in
  that subtree that matches the schema's "this is a secret"
  invariant as a candidate. (Practical advice: if you want
  per-element secrets in an array of objects, mark the leaf path
  inside the items spec, not the array itself.)
- Escapes: `/` is `~1`, `~` is `~0`. Avoid these characters in
  field names.

Secrets revoked from the project store:

- The plugin install **fails closed** when an operator picks a
  preset that references a revoked secret. Error message names
  the JSON Pointer of the offending field plus the secret_id, so
  the operator knows exactly which preset to update.
- Rotation is an explicit operator action: revoke the old secret,
  create a new one (often under the same name — the partial
  UNIQUE index allows reuse after revocation), then re-save the
  affected presets to point at the new id.

## Schema versioning

Every PluginSpec carries the `schema_version` it was authored
against. When you bump `config.schema_version` in a new plugin
release, **previously-saved configs that target the old version
are rejected**:

```
plugin "com.example.x" config schema_version mismatch:
spec has 1, manifest declares 2 (re-author the config against
the current schema)
```

This is intentional. A v1→v2 schema change might have added a
required field (silently writing `null` into a v1 config would be a
bug at best, a security regression at worst — e.g., a previously-
ignored `allow_unsigned: false` default flipping to
`allow_unsigned: true` because the field didn't exist in v1).
Forcing the operator to re-author is the safer default.

If you need a migration path between versions, document it in your
plugin's release notes; the operator updates the saved presets
explicitly. Future versions of Platypus may grow declarative
migration rules; for now the operator is in the loop.

## When to NOT use config

If your plugin's parameters are:

- **Per-call** (RPC arguments that change with each invocation),
  put them in your protobuf RPC schema, not in `config`. Config is
  the install-time, deployment-shaped settings; RPC args are
  per-request.
- **Fully derivable** from agent state (host fingerprint, OS, IP),
  let the agent SDK derive them at runtime. Don't ask the operator
  to type a value that the agent already knows.
- **Trivially small** (one boolean), consider whether it really
  needs to be configurable, or if a sane default suffices.

## Checklist before publishing

- [ ] Schema validates as draft 2020-12 (run `ajv compile` or
      similar against your manifest).
- [ ] Every required field has either a `description:` explaining
      what to enter, or an obvious name that makes the description
      redundant.
- [ ] Every secret-marked path has its leaf type set to `string`.
- [ ] You've installed the plugin in a test project and verified
      the wizard renders the form sensibly. Long forms? Add
      grouping. Confusing labels? Add `description:`.
- [ ] `schema_version` matches a release tag in your changelog so
      operators can correlate "this preset doesn't load" with a
      version bump.

## Reference: example schemas

A minimal "fan-out destinations" pattern:

```yaml
config:
  schema_version: 1
  schema:
    type: object
    properties:
      destinations:
        type: array
        minItems: 1
        items:
          type: object
          required: [name, url]
          properties:
            name: {type: string, minLength: 1}
            url:  {type: string, format: uri}
            auth_token:
              type: string
              description: "Bearer token for this destination"
            tls: {type: boolean, default: true}
  secret_fields:
    - "/destinations/items/auth_token"
```

A discriminated-union pattern for "pick one of three modes":

```yaml
config:
  schema_version: 1
  schema:
    type: object
    required: [mode]
    properties:
      mode: {type: string, enum: [s3, gcs, local]}
    allOf:
      - if:   {properties: {mode: {const: s3}}}
        then:
          required: [s3_bucket, s3_region]
          properties:
            s3_bucket: {type: string}
            s3_region: {type: string}
            s3_access_key: {type: string}
      - if:   {properties: {mode: {const: gcs}}}
        then:
          required: [gcs_bucket, gcs_credentials]
          properties:
            gcs_bucket: {type: string}
            gcs_credentials: {type: string}
      - if:   {properties: {mode: {const: local}}}
        then:
          required: [local_path]
          properties:
            local_path: {type: string, format: uri}
  secret_fields:
    - "/s3_access_key"
    - "/gcs_credentials"
```

Both schemas validate, both render to forms, both round-trip
faithfully through the install pipeline.
