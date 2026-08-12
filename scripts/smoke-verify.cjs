// Extracts text from a PDF (pdfjs-dist) and asserts an expected substring is
// present. Used by scripts/smoke.sh to validate the translated output.
const fs = require("node:fs");

(async () => {
  const [file, expected] = process.argv.slice(2);
  if (!file) {
    console.error("usage: node smoke-verify.cjs <file.pdf> [expected-substring]");
    process.exit(2);
  }

  const pdfjs = await import("pdfjs-dist/legacy/build/pdf.mjs");
  const data = new Uint8Array(fs.readFileSync(file));
  const doc = await pdfjs.getDocument({ data }).promise;

  const texts = [];
  for (let i = 1; i <= doc.numPages; i++) {
    const page = await doc.getPage(i);
    const tc = await page.getTextContent();
    for (const item of tc.items) {
      if (item.str) texts.push(item.str.trim());
    }
  }
  await doc.destroy();

  const joined = texts.filter(Boolean).join(" ");
  console.log("=== extracted text ===");
  console.log(joined.slice(0, 2000));

  if (expected) {
    if (!joined.includes(expected)) {
      console.error(`verify FAIL: expected "${expected}" not found in PDF text`);
      process.exit(1);
    }
    console.log(`verify OK: found "${expected}"`);
  }
})().catch((e) => {
  console.error("verify FAIL:", e);
  process.exit(1);
});