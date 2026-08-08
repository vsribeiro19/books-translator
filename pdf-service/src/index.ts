import express from "express";

import { config } from "./config";
import { healthRouter } from "./routes/health";
import { extractRouter } from "./routes/extract";
import { rebuildRouter } from "./routes/rebuild";

const port = Number(process.env.PORT ?? 8081);

const app = express();

app.use(express.json({ limit: "100mb" }));

app.use("/health", healthRouter);
app.use("/extract", extractRouter);
app.use("/rebuild", rebuildRouter);

app.listen(port, () => {
  console.log(`pdf-service listening on :${port}`);
});