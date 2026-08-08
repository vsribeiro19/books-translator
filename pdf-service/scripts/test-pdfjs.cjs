(async () => {
  const fs = require("fs");
  const pdfjs = await import("pdfjs-dist/legacy/build/pdf.mjs");
  console.log("keys:", Object.keys(pdfjs).filter((k) => ["getDocument", "InvalidPDFException", "PasswordException"].includes(k)));
  const data = new Uint8Array(fs.readFileSync("data/sample.pdf"));
  const doc = await pdfjs.getDocument({ data }).promise;
  console.log("pages:", doc.numPages);
  for (let i = 1; i <= doc.numPages; i++) {
    const page = await doc.getPage(i);
    const tc = await page.getTextContent();
    for (const item of tc.items) {
      if (item.str) {
        const size = item.height ?? tc.styles?.[item.fontName]?.fontSize;
        console.log("P"+i, "y=" + item.transform[5].toFixed(1), "x=" + item.transform[4].toFixed(1), "h=" + size?.toFixed(1), JSON.stringify(item.str));
      }
    }
  }
  await doc.destroy();
})().catch((e) => {
  console.error("ERR", e);
  process.exit(1);
});