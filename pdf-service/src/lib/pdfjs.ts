import * as path from "node:path";
import type * as PdfJs from "pdfjs-dist";

let mod: typeof PdfJs | null = null;

export function standardFontsUrl(): string {
  const dir = path.dirname(require.resolve("pdfjs-dist/package.json"));
  return path.join(dir, "standard_fonts") + path.sep;
}

export async function loadPdfJs(): Promise<typeof PdfJs> {
  if (!mod) {
    mod = (await import("pdfjs-dist/legacy/build/pdf.mjs")) as typeof PdfJs;
  }
  return mod;
}