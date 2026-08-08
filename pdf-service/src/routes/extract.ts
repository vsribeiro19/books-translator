import { Router } from "express";
import multer from "multer";

import { config } from "../config";
import { InvalidPdfError, NoTextError } from "../lib/errors";
import { extractPdf } from "../lib/extractor";

const upload = multer({
  storage: multer.memoryStorage(),
  limits: { fileSize: config.extract.maxUploadBytes },
});

export const extractRouter = Router();

extractRouter.post("/", upload.single("pdf"), async (req, res) => {
  if (!req.file || req.file.buffer.length === 0) {
    res.status(400).json({
      error: "invalid_pdf",
      message: "missing multipart field 'pdf' with the PDF file",
    });
    return;
  }
  try {
    const result = await extractPdf(req.file.buffer);
    res.json(result);
  } catch (err) {
    if (err instanceof NoTextError) {
      res.status(422).json({ error: "no_text", message: err.message });
      return;
    }
    if (err instanceof InvalidPdfError) {
      res.status(400).json({ error: "invalid_pdf", message: err.message });
      return;
    }
    console.error("extract failed", err);
    res.status(500).json({ error: "internal", message: "extraction failed" });
  }
});

extractRouter.use((err: unknown, _req: unknown, res: import("express").Response, next: import("express").NextFunction) => {
  if (err instanceof multer.MulterError) {
    if (err.code === "LIMIT_FILE_SIZE") {
      res.status(413).json({
        error: "file_too_large",
        message: `file exceeds the size limit (${config.extract.maxUploadBytes} bytes)`,
      });
      return;
    }
    res.status(400).json({ error: "invalid_pdf", message: err.message });
    return;
  }
  next(err);
});