// Typed wrappers over /api/v1. Domain-split so the file layout matches the
// server's handler_*_v2.go grouping; this barrel re-exports everything so
// callers can keep `import { X } from "../lib/api"`.

export * from "./projects";
export * from "./hosts";
export * from "./config_audit";
export * from "./users";
export * from "./settings";
export * from "./server";
export * from "./enrollment";
export * from "./enrollment_presets";
export * from "./project_secrets";
export * from "./account";
export * from "./rbac";
export * from "./install";
export * from "./activities";
export * from "./topology";
export * from "./recordings";
export * from "./marketplace";
