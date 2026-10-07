#!/usr/bin/env node
// Spec: docs/specs/task-retrospective.md (fixture capture runner appending
// the exact envelope document the adapter emitted, for tooling/record-envelopes.mjs)
const fs = require("node:fs");
const chunks = [];
process.stdin.on("data", (chunk) => chunks.push(chunk));
process.stdin.on("end", () => {
  fs.appendFileSync(process.env.AGF_CAPTURE_FILE, Buffer.concat(chunks).toString("utf8").trim() + "\n");
  process.exit(Number(process.env.AGF_CAPTURE_EXIT || "0"));
});
