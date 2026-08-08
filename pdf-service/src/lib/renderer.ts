import PDFDocument from "pdfkit";

import type { Chapter, RebuildInput } from "./types";

const MARGIN = 60;
const PAGE_HEIGHT = 792;
const CONTENT_BOTTOM = PAGE_HEIGHT - MARGIN;

function normalizeText(s: string): string {
  return s
    .replace(/\u2018|\u2019/g, "'")
    .replace(/\u201C|\u201D/g, '"')
    .replace(/\u2013|\u2014/g, "-")
    .replace(/\u2026/g, "...")
    .replace(/\u00A0/g, " ")
    .replace(/\s+/g, " ")
    .trim();
}

function ensureSpace(doc: PDFKit.PDFDocument, needed: number): void {
  if (doc.y + needed > CONTENT_BOTTOM) {
    doc.addPage();
  }
}

function renderParagraph(doc: PDFKit.PDFDocument, text: string): void {
  ensureSpace(doc, 44);
  doc.font("Helvetica").fontSize(11).lineGap(2).text(normalizeText(text), {
    align: "justify",
  });
  doc.moveDown(0.7);
}

function renderHeadingBlock(doc: PDFKit.PDFDocument, level: number, text: string): void {
  doc.moveDown(0.8);
  const size = level >= 2 ? 15 : 19;
  ensureSpace(doc, size * 2);
  doc.font("Helvetica-Bold").fontSize(size).text(normalizeText(text), {
    align: "left",
  });
  doc.font("Helvetica");
  doc.moveDown(0.6);
}

function renderChapter(doc: PDFKit.PDFDocument, chapter: Chapter, first: boolean): void {
  if (!first) doc.addPage();
  if (chapter.title) {
    ensureSpace(doc, 48);
    doc.font("Helvetica-Bold").fontSize(22).text(normalizeText(chapter.title), {
      align: "left",
    });
    doc.font("Helvetica");
    doc.moveDown(1.2);
  }
  for (const block of chapter.blocks) {
    if (block.type === "heading") {
      renderHeadingBlock(doc, block.level, block.text);
    } else {
      renderParagraph(doc, block.text);
    }
  }
}

function renderBook(doc: PDFKit.PDFDocument, input: RebuildInput): void {
  if (input.title) {
    doc.font("Helvetica-Bold").fontSize(30).text(normalizeText(input.title), {
      align: "center",
    });
    doc.font("Helvetica");
    doc.moveDown(2.5);
  }
  for (let i = 0; i < input.chapters.length; i++) {
    renderChapter(doc, input.chapters[i], i === 0);
  }
}

export function renderPdf(out: NodeJS.WritableStream, input: RebuildInput): Promise<void> {
  return new Promise<void>((resolve, reject) => {
    const doc = new PDFDocument({
      size: "LETTER",
      margin: MARGIN,
      info: {
        Title: input.title || "Translated book",
        Creator: "books-translator pdf-service",
      },
    });

    doc.pipe(out);

    const fail = (err: Error): void => {
      reject(err);
    };
    doc.on("error", fail);
    out.on("error", fail);
    out.on("finish", resolve);

    try {
      renderBook(doc, input);
      doc.end();
    } catch (err) {
      reject(err as Error);
    }
  });
}