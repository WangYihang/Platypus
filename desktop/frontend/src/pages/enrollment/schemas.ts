import { z } from "zod";

// Shared zod schemas for the two issue-flow forms on the Enrollment
// management page (Audit → Enrollment). Lives here so the form
// components and any future automation surface (CLI helper, scripted
// issue flow) can validate against the same wire shape without
// importing a panel.
//
// All optional fields stay optional; the backend fills in defaults.
// Numeric fields coerce empty strings to undefined so RHF's
// optional-number round-trip works without manual coercion at every
// onChange.

export const installSchema = z.object({
    server_endpoint: z.string().min(1, "required"),
    target_os: z.string().optional(),
    target_arch: z.string().optional(),
    ttl_seconds: z.number().int().positive().optional(),
    auto_approve: z.boolean().optional(),
});
export type InstallFormValues = z.infer<typeof installSchema>;

export const patSchema = z.object({
    description: z.string().optional(),
    ttl_seconds: z.number().int().positive().optional(),
    max_uses: z.number().int().positive().optional(),
    binding_machine_id: z.string().optional(),
});
export type PATFormValues = z.infer<typeof patSchema>;
