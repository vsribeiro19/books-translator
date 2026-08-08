import * as fs from "node:fs";
import PDFDocument from "pdfkit";
import { extractPdf } from "../src/lib/extractor";
import { renderPdf } from "../src/lib/renderer";
import { InvalidPdfError, NoTextError } from "../src/lib/errors";

function makeBlankPdf(file: string): Promise<void> {
  const doc = new PDFDocument();
  const out = fs.createWriteStream(file);
  doc.pipe(out);
  doc.addPage();
  doc.end();
  return new Promise((resolve) => out.on("finish", resolve));
}

function makeEncryptedPdf(file: string): Promise<void> {
  const doc = new PDFDocument({
    userPassword: "secret123",
    ownerPassword: "owner123",
  });
  const out = fs.createWriteStream(file);
  doc.pipe(out);
  doc.fontSize(12).text("protected content here");
  doc.end();
  return new Promise((resolve) => out.on("finish", resolve));
}

async function expectPdfError(buf: Buffer, label: string): Promise<void> {
  try {
    await extractPdf(buf);
    console.log(`[KO] ${label}: expected an error but got a result`);
  } catch (err) {
    if (err instanceof InvalidPdfError || err instanceof NoTextError) {
      console.log(`[OK] ${label}: ${err.name} -> ${err.message}`);
    } else {
      console.log(`[??] ${label}: unexpected error`, err);
    }
  }
}

async function main(): Promise<void> {
  await expectPdfError(Buffer.from(["%PDF-1.4 not really a pdf", "garbage"].join("\n")), "invalid-file");

  fs.mkdirSync("data/tmp", { recursive: true });
  await makeBlankPdf("data/tmp/blank.pdf");
  await expectPdfError(fs.readFileSync("data/tmp/blank.pdf"), "blank-no-text");

  await makeEncryptedPdf("data/tmp/encrypted.pdf");
  await expectPdfError(fs.readFileSync("data/tmp/encrypted.pdf"), "encrypted");

  const input = {
    title: "O Livro Traduzido",
    chapters: [
      {
        title: "Capítulo 1: Introdução",
        blocks: [
          { type: "heading", level: 2, text: "Visão geral" },
          { type: "paragraph", level: 0, text: "Texto traduzido com acentuação: coração, ação, órgão, café, exceção — e aspas “curvas” e travessões – — ..." },
          { type: "paragraph", level: 0, text: "Segundo parágrafo curto para separar estruturas." },
        ],
      },
      {
        title: "Capítulo 2: Métodos",
        blocks: [
          { type: "paragraph", level: 0, text: "Conteúdo do segundo capítulo." },
        ],
      },
    ],
  };

  const outFile = "data/tmp/roundtrip.pdf";
  const out = fs.createWriteStream(outFile);
  await renderPdf(out, input);
  const rebuilt = await extractPdf(fs.readFileSync(outFile));
  console.log("\n=== ROUND TRIP REBUILD EXTRACT ===");
  console.log(JSON.stringify(rebuilt, null, 2));
}

main().catch((err) => {
  console.error("TEST FAILED", err);
  process.exit(1);
});