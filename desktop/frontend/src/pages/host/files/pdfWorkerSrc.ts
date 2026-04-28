// Resolves the pdfjs worker as a URL Vite emits into the build assets.
// Importing via `?url` is the only way Vite picks it up — `new URL(...,
// import.meta.url)` over a node_modules path silently produces a URL
// that doesn't resolve at runtime, which is what caused the
// "Setting up fake worker failed" load error in dev.
//
// Lives in a dedicated module so vitest can alias-stub it (the `?url`
// query confuses vitest's import-analysis when imported directly from
// PdfViewer.tsx).
import workerUrl from "pdfjs-dist/build/pdf.worker.min.mjs?url";
export default workerUrl;
