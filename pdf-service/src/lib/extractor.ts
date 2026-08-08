import type * as PdfJs from "pdfjs-dist";

import { InvalidPdfError, NoTextError } from "./errors";
import { loadPdfJs, standardFontsUrl } from "./pdfjs";
import type { Block, Chapter, ExtractResult } from "./types";

const HEADING_RATIO = 1.25;
const MAX_HEADING_LENGTH = 240;
const LINE_GROUP_TOLERANCE = 0.3;
const SAME_SIZE_TOLERANCE = 0.6;
const PARAGRAPH_GAP_RATIO = 1.9;
const MIN_CHARS_PER_PAGE = 2;

interface RawLine {
  page: number;
  x: number;
  y: number;
  size: number;
  text: string;
}

interface TextItemData {
  str: string;
  transform: number[];
  width: number;
  height: number;
  fontName: string;
}

interface TextStyle {
  fontSize?: number;
}

interface TextContentData {
  items: unknown[];
  styles: Record<string, TextStyle | undefined>;
}

function textItemOf(it: unknown): TextItemData | null {
  if (typeof it !== "object" || it === null || !("str" in it)) return null;
  const s = (it as { str?: unknown }).str;
  if (typeof s !== "string" || !s.trim()) return null;
  return it as TextItemData;
}

function fontSizeOf(item: TextItemData, tc: TextContentData): number {
  if (item.height && item.height > 0) return item.height;
  return tc.styles[item.fontName]?.fontSize ?? 12;
}

function groupLines(tc: TextContentData, page: number): RawLine[] {
  const buckets: TextItemData[][] = [];

  for (const raw of tc.items) {
    const it = textItemOf(raw);
    if (!it) continue;
    const y = it.transform[5];
    const tol = Math.max(2, fontSizeOf(it, tc) * LINE_GROUP_TOLERANCE);
    let target = -1;
    for (let i = 0; i < buckets.length; i++) {
      if (Math.abs(buckets[i][0].transform[5] - y) <= tol) {
        target = i;
        break;
      }
    }
    if (target === -1) buckets.push([it]);
    else buckets[target].push(it);
  }

  const lines: RawLine[] = [];
  for (const group of buckets) {
    group.sort((a, b) => a.transform[4] - b.transform[4]);
    let text = "";
    let prevEnd = -Infinity;
    let size = 0;
    let x = Infinity;
    for (const it of group) {
      const curX = it.transform[4];
      if (text && prevEnd > -Infinity && curX - prevEnd > 2 && !text.endsWith(" ")) {
        text += " ";
      }
      text += it.str;
      if (it.str) {
        prevEnd = curX + (it.width ?? 0);
        size = Math.max(size, fontSizeOf(it, tc));
        x = Math.min(x, curX);
      }
    }
    const cleaned = text.replace(/\s+/g, " ").trim();
    if (!cleaned) continue;
    lines.push({ page, x, y: group[0].transform[5], size, text: cleaned });
  }

  lines.sort((a, b) => a.page - b.page || b.y - a.y || a.x - b.x);
  return lines;
}

function median(values: number[]): number {
  if (values.length === 0) return 0;
  const sorted = [...values].sort((a, b) => a - b);
  const mid = Math.floor(sorted.length / 2);
  return sorted.length % 2 ? sorted[mid] : (sorted[mid - 1] + sorted[mid]) / 2;
}

function classifyError(err: unknown): Error {
  const name = (err as Error)?.name ?? "";
  const message = (err as Error)?.message ?? String(err);
  if (name.includes("Password") || /password|encrypted/i.test(message)) {
    return new InvalidPdfError("PDF is password-protected; password-protected PDFs are not supported");
  }
  return new InvalidPdfError(`file is not a valid text-extractable PDF: ${message}`);
}

function buildResult(lines: RawLine[], pageCount: number): ExtractResult {
  const totalChars = lines.reduce((acc, l) => acc + l.text.length, 0);
  if (totalChars < Math.max(10, pageCount * MIN_CHARS_PER_PAGE)) {
    throw new NoTextError("PDF has no extractable text (scanned document); OCR is not supported in the MVP");
  }

  const bodySize = median(lines.map((l) => l.size));

  const isHeading = (line: RawLine): boolean =>
    line.size >= bodySize * HEADING_RATIO &&
    line.size > bodySize &&
    line.text.length <= MAX_HEADING_LENGTH;

  const distinctSizes: number[] = [];
  for (const line of lines) {
    if (!isHeading(line)) continue;
    if (!distinctSizes.some((s) => Math.abs(s - line.size) <= SAME_SIZE_TOLERANCE)) {
      distinctSizes.push(line.size);
    }
  }
  const levelsMap = new Map<number, number>();
  [...distinctSizes].sort((a, b) => b - a).forEach((s, i) => levelsMap.set(s, i + 1));

  const chapters: Chapter[] = [];
  let current: Chapter | null = null;
  let paraWords: string[] = [];
  let lastLine: RawLine | null = null;
  let lastWasHeading: boolean = false;

  const flushParagraph = (): void => {
    if (paraWords.length === 0) return;
    if (!current) current = { title: "", blocks: [] };
    current.blocks.push({ type: "paragraph", level: 0, text: paraWords.join(" ") });
    paraWords = [];
  };

  const closeChapter = (): void => {
    flushParagraph();
    if (current) {
      chapters.push(current);
      current = null;
    }
    lastLine = null;
    lastWasHeading = false;
  };

  const appendHeading = (level: number, text: string): void => {
    if (!current) current = { title: "", blocks: [] };
    const last = current.blocks[current.blocks.length - 1];
    if (last && last.type === "heading" && last.level === level) {
      last.text = `${last.text} ${text}`;
      return;
    }
    current.blocks.push({ type: "heading", level, text });
  };

  const handleHeading = (line: RawLine): void => {
    const level = levelsMap.get(line.size) ?? 1;
    if (level === 1) {
      if (lastWasHeading && lastLine) {
        current = current ?? { title: "", blocks: [] };
        current.title = `${current.title}${current.title ? " " : ""}${line.text}`;
      } else {
        closeChapter();
        current = { title: line.text, blocks: [] };
      }
    } else {
      flushParagraph();
      appendHeading(level, line.text);
    }
    lastLine = line;
    lastWasHeading = true;
  };

  const handleBody = (line: RawLine): void => {
    if (!current) current = { title: "", blocks: [] };
    if (lastLine && !lastWasHeading) {
      const gap = lastLine.y - line.y;
      if (lastLine.page !== line.page || gap > Math.max(bodySize * PARAGRAPH_GAP_RATIO, 2)) {
        flushParagraph();
      }
    }
    const prev = paraWords.length > 0 ? paraWords[paraWords.length - 1] : "";
    if (prev.endsWith("-") && line.text.length > 0) {
      paraWords[paraWords.length - 1] = prev.slice(0, -1);
      paraWords.push(line.text);
    } else {
      paraWords.push(line.text);
    }
    lastLine = line;
    lastWasHeading = false;
  };

  for (const line of lines) {
    if (isHeading(line)) handleHeading(line);
    else handleBody(line);
  }
  closeChapter();

  return { pageCount, chapters };
}

export async function extractPdf(buffer: Buffer): Promise<ExtractResult> {
  const pdfjs = await loadPdfJs();
  let doc: PdfJs.PDFDocumentProxy;
  try {
    doc = await pdfjs
      .getDocument({
        data: new Uint8Array(buffer),
        standardFontDataUrl: standardFontsUrl(),
        disableFontFace: true,
      })
      .promise;
  } catch (err) {
    throw classifyError(err);
  }

  try {
    const lines: RawLine[] = [];
    for (let pageNum = 1; pageNum <= doc.numPages; pageNum++) {
      const page = await doc.getPage(pageNum);
      const tc = await page.getTextContent();
      lines.push(...groupLines(tc as unknown as TextContentData, pageNum));
    }
    return buildResult(lines, doc.numPages);
  } catch (err) {
    if (err instanceof NoTextError) throw err;
    throw classifyError(err);
  } finally {
    await doc.destroy().catch(() => {});
  }
}