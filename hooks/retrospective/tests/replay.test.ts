// Spec: docs/specs/task-retrospective.md — recorded-event replay tests of
// the OpenCode observation adapter. Offline, deterministic, no live
// sessions: the CLI spawn is replaced by a fixture capture script (env
// AGF_BIN) and the server version by AGF_PLUGIN_FAKE_HEALTH.
import test, { describe } from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { chmodSync, mkdirSync, mkdtempSync, readFileSync, writeFileSync, readdirSync, existsSync, symlinkSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import type { Envelope } from "../opencode";
import { RetrospectivePlugin } from "../opencode";
import nativePlugin from "../opencode";
import { execFileSync } from "node:child_process";
import { EventEmitter } from "node:events";
// spec: docs/specs/task-retrospective.md (OpenCode observation adapter)

// Compiled output lives at dist/tests; the recorded fixtures live at the
// repository-root fixtures/retrospective/opencode (shared with the Go replay
// and loop tests).
const FIXTURES = path.resolve(__dirname, "../../../../fixtures/retrospective/opencode");
const SUPPORTED_VERSION = "2.0.24";

for (const nativeVersion of ["2.0.24", "2.0.27", "2.0.30", "2.0.31"]) test(`native ${nativeVersion} read brackets reach ingest, native history, episode and strict reflection without assigned prerequisites`, async () => {
  const harness = fixtureCLI(); delete process.env.AGF_PLUGIN_FAKE_HEALTH;
  let root = path.join(harness.root, "proj");
  const git = (...args: string[]) => execFileSync("git", ["-C", root, "-c", "core.hooksPath=/dev/null", "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", ...args], { encoding: "utf8", env: { ...process.env, GIT_CONFIG_NOSYSTEM: "1", GIT_CONFIG_GLOBAL: "/dev/null" }, timeout: 10000 });
  git("init", "-q", "--initial-branch=bracket-fixture"); git("commit", "--allow-empty", "-qm", "bracket fixture");
  if (nativeVersion === "2.0.27") git("switch", "--detach", "HEAD");
  if (nativeVersion === "2.0.31") {
    const worktree = path.join(harness.root, "worktree");
    git("worktree", "add", "-q", "-b", "bracket-worktree", worktree); root = worktree;
  }
  const file = path.join(root, "catalog.txt"); writeFileSync(file, "one\ntwo\n");
  const revision = git("rev-parse", "HEAD").trim();
  const history: Record<string, any[]> = {};
  const hooks: Record<string, any> = {};
  let intake: any;
  const modes = ["positive", "missingbefore", "missingafter", "changedfile", "mismatch", "historymismatch", "conflict", "reconcilegap", "symlink", "escape", "oversize", "secret", "missingrevision", "packedrevision", "revisionchange"];
  symlinkSync(file, path.join(root, "link.txt"));
  writeFileSync(path.join(root, "large.txt"), "x".repeat(65537));
  writeFileSync(path.join(root, ".env"), "BRACKET-SECRET-CANARY");
  writeFileSync(path.join(harness.root, "outside.txt"), "OUTSIDE-ROOT-CANARY");
  try {
    const cleanup = await nativePlugin.setup({ app: { version: nativeVersion }, location: { directory: root },
      session: { async get({ sessionID }: any) { return { id: sessionID, title: "bracket QA", location: { directory: root } }; }, async hook(_name: string, callback: any) { intake = callback; return { async dispose() {} }; } },
      tool: { async hook(name: string, callback: any) { hooks[name] = callback; return { async dispose() {} }; } },
      event: { async *subscribe() {
        for (const mode of modes) {
          const sessionID = `ses_bracket_${mode}`;
          const user = `user_${mode}`;
          if (mode === "packedrevision") { if (nativeVersion === "2.0.27") git("switch", "-q", "bracket-fixture"); git("pack-refs", "--all"); }
          history[mode] = [{ id: user, type: "user", time: { created: 1 }, text: "Read the unchanged catalog at valid offsets; retain observed failures." }];
          await intake({ sessionID, messageID: user, prompt: { text: history[mode][0].text } });
          for (let i = 0; i < 4; i++) {
            const messageID = `msg_${mode}_${i}`, id = "read:0";
            const target = mode === "symlink" ? "link.txt" : mode === "oversize" ? "large.txt" : mode === "secret" ? ".env" : "catalog.txt";
            const input = { path: mode === "escape" ? root + "/../outside.txt" : path.join(root, target), offset: i % 2 === 0 ? 1000 : 1, limit: 1 };
            const event = { sessionID, messageID, id, tool: "read", input };
            if (!(mode === "missingbefore" && i === 0)) await hooks["execute.before"](event);
            if (mode === "changedfile" && i === 0) writeFileSync(file, "changed during call\n");
            if (mode === "revisionchange" && i === 0) git("commit", "--allow-empty", "-qm", "changed revision during call");
            if (mode === "mismatch" && i === 0) event.input = { ...input, offset: 1001 };
            const failed = i % 2 === 0;
            if (!(mode === "missingafter" && i === 0)) await hooks["execute.after"]({ ...event, status: failed ? "error" : "completed" });
            if (mode === "conflict" && i === 0) await hooks["execute.after"]({ ...event, status: "error" });
            const data = { sessionID, assistantMessageID: messageID, id };
            yield { type: "session.step.started", data };
            yield { type: "session.tool.input.started", data: { ...data, name: "read" } };
            yield { type: "session.tool.called", data: { ...data, input: event.input } };
            yield { type: failed ? "session.tool.failed" : "session.tool.success", data: { ...data, error: failed ? { message: "Offset beyond end of file" } : undefined, content: failed ? [] : [{ type: "text", text: "one" }] } };
            const historyInput = mode === "historymismatch" && i === 0 ? { ...event.input, offset: 1001 } : event.input;
            const state = failed ? { status: "error", input: historyInput, error: { type: "tool.execution", message: "Offset beyond end of file" } } : { status: "completed", input: historyInput, content: [{ type: "text", text: "one" }] };
            history[mode].push({ id: messageID, type: "assistant", agent: "build", model: { id: "fixture", providerID: "fixture" }, time: { created: i+2, completed: i+2 }, content: [{ type: "tool", id, name: "read", state, time: { created: i+2, completed: i+2 } }] });
          }
          history[mode].push({ id: `idle_${mode}`, type: "idle", time: { created: 9 }, outcome: "succeeded" });
          yield { type: "session.execution.succeeded", data: { sessionID } };
        }
      } },
    } as any);
    await cleanup();
    const envelopes = envelopesOf(harness);
    const brackets = envelopes.flatMap(e => e.records).filter(r => r.kind === "read_bracket");
    assert.equal(brackets.filter(r => r.session_id === "ses_bracket_positive").length, 4);
    assert.ok(!JSON.stringify(brackets).includes("BRACKET-SECRET-CANARY"));
    assert.ok(!brackets.some(r => r.session_id === "ses_bracket_secret"));
    const escape = brackets.filter(r => r.session_id === "ses_bracket_escape");
    assert.ok(escape.every(r => !r.read_bracket?.before && !r.read_bracket?.after), "outside-root paths must never acquire snapshots");
    assert.notEqual(readFileSync(file, "utf8"), "one\ntwo\n", "later disk state must not rehydrate positive prerequisite snapshots");
    assert.notEqual(git("rev-parse", "HEAD").trim(), revision, "later HEAD must not rehydrate the positive episode version");
    const fixture = path.join(harness.root, "brackets.json");
    writeFileSync(fixture, JSON.stringify({ envelopes, histories: history, revision }));
    try {
      const result = execFileSync("go", ["test", "-count=1", "-v", "./internal/retrospective", "-run", "^TestNativeReadBracketCapturedFlow$"], {
        cwd: path.resolve(FIXTURES, "../../.."), timeout: 120000,
        env: { ...process.env, GOPROXY: "off", GOSUMDB: "off", AGF_TEST_READ_BRACKETS: fixture }, encoding: "utf8",
      });
      assert.match(result, /PASS/);
    } catch (error: any) { throw new Error(`${error.message}\n${error.stdout}\n${error.stderr}`); }
  } finally { harness.restore(); }
});

interface CaptureRow {
  argv: string[];
  stdin: string;
}

function installCaptureCLI(root: string, exit: number): string {
  const script = path.join(root, "capture-cli.cjs");
  writeFileSync(
    script,
    [
      "#!/usr/bin/env node",
      "const fs = require('node:fs');",
      "if (process.argv.at(-1) === '--help') { process.stderr.write('agent-fabric: flag: help requested\\n'); process.exit(1); }",
      "const chunks = [];",
      "process.stdin.on('data', (chunk) => chunks.push(chunk));",
      "process.stdin.on('end', () => {",
      "  fs.appendFileSync(process.env.AGF_CAPTURE_FILE, JSON.stringify({ argv: process.argv.slice(2), stdin: Buffer.concat(chunks).toString('utf8') }) + '\\n');",
      "  process.exit(Number(process.env.AGF_CAPTURE_EXIT || '0'));",
      "});",
    ].join("\n"),
  );
  chmodSync(script, 0o700);
  process.env.AGF_BIN = script;
  process.env.AGF_CAPTURE_EXIT = String(exit);
  process.env.AGF_PLUGIN_FAKE_HEALTH = JSON.stringify({ healthy: true, version: SUPPORTED_VERSION });
  return path.join(root, "captures.jsonl");
}

function loadEvents(name: string): unknown[] {
  return readFileSync(path.join(FIXTURES, "events", name), "utf8")
    .trim()
    .split("\n")
    .map((line) => JSON.parse(line));
}

test("ingest settles once per attempt across timeout/close/error/throw and disabled", async (t) => {
  const cp = require("node:child_process");
  for (const mode of ["timeout", "error", "throw", "no-stdin", "exit", "disabled", "ok"]) {
    const harness = fixtureCLI();
    const previous = process.env.AGF_RETROSPECTIVE_STATE;
    const state = path.join(harness.root, "state"); process.env.AGF_RETROSPECTIVE_STATE = state;
    const sent: string[] = [];
    let attempts = 0;
    t.mock.timers.enable({ apis: ["setTimeout"] });
    const mock = t.mock.method(cp, "spawn", () => {
      attempts++;
      if (mode === "throw") throw new Error("synthetic spawn throw");
      const child: any = new EventEmitter(); child.stderr = new EventEmitter();
      child.kill = () => { queueMicrotask(() => child.emit("close", null)); return true; };
      if (mode !== "no-stdin") {
        child.stdin = new EventEmitter();
        child.stdin.end = (text: string) => {
          sent.push(text);
          if (mode !== "timeout") queueMicrotask(() => {
            child.stderr.emit("data", Buffer.from("synthetic diagnostic\n"));
            if (mode === "error") child.emit("error", new Error("synthetic error"));
            child.emit("close", mode === "disabled" ? 2 : mode === "ok" ? 0 : 1);
          });
        };
      } else queueMicrotask(() => child.emit("close", 1));
      return child;
    });
    try {
      const hooks = await RetrospectivePlugin({ serverUrl: new URL("http://127.0.0.1:4200"), directory: harness.root } as any);
      await hooks.event!({ event: { type: "message.part.updated", properties: { part: { id: "opening", sessionID: "ses_spawn", messageID: "user", type: "text", text: "Actual opening bytes" } } } } as any);
      const flush = hooks.event!({ event: { type: "session.idle", properties: { sessionID: "ses_spawn" } } } as any);
      for (let turn = 0; turn < 3; turn++) { await new Promise(setImmediate); t.mock.timers.tick(30000); }
      await flush; await new Promise(setImmediate);
      const failure = mode !== "disabled" && mode !== "ok";
      assert.equal(attempts, failure ? 2 : 1, mode);
      const directory = path.join(state, "observer-errors");
      const reports = existsSync(directory) ? readdirSync(directory) : [];
      assert.equal(reports.length, failure ? attempts : 0, `${mode}: exactly one diagnostic per failed attempt`);
      if (sent.length === 2) assert.equal(sent[0], sent[1], `${mode}: retained exact retry batch`);
    } finally { mock.mock.restore(); t.mock.timers.reset(); harness.restore(); if (previous === undefined) delete process.env.AGF_RETROSPECTIVE_STATE; else process.env.AGF_RETROSPECTIVE_STATE = previous; }
  }
});

test("native read provenance excludes secret suffixes/segments and hashes only normal text", async () => {
  const harness = fixtureCLI(); delete process.env.AGF_PLUGIN_FAKE_HEALTH;
  const root = path.join(harness.root, "proj");
  const secrets = ["config/app.env", "key.pem", "key.p12", "key.pfx", "credentials/token.txt", "secrets/data.txt", "id_rsa", "id_ed25519", ".env.local"];
  let after: any;
  try {
    for (const file of [...secrets, "normal.txt"]) { mkdirSync(path.dirname(path.join(root, file)), { recursive: true }); writeFileSync(path.join(root, file), file === "normal.txt" ? "ordinary text" : "CANARY-NEVER-HASH-SECRET"); }
    const cleanup = await nativePlugin.setup({ app: { version: "2.0.24" }, location: { directory: root },
      session: { async get() { throw new Error("unused identity lookup"); }, async hook() { return { async dispose() {} }; } },
      tool: { async hook(_name: string, callback: any) { after = callback; return { async dispose() {} }; } },
      event: { async *subscribe() {
        for (const file of [...secrets, "normal.txt"]) await after({ tool: "read", status: "completed", sessionID: "ses_secret", input: { path: file } });
        yield { type: "session.execution.succeeded", data: { sessionID: "ses_secret" } };
      } },
    } as any);
    await cleanup();
    const envelopes = envelopesOf(harness); const records = envelopes.flatMap(envelope => envelope.records);
    assert.equal(records.filter(record => record.kind === "source_gap").length, secrets.length);
    const sources = records.filter(record => record.kind === "source");
    assert.equal(sources.length, 1); assert.equal(sources[0].sha256, createHash("sha256").update("ordinary text").digest("hex"));
    assert.ok(!JSON.stringify(envelopes).includes("CANARY-NEVER-HASH-SECRET"));
    assert.ok(!JSON.stringify(envelopes).includes(createHash("sha256").update("CANARY-NEVER-HASH-SECRET").digest("hex")));
  } finally { harness.restore(); }
});

test("native duplicate old step cannot borrow a later opening or queued inbox identity", async () => {
  const harness = fixtureCLI(); delete process.env.AGF_PLUGIN_FAKE_HEALTH;
  let intake: any;
  try {
    const cleanup = await nativePlugin.setup({ app: { version: "2.0.24" }, location: { directory: harness.root },
      session: { async get() { return { id: "ses_owner", location: { directory: harness.root } }; }, async hook(_name: string, callback: any) { intake = callback; return { async dispose() {} }; } },
      tool: { async hook() { return { async dispose() {} }; } },
      event: { async *subscribe() {
        yield { type: "session.inbox.enqueued", data: { sessionID: "ses_owner", inboxID: "queued_not_admitted", item: { type: "user", payload: { text: "queued only" } } } };
        yield { type: "session.step.started", data: { sessionID: "ses_owner", assistantMessageID: "original" } };
        await intake({ sessionID: "ses_owner", messageID: "later", prompt: { text: "Later separate task" } });
        yield { type: "session.step.started", data: { sessionID: "ses_owner", assistantMessageID: "original" } };
        yield { type: "session.step.started", data: { sessionID: "ses_owner", assistantMessageID: "later_step" } };
        yield { type: "session.execution.succeeded", data: { sessionID: "ses_owner" } };
      } },
    } as any);
    await cleanup();
    const messages = envelopesOf(harness).flatMap(envelope => envelope.records).filter(row => row.kind === "message");
    const original = messages.filter(row => row.record_id === "original");
    assert.equal(original.length, 2); assert.ok(original.every(row => !row.parent_id));
    assert.equal(messages.find(row => row.record_id === "later_step")?.parent_id, "later");
  } finally { harness.restore(); }
});

for (const nativeVersion of ["2.0.24", "2.0.27", "2.0.30", "2.0.31"]) test(`fresh native ${nativeVersion} opening failed reads survive real ingest validation and terminal batching`, async () => {
  const harness = fixtureCLI();
  const previous = { state: process.env.AGF_RETROSPECTIVE_STATE, config: process.env.AGF_CONFIG, noSpawn: process.env.AGF_RETROSPECTIVE_NO_SPAWN };
  delete process.env.AGF_PLUGIN_FAKE_HEALTH;
  const root = path.join(harness.root, "proj");
  const state = path.join(harness.root, "state");
  const settings = path.join(harness.root, "settings.json");
  writeFileSync(settings, JSON.stringify({ retrospective: { enabled: true } }));
  process.env.AGF_CONFIG = settings;
  process.env.AGF_RETROSPECTIVE_STATE = state;
  process.env.AGF_RETROSPECTIVE_NO_SPAWN = "1";
  const binary = path.join(harness.root, "agf");
  const repository = path.resolve(FIXTURES, "../../..");
  execFileSync("go", ["build", "-o", binary, "./cmd/agent-fabric"], { cwd: repository, timeout: 60000, env: { ...process.env, GOPROXY: "off", GOSUMDB: "off" } });
  const proxy = path.join(harness.root, "ingest.cjs");
  writeFileSync(proxy, `#!/usr/bin/env node\nconst fs=require('node:fs'),cp=require('node:child_process');if(process.argv.at(-1)==='--help'){const r=cp.spawnSync(${JSON.stringify(binary)},process.argv.slice(2),{encoding:'utf8',timeout:5000});process.stdout.write(r.stdout||'');process.stderr.write(r.stderr||'');process.exit(r.status??1);}let text='';process.stdin.on('data',c=>text+=c);process.stdin.on('end',()=>{const first=!fs.existsSync(${JSON.stringify(harness.captureFile)});const r=first?{status:1,stderr:'synthetic transient ingest failure\\n',stdout:''}:cp.spawnSync(${JSON.stringify(binary)},process.argv.slice(2),{input:text,encoding:'utf8',timeout:5000});fs.appendFileSync(${JSON.stringify(harness.captureFile)},JSON.stringify({argv:process.argv.slice(2),stdin:text,code:r.status,stderr:r.stderr})+'\\n');process.stdout.write(r.stdout||'');process.stderr.write(r.stderr||'');process.exit(r.status??1);});`);
  chmodSync(proxy, 0o700); process.env.AGF_BIN = proxy;
  const id = "ses_fresh_failed_reads";
  let hook: any;
  let intake: any;
  const hash = createHash("sha256").update("owning instruction at read time\n").digest("hex");
  const source = path.join(root, "AGENTS.md"); writeFileSync(source, "owning instruction at read time\n");
  try {
    const cleanup = await nativePlugin.setup({ app: { version: nativeVersion }, location: { directory: root },
      session: { async get() { return { id, title: "fresh task", location: { directory: root } }; }, async hook(_name: string, callback: any) { intake = callback; return { async dispose() {} }; } },
      tool: { async hook(_name: string, callback: any) { hook = callback; return { async dispose() {} }; } },
      event: { async *subscribe() {
        // Fresh location setup missed creation/enqueue: actual prompt hook
        // must preserve identity/user intake without inferring a past source.
        await intake({ sessionID: id, messageID: "msg_user", prompt: { text: "Read owning instructions and recover stale paths." } });
        for (let n = 0; n < 29; n++) {
          const data = { sessionID: id, assistantMessageID: `msg_${n}`, id: "read:0" };
          const file = n === 0 ? source : path.join(root, `missing-${n}.txt`);
          yield { type: "session.step.started", data };
          yield { type: "session.tool.input.started", data: { ...data, name: "read" } };
          yield { type: "session.tool.called", data: { ...data, input: { path: file } } };
          if (n === 0) {
            await hook({ tool: "read", status: "completed", sessionID: id, input: { path: file } });
            writeFileSync(source, "changed after the actual read\n");
          }
          yield { type: n > 0 && n < 4 ? "session.tool.failed" : "session.tool.success", data: { ...data, error: n > 0 && n < 4 ? { message: "File not found" } : undefined, content: [{ type: "text", text: "observable result" }] } };
        }
        yield { type: "session.text.ended", data: { sessionID: id, assistantMessageID: "msg_final", ordinal: 0, text: "DONE" } };
        yield { type: "session.execution.succeeded", data: { sessionID: id } };
      } },
    } as any);
    await cleanup();
    const captures = capturesOf(harness.captureFile) as (CaptureRow & { code: number; stderr: string })[];
    assert.equal(captures[0].code, 1);
    assert.ok(captures.slice(1).every(row => row.code === 0), captures.map(row => row.stderr).join("\n"));
    const spool = JSON.parse(readFileSync(path.join(state, "spool", `${id}.json`), "utf8"));
    assert.equal(spool.source_refs[0].sha256, hash, "source hash must remain the observed version, not today's bytes");
    assert.equal(spool.source_refs.length, 1);
    assert.ok(spool.sessions[id]);
    assert.equal(spool.messages.msg_0.parent_id, "msg_user", "native step ownership names the admitted opening, not a later prompt");
    assert.equal(Object.values(spool.parts).filter((p: any) => p.part_type === "tool").length, 29);
    assert.ok(JSON.parse(readFileSync(path.join(state, "jobs", `${id}.json`), "utf8")));
    const reports = readdirSync(path.join(state, "observer-errors"));
    assert.ok(reports.length > 0);
    assert.match(readFileSync(path.join(state, "observer-errors", reports[0]), "utf8"), /synthetic transient ingest failure/);
  } finally {
    harness.restore();
    for (const [name, value] of Object.entries({ AGF_RETROSPECTIVE_STATE: previous.state, AGF_CONFIG: previous.config, AGF_RETROSPECTIVE_NO_SPAWN: previous.noSpawn })) { if (value === undefined) delete process.env[name]; else process.env[name] = value; }
  }
});

test("native setup rejects unsupported versions and missing capabilities before staging", async () => {
  const harness = fixtureCLI(); delete process.env.AGF_PLUGIN_FAKE_HEALTH;
  const previous = process.env.AGF_RETROSPECTIVE_STATE;
  process.env.AGF_RETROSPECTIVE_STATE = path.join(harness.root, "state");
  const context = { app: { version: "2.0.30" }, location: { directory: harness.root },
    session: { async get() {}, async hook() {} }, tool: { async hook() {} }, event: { async *subscribe() {} } };
  try {
    for (const version of ["2.0.25", "2.0.26", "2.0.28", "2.0.32", "2.1.0", "3.0.0", "2.0.30-dev", "garbage"]) {
      await assert.rejects(nativePlugin.setup({ ...context, app: { version } } as any), /unsupported.*docs\/runbooks\/task-retrospective\.md/);
    }
    for (const domain of ["session", "tool", "event"]) {
      await assert.rejects(nativePlugin.setup({ ...context, [domain]: {} } as any), /capabilities.*docs\/runbooks\/task-retrospective\.md/);
    }
    assert.equal(capturesOf(harness.captureFile).length, 0);
    const reports = readdirSync(path.join(harness.root, "state", "observer-errors"));
    assert.equal(reports.length, 11);
    assert.ok(reports.every(file => JSON.parse(readFileSync(path.join(harness.root, "state", "observer-errors", file), "utf8")).detail.includes("before staging")));
  } finally { harness.restore(); if (previous === undefined) delete process.env.AGF_RETROSPECTIVE_STATE; else process.env.AGF_RETROSPECTIVE_STATE = previous; }
});

test("native 2.0.30 public queued event fixture retains intake without inventing execution", async () => {
  const harness = fixtureCLI(); delete process.env.AGF_PLUGIN_FAKE_HEALTH;
  const events = loadEvents("native-2.0.30.jsonl") as any[];
  const id = events[0].data.sessionID;
  try {
    const cleanup = await nativePlugin.setup({ app: { version: "2.0.30" }, location: { directory: harness.root },
      session: { async get() {}, async hook() { return { async dispose() {} }; } },
      tool: { async hook() { return { async dispose() {} }; } },
      event: { async *subscribe() {
        for (const event of events) yield event;
        // Synthetic flush trigger only: recorded queued input ran no model/tool.
        yield { type: "session.execution.succeeded", data: { sessionID: id } };
      } },
    } as any);
    await cleanup();
    const envelope = JSON.parse(capturesOf(harness.captureFile).at(-1)!.stdin) as Envelope;
    assert.equal(envelope.sdk_version, "2.0.30");
    assert.ok(envelope.records.some(row => row.text === "Disposable queued input 0"));
    assert.ok(!envelope.records.some(row => row.role === "assistant" || row.part_type === "tool" || row.kind === "source"));
  } finally { harness.restore(); }
});

for (const nativeVersion of ["2.0.24", "2.0.27"]) test(`actual ${nativeVersion} default loader consumes recorded native stream and message-scoped call IDs`, async () => {
  const harness = fixtureCLI();
  delete process.env.AGF_PLUGIN_FAKE_HEALTH;
  const events = loadEvents(`native-${nativeVersion}.jsonl`) as any[];
  // Duplicate tool ID in another assistant message is a distinct attempt.
  const repeated = events.slice(3, 6).map((event) => ({ ...event, data: { ...event.data, assistantMessageID: "msg_second_attempt" } }));
  events.splice(6, 0, ...repeated);
  events.splice(7, 0, { type: "session.reasoning.delta", data: { sessionID: events[0].data.sessionID, text: "HIDDEN-CANARY" } });
  let disposed = false;
  try {
    const cleanup = await nativePlugin.setup({
      app: { version: nativeVersion }, location: { directory: path.join(harness.root, "proj") },
      session: { async get() { throw new Error("unused identity lookup"); }, async hook() { return { async dispose() {} }; } },
      event: { async *subscribe() { for (const event of events) yield event; } },
      tool: { async hook() { return { async dispose() { disposed = true; } }; } },
    } as any);
    await cleanup();
    const captures = capturesOf(harness.captureFile);
    assert.ok(captures.length > 0, "native terminal did not ingest");
    const envelope = JSON.parse(captures.at(-1)!.stdin) as Envelope;
    assert.equal(envelope.sdk_version, nativeVersion);
    assert.equal(envelope.trigger, true);
    assert.ok(envelope.records.some((record) => record.text === "QA_OBSERVER_DONE"));
    const completed = envelope.records.filter((record) => record.tool_status === "completed");
    assert.equal(new Set(completed.map((record) => record.call_id)).size, 2);
    assert.ok(!JSON.stringify(envelope).includes("HIDDEN-CANARY"));
    assert.ok(disposed);
  } finally { harness.restore(); }
});

function capturesOf(file: string): CaptureRow[] {
  let body: string;
  try {
    body = readFileSync(file, "utf8");
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") {
      return [];
    }
    throw error;
  }
  if (body.length === 0) {
    return [];
  }
  return body.trim().split("\n").map((line) => JSON.parse(line) as CaptureRow);
}

interface PluginHarness {
  hooks: any;
  captureFile: string;
  root: string;
  restore(): void;
}

let savedEnv: string | undefined;

function snapshotEnv(name: string): void {
  savedEnv = process.env[name];
}

function restoreEnv(name: string): void {
  if (savedEnv === undefined) {
    delete process.env[name];
  } else {
    process.env[name] = savedEnv;
  }
}

function fixtureCLI(exit = 0, version = SUPPORTED_VERSION): PluginHarness {
  const root = mkdtempSync(path.join(os.tmpdir(), "agf-retro-hook-"));
  mkdirSync(path.join(root, "proj"), { recursive: true });
  const previous = {
    bin: process.env.AGF_BIN,
    capture: process.env.AGF_CAPTURE_FILE,
    exit: process.env.AGF_CAPTURE_EXIT,
    health: process.env.AGF_PLUGIN_FAKE_HEALTH,
  };
  const captureFile = installCaptureCLI(root, exit);
  process.env.AGF_CAPTURE_FILE = captureFile;
  process.env.AGF_PLUGIN_FAKE_HEALTH = JSON.stringify({ healthy: true, version });
  const pending = RetrospectivePlugin({
    serverUrl: new URL("http://127.0.0.1:4096"),
    directory: path.join(root, "proj"),
  } as any);
  return {
    // RetrospectivePlugin returns a promise; tests await it.
    get hooks() {
      return pending;
    },
    captureFile,
    root,
    restore() {
      process.env.AGF_BIN = previous.bin;
      process.env.AGF_CAPTURE_FILE = previous.capture;
      process.env.AGF_CAPTURE_EXIT = previous.exit;
      process.env.AGF_PLUGIN_FAKE_HEALTH = previous.health;
    },
  };
}

async function withPlugin(exit: number, fn: (hooks: any, harness: PluginHarness) => Promise<void>, version?: string): Promise<void> {
  const harness = fixtureCLI(exit, version);
  try {
    const hooks = await harness.hooks;
    await fn(hooks, harness);
  } finally {
    harness.restore();
  }
}

async function replay(hooks: any, events: unknown[]): Promise<void> {
  for (const event of events) {
    await hooks.event({ event });
  }
}

function envelopesOf(harness: PluginHarness): Envelope[] {
  return capturesOf(harness.captureFile).map((row) => JSON.parse(row.stdin) as Envelope);
}

function argvOf(harness: PluginHarness): string[] | undefined {
  const rows = capturesOf(harness.captureFile);
  return rows.length > 0 ? rows[0].argv : undefined;
}

describe("retrospective opencode adapter replay", () => {
  test("concurrent batching and idle preserve opening records and terminal trigger", async () => {
    await withPlugin(0, async (hooks, harness) => {
      const sessionID = "ses_concurrent";
      await hooks.event({ event: { type: "session.created", properties: { info: { id: sessionID, directory: harness.root, title: "opening" } } } });
      const pending = [hooks.event({ event: { type: "message.updated", properties: { info: { id: "msg_user", sessionID, role: "user" } } } })];
      for (let n = 0; n < 60; n++) pending.push(hooks.event({ event: { type: "message.part.updated", properties: { part: { id: `part_${n}`, sessionID, messageID: "msg_user", type: "text", text: `observable ${n}` } } } }));
      pending.push(hooks.event({ event: { type: "session.idle", properties: { sessionID } } }));
      await Promise.all(pending);
      const envelopes = envelopesOf(harness);
      const parts = new Set(envelopes.flatMap(envelope => envelope.records).filter(record => record.kind === "part").map(record => record.record_id));
      assert.equal(parts.size, 60);
      assert.ok(envelopes.some(envelope => envelope.trigger));
      assert.ok(envelopes.flatMap(envelope => envelope.records).some(record => record.record_id === "msg_user"));
      assert.ok(envelopes.every(envelope => envelope.records.some(record => record.kind === "session")));
    });
  });
  test("streamed text updates collapse to the cumulative final part", async () => {
    await withPlugin(0, async (hooks, harness) => {
      await replay(hooks, loadEvents("streamed.jsonl"));
      const envelopes = envelopesOf(harness);
      assert.equal(envelopes.length, 1, "threshold flushes never happen below 25 records");
      const [envelope] = envelopes;
      assert.equal(envelope.version, 1);
      assert.equal(envelope.harness, "opencode");
      assert.equal(envelope.session_id, "ses_stream");
      assert.equal(envelope.trigger, true);
      assert.equal(envelope.project_dir, path.join(harness.root, "proj"));
      const parts = envelope.records.filter((record) => record.kind === "part" && record.part_type === "text");
      assert.equal(parts.length, 5, "one part record per part id (user + 3 growing assistant + final)");
      const last = parts.at(-1);
      assert.equal(last?.text, "Onboarding instructions summarized.");
    });
  });

  test("duplicate/out-of-order terminal events stay idempotent handoffs", async () => {
    await withPlugin(0, async (hooks, harness) => {
      await replay(hooks, loadEvents("duplicates.jsonl"));
      const envelopes = envelopesOf(harness);
      assert.equal(envelopes.length, 2, "one flush per terminal idle");
      const first = envelopes[0].records;
      assert.equal(
        first.filter((record) => record.record_id === "prt_tool_1").length,
        3,
        "running + terminal + duplicate redelivery all forward (ingest upserts)",
      );
      const second = envelopes[1].records;
      assert.deepEqual(second.map((record) => record.kind), ["session", "part", "event"]);
      assert.equal(second[1].record_id, "prt_tool_1");
      assert.equal(second[1].tool_status, "completed");
    });
  });

  test("root/child aggregation routes child rows to the owner root", async () => {
    await withPlugin(0, async (hooks, harness) => {
      await replay(hooks, loadEvents("childsession.jsonl"));
      const envelopes = envelopesOf(harness);
      assert.equal(envelopes.length, 2, "child trigger first, then the root");
      assert.equal(envelopes[0].session_id, "ses_child");
      assert.equal(envelopes[1].session_id, "ses_root");
      const rootRows = envelopes[1].records.filter((record) => record.kind === "session");
      const childRow = rootRows.find((record) => record.session_id === "ses_child");
      assert.ok(childRow, "the root buffer carries the child identity row");
      assert.equal(childRow?.parent_id, "ses_root");
    });
  });

  test("abort/error then idle keeps the transport error as a bounded detail", async () => {
    await withPlugin(0, async (hooks, harness) => {
      await replay(hooks, loadEvents("abort-then-idle.jsonl"));
      const [envelope] = envelopesOf(harness);
      const errors = envelope.records.filter((record) => record.kind === "event" && record.event === "session.error");
      assert.equal(errors.length, 1);
      assert.equal(errors[0].detail, "MessageAbortedError");
      assert.equal(errors[0].session_id, "ses_abort");
    });
  });

  test("multi-turn task boundaries forward every turn marker", async () => {
    await withPlugin(0, async (hooks, harness) => {
      await replay(hooks, loadEvents("multiturn.jsonl"));
      const [envelope] = envelopesOf(harness);
      const users = envelope.records.filter(
        (record) => record.kind === "message" && record.role === "user",
      );
      assert.equal(users.length, 3);
    });
  });

  test("history compaction is forwarded as a session event before capture", async () => {
    await withPlugin(0, async (hooks, harness) => {
      await replay(hooks, loadEvents("compaction.jsonl"));
      const [envelope] = envelopesOf(harness);
      assert.ok(envelope.records.some((record) => record.kind === "event" && record.event === "session.compacted"));
      assert.ok(
        envelope.records.some(
          (record) => record.kind === "part" && record.record_id === "prt_tool_1",
        ),
        "pre-compaction evidence stays staged",
      );
    });
  });

  test("restart replays only post-restart records (no duplicated staging)", async () => {
    const pre = loadEvents("restart-pre.jsonl");
    const post = loadEvents("restart-post.jsonl");
    await withPlugin(0, async (hooks, harness) => {
      await replay(hooks, pre);
      assert.equal(envelopesOf(harness).length, 0, "no idle in the pre-restart window: nothing flushed");
    });
    // The server restart: a fresh plugin instance replays the remaining
    // events; the ingest spool (Go side) keeps the pre-restart upserts.
    await withPlugin(0, async (hooks, harness) => {
      await replay(hooks, post);
      const [second] = envelopesOf(harness);
      assert.ok(second, "the post-restart idle flushes once");
      assert.equal(second.records.filter((record) => record.record_id === "prt_tool_1").length, 0);
      assert.equal(second.records.filter((record) => record.record_id === "prt_tool_2").length, 1);
    });
  });

  test("learning-session suppression never stages a correction session", async () => {
    await withPlugin(0, async (hooks, harness) => {
      await replay(hooks, loadEvents("learning-suppressed.jsonl"));
      assert.equal(envelopesOf(harness).length, 0, "no envelope for a learning-origin session or child");
    });
  });

  test("the synthetic success fixture forwards wrong-directory calls and recovery", async () => {
    await withPlugin(0, async (hooks, harness) => {
      await replay(hooks, loadEvents("success-recovery.jsonl"));
      const records = envelopesOf(harness).flatMap((envelope) => envelope.records);
      const wrongs = records.filter((record) => (record.call_id ?? "").startsWith("call_wrong_"));
      assert.equal(wrongs.length, 6, "three wrong reads and three wrong writes");
      assert.ok(wrongs.every((record) => (record.input ?? "").includes("{{wrong_root}}")));
      const rights = records.filter((record) => record.record_id === "prt_righ_r1");
      assert.equal(rights.length, 1, "the recovery read arrives once");
      const idle = records.filter((record) => record.kind === "event" && record.event === "session.idle");
      assert.equal(idle.length, 1, "a single terminal trigger");
    });
  });

  test("source provenance hashes instruction files at use time", async () => {
    await withPlugin(0, async (hooks, harness) => {
      const instruction = path.join(harness.root, "proj", "AGENTS.md");
      writeFileSync(instruction, "the instruction bytes\n");
      await hooks["tool.execute.after"](
        { tool: "read", sessionID: "ses_src", callID: "call_src", args: { filePath: instruction } },
        { title: "read", output: "the instruction bytes\n", metadata: {} },
      );
      await hooks.event({ event: { type: "session.idle", properties: { sessionID: "ses_src" } } });
      const [envelope] = envelopesOf(harness);
      const record = envelope.records.find((row) => row.kind === "source");
      assert.ok(record, "a source record flushed");
      assert.equal(record?.path, instruction);
      assert.equal(record?.sha256, createHash("sha256").update("the instruction bytes\n").digest("hex"));
    });
  });

  test("secret-file candidates are source gaps, never hashed or staged", async () => {
    await withPlugin(0, async (hooks, harness) => {
      const envFile = path.join(harness.root, "proj", ".env");
      writeFileSync(envFile, "SUPERSECRET-CANARY=1\n");
      await hooks["tool.execute.after"](
        { tool: "read", sessionID: "ses_src2", callID: "call_src2", args: { filePath: envFile } },
        { title: "read", output: "SUPERSECRET-CANARY=1\n", metadata: {} },
      );
      await hooks.event({ event: { type: "session.idle", properties: { sessionID: "ses_src2" } } });
      const [envelope] = envelopesOf(harness);
      const record = envelope.records.find((row) => row.path === envFile);
      assert.ok(record);
      assert.equal(record?.kind, "source_gap");
      assert.ok(!JSON.stringify(envelope).includes("SUPERSECRET-CANARY"));
    });
  });

  test("unsupported server versions degrade to inert (no staging)", async () => {
    await withPlugin(0, async (hooks, harness) => {
      await replay(hooks, loadEvents("streamed.jsonl"));
      assert.equal(envelopesOf(harness).length, 0, "no staging for version drift");
    }, "1.18.32");
  });

  test("a disabled ingest verdict stops staging until the next trigger", async () => {
    await withPlugin(2, async (hooks, harness) => {
      await replay(hooks, loadEvents("streamed.jsonl"));
      assert.equal(envelopesOf(harness).length, 1, "the probe flush still reaches the CLI");
      await replay(hooks, loadEvents("restart-post.jsonl"));
      assert.equal(envelopesOf(harness).length, 1, "disabled staging is dropped");
    });
  });

  test("the spawn never carries transcript data in argv", async () => {
    await withPlugin(0, async (hooks, harness) => {
      await replay(hooks, loadEvents("streamed.jsonl"));
      const argv = argvOf(harness);
      assert.deepEqual(argv, ["retrospective", "ingest", "--harness", "opencode"]);
      const row = capturesOf(harness.captureFile)[0];
      assert.ok(row.stdin.includes("ses_stream"), "the envelope travels on stdin");
      assert.ok(!argv!.join(" ").includes("ses_stream"));
    });
  });
});
