#!/usr/bin/env node
/**
 * Regenerates the recorded observation envelopes
 * (fixtures/retrospective/opencode/envelopes/<scenario>.jsonl) from the raw
 * event fixtures by replaying them through the REAL adapter exactly like
 * the npm replay tests do (AGF_BIN fixture capture runner + version stub).
 *
 * Rerun after an adapter change and review the diff:
 *
 *   cd hooks/retrospective && npm test && node tools/record-envelopes.mjs
 *
 * The recorded envelopes are what the Go replay and loop tests consume —
 * the event→envelope mapping exists exactly once (in the adapter).
 *
 * Spec: docs/specs/task-retrospective.md
 */
import { createRequire } from "node:module";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";

const require = createRequire(import.meta.url);
const hooksRoot = path.resolve(import.meta.dirname, "..");
const repoRoot = path.resolve(hooksRoot, "../..");
const fixtureRoot = path.join(repoRoot, "fixtures", "retrospective", "opencode");
const { RetrospectivePlugin } = require(path.join(hooksRoot, "dist", "opencode.js"));

process.env.AGF_PLUGIN_FAKE_HEALTH = JSON.stringify({ healthy: true, version: "2.0.24" });
const work = mkdtempSync(path.join(os.tmpdir(), "agf-record-"));
const captureFile = path.join(work, "captures.jsonl");
process.env.AGF_CAPTURE_FILE = captureFile;
process.env.AGF_BIN = path.join(hooksRoot, "tests", "record-cli.cjs");

const scenarios = [
  "streamed.jsonl", "duplicates.jsonl", "childsession.jsonl",
  "abort-then-idle.jsonl", "multiturn.jsonl", "compaction.jsonl",
  "restart-pre.jsonl", "restart-post.jsonl", "learning-suppressed.jsonl",
  "success-recovery.jsonl",
];
const tokens = { "{{execution_root}}": "/proj/fixture", "{{wrong_root}}": "/proj/other" };

mkdirEnvelopes();
for (const scenario of scenarios) {
  rmSync(captureFile, { force: true });
  const events = readEvents(scenario);
  // One plugin instance per scenario (a fresh session window): the same
  // shape the replay tests use.
  const hooks = await RetrospectivePlugin({ serverUrl: new URL("http://127.0.0.1:4096"), directory: "/proj/fixture" });
  for (const event of events) {
    await hooks.event({ event });
  }
  const destination = path.join(fixtureRoot, "envelopes", scenario);
  let body = "";
  try {
    body = readFileSync(captureFile, "utf8").trim();
  } catch {
    body = "";
  }
  // Re-tokenize the host-specific observed fields so every consumer
  // substitutes them: the observed server URL, the project directory and
  // the session identity directories.
  if (body.length > 0) {
    const lines = body.split("\n").filter(Boolean).map((line) => {
      const envelope = JSON.parse(line);
      if (envelope.server_url) {
        envelope.server_url = "{{server_url}}";
      }
      if (envelope.project_dir) {
        envelope.project_dir = "{{execution_root}}";
      }
      for (const record of envelope.records ?? []) {
        if (record.kind === "session" && record.directory) {
          record.directory = "{{execution_root}}";
        }
      }
      return JSON.stringify(envelope);
    });
    body = lines.join("\n");
  }
  writeFileSync(destination, body.length > 0 ? body + "\n" : "");
  const records = body.length > 0 ? body.split("\n").length : 0;
  console.log(`${scenario}: ${records} envelope(s)`);
}

function mkdirEnvelopes() {
  mkdirSync(path.join(fixtureRoot, "envelopes"), { recursive: true });
}

function readEvents(scenario) {
  const body = readFileSync(path.join(fixtureRoot, "events", scenario), "utf8").trim();
  return body.split("\n").map((line) => {
    let text = line;
    for (const [token, replacement] of Object.entries(tokens)) {
      text = text.split(token).join(replacement);
    }
    return JSON.parse(text);
  });
}
