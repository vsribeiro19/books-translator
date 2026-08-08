import * as fs from "node:fs";
import { extractPdf } from "../src/lib/extractor";

(async () => {
  const buf = fs.readFileSync("data/sample.pdf");
  const result = await extractPdf(buf);
  console.log(JSON.stringify(result, null, 2));
})().catch((e) => {
  console.error("FAILED", e);
  process.exit(1);
});