// Tracked stub so `tsc` can resolve the wailsjs-path imports every page
// uses ("../../wailsjs/go/app/App") WITHOUT `wails generate` having run.
//
// - Web build   (`vite --mode web`): Vite aliases these imports to
//   src/platform/App.web.ts (see vite.config.ts), so this file is never
//   actually loaded at runtime. The re-export below only ever matters
//   for type-checking in that mode.
// - Desktop build: `wails generate` overwrites this file with the real
//   TS bindings that call into the Go runtime. The exported symbol set
//   stays identical, so `tsc` keeps passing before generation.
//
// Expect this file to appear as "modified" in `git status` after
// `wails generate`; revert it if you don't want to commit the
// generated form.

export * from "../../../src/platform/App.web";
