import { Router } from "express";

import { renderPdf } from "../lib/renderer";
import type { Block, RebuildInput } from "../lib/types";

export const rebuildRouter = Router();

const MAX_BLOCKS = 20_000;
const MAX_TEXT_LENGTH = 100_000;

function validBlock(v: unknown): v is Block {
  if (typeof v !== "object" || v === null) return false;
  const b = v as Record<string, unknown>;
  return (
    (b.type === "heading" || b.type === "paragraph") &&
    typeof b.level === "number" &&
    typeof b.text === "string" &&
    b.text.length <= MAX_TEXT_LENGTH
  );
}

function validInput(body: unknown): RebuildInput | null {
  if (typeof body !== "object" || body === null) return null;
  const raw = body as Record<string, unknown>;
  if (typeof raw.title !== "string" || !Array.isArray(raw.chapters)) return null;
  if (raw.chapters.length > MAX_BLOCKS) return null;

  const chapters: RebuildInput["chapters"] = [];
  for (const chapterRaw of raw.chapters) {
    if (typeof chapterRaw !== "object" || chapterRaw === null) return null;
    const c = chapterRaw as Record<string, unknown>;
    if (typeof c.title !== "string" || !Array.isArray(c.blocks)) return null;
    if (c.blocks.length > MAX_BLOCKS) return null;
    if (!c.blocks.every(validBlock)) return null;
    chapters.push({ title: c.title, blocks: c.blocks as Block[] });
  }
  return { title: raw.title, chapters };
}

rebuildRouter.post("/", (req, res) => {
  const input = validInput(req.body);
  if (!input) {
    res.status(400).json({
      error: "invalid_structure",
      message: "expected { title: string, chapters: [{ title: string, blocks: [{ type: 'heading'|'paragraph', level: number, text: string }] }] }",
    });
    return;
  }

  res.setHeader("Content-Type", "application/pdf");
  res.setHeader("Content-Disposition", 'inline; filename="translated.pdf"');

  renderPdf(res, input).catch((err: unknown) => {
    console.error("rebuild failed", err);
    res.destroy(err instanceof Error ? err : new Error(String(err)));
  });
});