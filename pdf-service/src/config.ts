import * as fs from "node:fs";
import * as path from "node:path";
import * as dotenv from "dotenv";
import { parse } from "yaml";

export interface ServiceConfig {
  extract: {
    maxUploadBytes: number;
    requestTimeoutSec: number;
  };
  rebuild: {
    requestTimeoutSec: number;
  };
}

function resolveConfigPath(): string {
  const explicit = process.env.CONFIG_FILE;
  if (explicit) return explicit;
  for (const cand of ["config/pdf-service.yaml", "../config/pdf-service.yaml"]) {
    if (fs.existsSync(cand)) return cand;
  }
  throw new Error(
    "pdf-service config not found (tried config/pdf-service.yaml, ../config/pdf-service.yaml; set CONFIG_FILE to override)"
  );
}

function parseDuration(v: string): number {
  const m = /^(\d+)(h|m|s)$/.exec(v.trim());
  if (!m) throw new Error(`invalid duration: ${v}`);
  const n = Number(m[1]);
  switch (m[2]) {
    case "h":
      return n * 3600;
    case "m":
      return n * 60;
    default:
      return n;
  }
}

const configPath = resolveConfigPath();

dotenv.config({
  path: path.join(path.dirname(configPath), "secrets.env"),
  quiet: true,
});

const raw = parse(fs.readFileSync(configPath, "utf8")) as any;

const extract = (raw.extract ?? {}) as any;
const rebuild = (raw.rebuild ?? {}) as any;

const maxUploadMB = Number(process.env.MAX_UPLOAD_MB ?? extract.max_upload_mb ?? 100);

export const config: ServiceConfig = {
  extract: {
    maxUploadBytes: maxUploadMB * 1024 * 1024,
    requestTimeoutSec: parseDuration(process.env.EXTRACT_TIMEOUT ?? extract.request_timeout ?? "5m"),
  },
  rebuild: {
    requestTimeoutSec: parseDuration(process.env.REBUILD_TIMEOUT ?? rebuild.request_timeout ?? "5m"),
  },
};