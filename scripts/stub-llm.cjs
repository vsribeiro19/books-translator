// Minimal OpenAI-compatible chat/completions stub used by scripts/smoke.sh.
// It parses the recommender's numbered-segment protocol and answers with a
// deterministic "N. PT: <text>" per segment so the pipeline can run offline.
const http = require("http");

const PORT = Number(process.env.PORT || 9876);

const server = http.createServer((req, res) => {
  if (req.method === "POST" && req.url === "/v1/chat/completions") {
    let body = "";
    req.setEncoding("utf8");
    req.on("data", (c) => (body += c));
    req.on("end", () => {
      let messages = [];
      try {
        messages = JSON.parse(body).messages || [];
      } catch {
        /* fall through with empty messages */
      }
      const content = messages.map((m) => m.content || "").join("\n");
      const blocks = content.split("Segments to translate:");
      const segments = blocks[blocks.length - 1] || "";
      const out = [];
      for (const line of segments.split("\n")) {
        const m = /^\s*(\d+)\s*\.\s*(.*)$/.exec(line);
        if (m) out.push(`${m[1]}. PT: ${m[2].trim()}`);
      }
      const reply = {
        choices: [{ message: { role: "assistant", content: out.join("\n") } }],
      };
      if (out.length === 0) {
        reply.error = { message: "stub: no numbered segments found in request" };
      }
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify(reply));
    });
    return;
  }
  if (req.method === "GET") {
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ status: "ok", service: "stub-llm" }));
    return;
  }
  res.writeHead(405);
  res.end();
});

server.listen(PORT, "127.0.0.1", () => {
  console.log(`stub-llm listening on http://127.0.0.1:${PORT}`);
});