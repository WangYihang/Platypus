// Package templates carries the embedded scaffolder template tree.
// Each language directory holds a parallel set of .tmpl files
// rendered via text/template against a templateContext built by
// plugin_new.go. The .tmpl suffix is stripped at write time so the
// emitted project looks idiomatic to the language tool — a Rust
// crate has Cargo.toml (not Cargo.toml.tmpl), etc.
package templates

import "embed"

// FS is the embedded file system holding every template file. The
// scaffolder reads it via fs.WalkDir so adding a new file only
// requires dropping it under the right language subdirectory.
//
//go:embed all:rust all:go
var FS embed.FS
