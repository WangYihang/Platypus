// Stub used by vitest in place of pdfWorkerSrc.ts (which uses Vite's
// `?url` query). The real value is never read because every PdfViewer
// test mocks react-pdf, but exporting *something* keeps module load
// from throwing under jsdom.
export default "test-stub://pdf.worker.min.mjs";
