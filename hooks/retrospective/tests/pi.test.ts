import assert from "node:assert/strict";
import { test } from "node:test";
import { createHash } from "node:crypto";
import { mkdtempSync, mkdirSync, writeFileSync, symlinkSync, chmodSync, readdirSync, readFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { tmpdir } from "node:os";
import path from "node:path";
import { PiObserver, registerPiObserver, piRevision, piSnapshot, type PiEnvelope } from "../pi.js";

// Protocol seams, not provider/runtime acceptance. Installed SDK checks below
// separately verify native persistence and extension loading without a model.
const sha = (s: string) => createHash("sha256").update(s).digest("hex");
const credentialPaths = ["auth.json", ".npmrc", ".netrc", ".git-credentials", ".pypirc", "token.json", "tokens.json", "access-token.json", "refresh_token.json", "client.key", ".keys/material.txt", "keys/material.txt", ".tokens/session.txt", "tokens/session.txt"];
function placeholder(root: string, file: string, value: string) {
  mkdirSync(path.dirname(path.join(root, file)), { recursive: true }); writeFileSync(path.join(root, file), value);
}
function fixture() {
  const root = mkdtempSync(path.join(tmpdir(), "agf-pi-"));
  mkdirSync(path.join(root, ".git")); writeFileSync(path.join(root, ".git", "HEAD"), "a".repeat(40));
  writeFileSync(path.join(root, "AGENTS.md"), "Use bounded tests.\n");
  const entries: any[] = [];
  let session = "pi-native-session", title = "task", parent: string | undefined;
  const ctx = { cwd: root, sessionManager: { getSessionId: () => session, getEntries: () => entries, getHeader: () => ({ parentSession: parent }), getSessionName: () => title } };
  const sent: PiEnvelope[] = [];
  const observer = new PiObserver(async (_args, input) => { sent.push(structuredClone(input as PiEnvelope)); return { code: 0, stdout: "" }; }, "b".repeat(64));
  const handlers = new Map<string, (event: any, ctx: any) => any>();
  registerPiObserver({ on: (name, fn) => { handlers.set(name, fn); } }, observer);
  const emit = async (name: string, event: any = {}) => handlers.get(name)?.({ type: name, ...event }, ctx);
  const prompt = async (content = "Read AGENTS.md", source = "interactive") => {
    await emit("input", { text: content, source });
    const message = { role: "user", content: [{ type: "text", text: content }], timestamp: 123 };
    await emit("message_start", { message });
    entries.push({ type: "message", id: "user-native", message });
  };
  return { root, ctx, observer, sent, entries, emit, prompt, handlers, setTitle: (value: string) => title = value, setParent: (value: string) => parent = value, setSession: (value: string) => session = value };
}

test("Pi protocol: actual native IDs, exact prompt revision, terminal read and authoritative settle", async () => {
  const f = fixture(); await f.emit("session_start"); await f.prompt();
  const args = { path: "AGENTS.md" };
  f.entries.push({ type: "message", id: "assistant-native", message: { role: "assistant", content: [{ type: "thinking", thinking: "PRIVATE REASONING" }, { type: "toolCall", id: "call-native", name: "read", arguments: args }] } });
  await f.emit("tool_execution_start", { toolName: "read", toolCallId: "call-native", args });
  await f.emit("tool_execution_end", { toolName: "read", toolCallId: "call-native", isError: false, result: { content: [{ type: "text", text: "Use bounded tests." }] } });
  assert.equal(f.handlers.has("agent_end"), false);
  assert.equal(f.sent.some(e => e.trigger), false);
  await f.emit("agent_settled");
  const rows = f.sent.flatMap(e => e.records);
  const revision = rows.find(r => r.kind === "source_revision");
  assert.equal(revision?.record_id, "user-native"); assert.equal(revision?.sha256, sha("Read AGENTS.md")); assert.equal(revision?.revision, "a".repeat(40));
  const terminal = rows.find(r => r.part_type === "tool" && r.tool_status === "completed");
  assert.equal(terminal?.record_id, "call-native"); assert.equal(terminal?.message_id, "assistant-native"); assert.equal(terminal?.output, "Use bounded tests.");
  const bracket = rows.find(r => r.kind === "read_bracket");
  assert.equal(bracket?.call_id, "call-native"); assert.equal(bracket?.read_bracket.runtime, "pi:1.0.0");
  assert.deepEqual(bracket?.read_bracket.before, bracket?.read_bracket.after);
  assert.equal(bracket?.read_bracket.before.metadata.length, 8);
  assert.equal(JSON.stringify(rows).includes("PRIVATE REASONING"), false);
  assert.equal(f.sent.at(-1)?.trigger, true); assert.equal(rows.filter(r => r.event === "session.idle").length, 1);
});

test("Pi protocol: retry/queued intermediate turns are not idle; cancelled tools remain errors", async () => {
  const f = fixture(); await f.emit("session_start"); await f.prompt();
  f.entries.push({ type: "message", id: "assistant-error", message: { role: "assistant", stopReason: "error", content: [{ type: "toolCall", id: "cancelled-call", name: "bash", arguments: { command: "false" } }] } });
  await f.emit("turn_end"); await f.emit("agent_end");
  assert.equal(f.sent.some(e => e.trigger), false);
  await f.emit("tool_execution_end", { toolName: "bash", toolCallId: "cancelled-call", isError: true, result: { content: [{ type: "text", text: "cancelled" }] } });
  await f.emit("input", { text: "queued next", source: "rpc", streamingBehavior: "followUp" });
  await f.emit("agent_settled");
  assert.equal(f.sent.flatMap(e => e.records).find(r => r.tool_status === "error")?.output, "cancelled");
  assert.equal(f.sent.filter(e => e.trigger).length, 1);
});

test("Pi protocol: compaction preserves history identity; late attach has explicit gap", async () => {
  const f = fixture(); f.entries.push({ type: "message", id: "old", message: { role: "user", content: "old history" } });
  await f.emit("session_start");
  f.entries.push({ type: "compaction", id: "compact-native", summary: "NOT NEW HUMAN INTENT" });
  await f.emit("agent_settled");
  const rows = f.sent.flatMap(e => e.records);
  assert(rows.some(r => r.kind === "removed")); assert(rows.some(r => r.event === "session.compacted" && r.record_id === "compact-native"));
  assert.equal(JSON.stringify(rows).includes("NOT NEW HUMAN INTENT"), false);
  assert.equal(rows.some(r => r.record_id === "old"), false);
});

test("Pi protocol: learning origins and unresolved fork ancestry suppress all capture/context", async () => {
  for (const kind of ["title", "fork"]) {
    const f = fixture();
    if (kind === "title") f.setTitle("agf-learning-origin:v1:repair"); else f.setParent("/private/parent.jsonl");
    await f.emit("session_start"); await f.prompt(); await f.emit("agent_settled");
    assert.equal(f.sent.length, 0);
  }
});

for (const source of ["unknown", undefined, "transformed", "ambiguous", "no-input"]) test(`Pi input source ${source ?? "absent"}: nonhuman, gapped, no logical prompt boundary/provenance`, async () => {
  const f = fixture(); await f.emit("session_start");
  if (source !== "no-input") await f.emit("input", { text: source === "transformed" ? "original" : "automated", ...(source === undefined ? {} : { source: ["transformed", "ambiguous"].includes(source) ? "interactive" : source }) });
  if (source === "ambiguous") await f.emit("input", { text: "automated", source: "rpc" });
  const message = { role: "user", timestamp: 123, content: [{ type: "text", text: "automated" }] };
  await f.emit("message_start", { message }); f.entries.push({ type: "message", id: "nonhuman-native", message });
  await f.emit("agent_settled");
  const rows = f.sent.flatMap(e => e.records);
  assert.equal(rows.find(r => r.part_type === "text")?.synthetic, true);
  assert.equal(rows.find(r => r.kind === "message")?.role, "context");
  assert.equal(rows.some(r => r.kind === "message" && r.role === "user"), false, "no human prompt/task-opening record");
  assert.equal(rows.some(r => r.kind === "source_revision"), false);
  assert(rows.some(r => r.kind === "removed" && r.detail.includes("input source")));
});

for (const source of ["interactive", "rpc", "extension"]) test(`Pi known ${source} transport retains logical input without asserting human origin`, async () => {
  const f = fixture(); await f.emit("session_start"); await f.prompt("logical task data", source); await f.emit("agent_settled");
  const rows = f.sent.flatMap(e => e.records);
  assert.equal(rows.find(r => r.part_type === "text")?.synthetic, false);
  assert.equal(rows.find(r => r.kind === "message")?.role, "user");
  assert.equal(rows.find(r => r.kind === "message")?.input_source, source);
  assert.equal(rows.find(r => r.part_type === "text")?.text, "logical task data");
  assert.equal(rows.some(r => r.origin === "human" || r.authority === "human"), false);
  assert(rows.some(r => r.kind === "source_revision")); assert.equal(rows.some(r => r.kind === "removed"), false);
});

test("Pi native custom messages are not logical prompt boundaries even with pending extension input", async () => {
  const f = fixture(); await f.emit("session_start"); await f.emit("input", { text: "advisory", source: "extension" });
  f.entries.push({ type: "custom_message", id: "custom-native", customType: "advisory", content: "advisory", display: false });
  await f.emit("agent_settled");
  const rows = f.sent.flatMap(e => e.records);
  assert.equal(rows.some(r => r.kind === "message" && r.role === "user"), false);
  assert.equal(rows.some(r => r.kind === "source_revision"), false);
});

test("Pi native learning-origin custom marker suppresses known extension prompts", async () => {
  const f = fixture(); f.entries.push({ type: "custom", id: "origin-native", customType: "agf-learning-origin" });
  await f.emit("session_start"); await f.prompt("learning automation", "extension"); await f.emit("agent_settled");
  assert.equal(f.sent.length, 0);
});

test("Installed PiChamber 1.0.5 plain bridge: extension-transport prompt reaches idle without human-origin claims (contract probe, no provider)", async t => {
  const base = path.join(process.env.HOME ?? "", ".npm-global/lib/node_modules/@pi-chamber/web");
  let source: string;
  try { source = readFileSync(path.join(base, "server/lib/pi/session-daemon/session-daemon.js"), "utf8"); } catch { t.skip("installed PiChamber bridge unavailable"); return; }
  assert.equal(JSON.parse(readFileSync(path.join(base, "package.json"), "utf8")).version, "1.0.5");
  const bridge = source.slice(source.indexOf("promptPromise = isSlashPrompt"), source.indexOf("} catch (error)", source.indexOf("promptPromise = isSlashPrompt")));
  assert.match(bridge, /\? activeRuntime\.session\.prompt\(text,\s*\{\s*source: 'rpc'/);
  const plain = bridge.match(/: activeRuntime\.session\.prompt\(text,\s*\{\s*expandPromptTemplates: false,\s*source: '(extension)'/);
  assert(plain, "installed plain UI bridge must expose the exact pinned extension transport contract");
  const f = fixture(); await f.emit("session_start"); await f.prompt("Unmodified plain UI task", plain[1]); await f.emit("agent_settled");
  const rows = f.sent.flatMap(e => e.records), input = rows.find(r => r.kind === "message" && r.role === "user");
  assert.equal(input?.input_source, "extension", "automation-capable transport, not proven human origin");
  assert.equal(rows.find(r => r.part_type === "text")?.text, "Unmodified plain UI task");
  assert.equal(rows.find(r => r.part_type === "text")?.synthetic, false);
  assert.equal(rows.find(r => r.kind === "source_revision")?.sha256, sha("Unmodified plain UI task"));
  assert.equal(rows.some(r => r.kind === "removed"), false);
  assert.equal(rows.filter(r => r.event === "session.idle").length, 1); assert.equal(f.sent.at(-1)?.trigger, true);
  assert.equal(rows.some(r => r.origin === "human" || r.authority === "human"), false);
});

for (const boundary of ["terminal-event", "persisted-result"]) test(`Pi secret alias: ${boundary} never serializes actual target bytes`, async () => {
  const f = fixture(), canary = "PI_SECRET_ALIAS_CANARY_NEVER_FORWARD";
  writeFileSync(path.join(f.root, ".env"), canary); symlinkSync(path.join(f.root, ".env"), path.join(f.root, "innocent-alias"));
  await f.emit("session_start"); await f.prompt();
  const args = { path: "innocent-alias" };
  f.entries.push({ type: "message", id: "alias-assistant", message: { role: "assistant", content: [{ type: "toolCall", id: "alias-read", name: "read", arguments: args }] } });
  await f.emit("tool_execution_start", { toolName: "read", toolCallId: "alias-read", args });
  const content = [{ type: "text", text: readFileSync(path.join(f.root, "innocent-alias"), "utf8") }];
  if (boundary === "terminal-event") await f.emit("tool_execution_end", { toolName: "read", toolCallId: "alias-read", isError: false, result: { content } });
  else { f.entries.push({ type: "message", id: "alias-result", message: { role: "toolResult", toolCallId: "alias-read", content, isError: false } }); await f.emit("turn_end"); }
  await f.emit("agent_settled");
  assert.equal(JSON.stringify(f.sent).includes(canary), false);
  const rows = f.sent.flatMap(e => e.records);
  assert(rows.some(r => r.kind === "removed" && r.record_id === "alias-read:output-safety-gap"));
  assert.equal(rows.find(r => r.tool_status === "completed")?.output, undefined);
});

test("Pi unresolved read target withholds unknown error bytes but retains failure state", async () => {
  const f = fixture(); await f.emit("session_start"); await f.prompt();
  const args = { path: "missing" };
  f.entries.push({ type: "message", id: "missing-assistant", message: { role: "assistant", content: [{ type: "toolCall", id: "missing-read", name: "read", arguments: args }] } });
  await f.emit("tool_execution_start", { toolName: "read", toolCallId: "missing-read", args });
  await f.emit("tool_execution_end", { toolName: "read", toolCallId: "missing-read", isError: true, result: { content: [{ type: "text", text: "UNKNOWN_ERROR_SECRET_CANARY" }] } });
  assert.equal(JSON.stringify(f.sent).includes("UNKNOWN_ERROR_SECRET_CANARY"), false);
  assert(f.sent.flatMap(e => e.records).some(r => r.tool_status === "error" && r.error.includes("unresolved")));
});

test("Pi credential names: no snapshots for private basenames/suffixes/directories or aliases; ordinary catalogs remain observable", () => {
  const f = fixture();
  for (const [index, file] of credentialPaths.entries()) {
    placeholder(f.root, file, "PLACEHOLDER_PRIVATE_DATA");
    symlinkSync(path.join(f.root, file), path.join(f.root, `ordinary-alias-${index}`));
    assert.equal(piSnapshot({ path: file }, f.root), undefined, "private basename/directory must not acquire a digest");
    assert.equal(piSnapshot({ path: `ordinary-alias-${index}` }, f.root), undefined, "resolved credential target must not acquire a digest");
  }
  for (const file of ["catalog.json", "config.json", "tokenizer.json", "auth.test.ts"]) {
    placeholder(f.root, file, "ordinary catalog fixture");
    assert.equal(piSnapshot({ path: file }, f.root)?.sha256, sha("ordinary catalog fixture"));
  }
});

for (const boundary of ["terminal-event", "persisted-result"]) test(`Pi credential names: ${boundary} withholds placeholder bytes and digests even for regular readable files`, async () => {
  const f = fixture(), canary = "PLACEHOLDER_CREDENTIAL_NAMES_CANARY";
  await f.emit("session_start"); await f.prompt();
  for (const [index, file] of credentialPaths.entries()) {
    placeholder(f.root, file, canary);
    const alias = `ordinary-alias-${index}`; symlinkSync(path.join(f.root, file), path.join(f.root, alias));
    for (const requested of [file, alias]) {
      const id = `private-call-${index}-${requested === file ? "direct" : "alias"}`, args = { path: requested };
      f.entries.push({ type: "message", id: `${id}-assistant`, message: { role: "assistant", content: [{ type: "toolCall", id, name: "read", arguments: args }] } });
      await f.emit("tool_execution_start", { toolName: "read", toolCallId: id, args });
      const content = [{ type: "text", text: readFileSync(path.join(f.root, requested), "utf8") }];
      if (boundary === "terminal-event") await f.emit("tool_execution_end", { toolName: "read", toolCallId: id, isError: false, result: { content } });
      else { f.entries.push({ type: "message", id: `${id}-result`, message: { role: "toolResult", toolCallId: id, content, isError: false } }); await f.emit("turn_end"); }
    }
  }
  const serialized = JSON.stringify(f.sent);
  assert.equal(serialized.includes(canary), false, "no private bytes in any envelope");
  assert.equal(serialized.includes(sha(canary)), false, "no private file digest in any envelope");
  assert(f.sent.flatMap(e => e.records).some(r => r.record_id?.endsWith(":output-safety-gap")));
});

test("Pi provenance: mutation, failed reads, secrets, aliases and unknown arguments abstain", async () => {
  const f = fixture(); await f.emit("session_start"); await f.prompt();
  const args = { path: "AGENTS.md" };
  f.entries.push({ type: "message", id: "assistant", message: { role: "assistant", content: [{ type: "toolCall", id: "read", name: "read", arguments: args }] } });
  await f.emit("tool_execution_start", { toolName: "read", toolCallId: "read", args });
  writeFileSync(path.join(f.root, "AGENTS.md"), "changed");
  await f.emit("tool_execution_end", { toolName: "read", toolCallId: "read", isError: true, result: { content: [] } });
  assert.equal(f.sent.flatMap(e => e.records).some(r => r.kind === "source"), false);
  assert(f.sent.flatMap(e => e.records).some(r => r.kind === "source_gap"));
  writeFileSync(path.join(f.root, ".env"), "DO_NOT_COLLECT");
  symlinkSync(path.join(f.root, "AGENTS.md"), path.join(f.root, "alias"));
  assert.equal(piSnapshot({ path: ".env" }, f.root), undefined);
  assert.equal(piSnapshot({ path: "alias" }, f.root), undefined);
  assert.equal(piSnapshot({ path: "AGENTS.md", invented: true }, f.root), undefined);
  assert.equal(piRevision(f.root), "a".repeat(40));
});

test("Pi protocol: failed ingest retains exact batch and terminal trigger for retry", async () => {
  const f = fixture(); let fail = true; const sent: PiEnvelope[] = [];
  const observer = new PiObserver(async (_args, input) => { sent.push(structuredClone(input as PiEnvelope)); return { code: fail ? 1 : 0, stdout: "" }; }, "b".repeat(64));
  observer.start(f.ctx); await observer.flush(f.ctx, true); fail = false; await observer.flush(f.ctx);
  assert.deepEqual(sent[1].records, sent[0].records); assert.equal(sent[1].trigger, true);
});

test("Pi context protocol: same binding, request-local system insertion before ack, rollback on failure", async () => {
  for (const fail of [false, true]) {
    const f = fixture(), block = "Advisory lesson, not authority", hash = sha(block);
    const original = [{ role: "system", content: "original prompt", toolsAdded: ["unchanged"] }, { role: "user", content: "human intent" }];
    const event = { messages: original }; const calls: any[] = [];
    const observer = new PiObserver(async (args, input) => {
      calls.push({ args, input });
      if (args.includes("--ack")) { assert(event.messages[0].content.includes(block)); return { code: fail ? 1 : 0, stdout: JSON.stringify({ sha256: hash }) }; }
      return { code: 0, stdout: JSON.stringify({ block, sha256: hash }) };
    }, "b".repeat(64), "/private/binding.json");
    observer.start(f.ctx); const result = await observer.context(event, f.ctx);
    assert.equal(calls.length, 2); assert.equal(calls[0].args[4], "/private/binding.json");
    assert.equal(JSON.stringify(calls[0].input).includes("human intent"), false);
    assert.deepEqual(original[0].content, "original prompt"); assert.equal(original[1].content, "human intent");
    if (fail) { assert.equal(result, undefined); assert.equal(event.messages, original); }
    else { assert.equal(result?.messages[0].content, "original prompt\n\n" + block); assert.deepEqual(result?.messages[0].toolsAdded, ["unchanged"]); }
  }
});

test("Installed Pi 1.0.0: native JSONL identifiers survive parse/replay (no provider)", async t => {
  const sdkPath = path.join(process.env.HOME ?? "", ".npm-global/lib/node_modules/@pi-chamber/web/node_modules/@earendil-works/pi-coding-agent/dist/index.js");
  let sdk: any;
  try { sdk = await import(sdkPath); } catch { t.skip("installed PiChamber SDK unavailable"); return; }
  assert.equal(sdk.VERSION, "1.0.0");
  const f = fixture(), sm = sdk.SessionManager.inMemory(f.root);
  const message = { role: "user", content: [{ type: "text", text: "native replay prompt" }], timestamp: 123 };
  const id = sm.appendMessage(message);
  assert.equal(sm.getEntries().find((e: any) => e.id === id)?.message.content[0].text, "native replay prompt");
  const parsed = sdk.parseSessionEntries(sm.getEntries().map((e: any) => JSON.stringify(e)).join("\n"));
  assert(parsed.some((e: any) => e.id === id));
});

test("Installed Pi 1.0.0: native jiti loader resolves own VERSION and registers extension (fake CLI, no provider)", async t => {
  const base = path.join(process.env.HOME ?? "", ".npm-global/lib/node_modules/@pi-chamber/web/node_modules/@earendil-works/pi-coding-agent/dist");
  let loader: any;
  try { loader = await import(path.join(base, "core/extensions/loader.js")); } catch { t.skip("installed PiChamber SDK unavailable"); return; }
  const f = fixture(), cli = path.join(f.root, "agf");
  writeFileSync(cli, "#!/usr/bin/env node\nprocess.stderr.write('agent-fabric: flag: help requested\\n');process.exitCode=1;\n"); chmodSync(cli, 0o700);
  const previous = process.env.AGF_BIN; process.env.AGF_BIN = cli;
  try {
    const result = await loader.loadExtensions([path.resolve(__dirname, "../../pi.ts")], f.root);
    assert.deepEqual(result.errors, []);
    assert.equal(result.extensions.length, 1);
    assert(result.extensions[0].handlers.has("agent_settled"));
    assert.equal(result.extensions[0].handlers.has("agent_end"), false);
    const { createJiti } = await import(path.join(base, "core/extensions/jiti-loader.js"));
    const nativeModule = await createJiti(__filename, { fsCache: false, moduleCache: false }).import(path.resolve(__dirname, "../../pi.ts"));
    placeholder(f.root, "auth.json", "PLACEHOLDER_NATIVE_LOADER_PRIVATE_FILE");
    assert.equal(nativeModule.piSnapshot({ path: "auth.json" }, f.root), undefined);
    placeholder(f.root, "catalog.json", "ordinary native loader catalog");
    assert.equal(nativeModule.piSnapshot({ path: "catalog.json" }, f.root)?.sha256, sha("ordinary native loader catalog"));
  } finally { if (previous === undefined) delete process.env.AGF_BIN; else process.env.AGF_BIN = previous; }
});

test("Pi collector -> real Go ingest: isolated Store retains native IDs/read brackets (no drain/provider)", async () => {
  const f = fixture(), binary = path.join(f.root, "agf"), config = path.join(f.root, "retrospective.json"), enabled = path.join(f.root, "settings.json"), state = path.join(f.root, "state");
  execFileSync("go", ["build", "-o", binary, "./cmd/agent-fabric"], { cwd: path.resolve(__dirname, "../../../.."), timeout: 60000, env: { ...process.env, GOPROXY: "off", GOSUMDB: "off" } });
  writeFileSync(config, JSON.stringify({ state_dir: state })); writeFileSync(enabled, JSON.stringify({ retrospective: { enabled: true } }));
  const wire: string[] = [];
  const observer = new PiObserver(async (args, input) => {
    assert.equal((input as PiEnvelope).trigger, false, "this check cannot start a detached drain");
    wire.push(JSON.stringify(input));
    const stdout = execFileSync(binary, [...args, "--config", config], { input: JSON.stringify(input), encoding: "utf8", timeout: 10000, env: { ...process.env, AGF_CONFIG: enabled, AGF_RETROSPECTIVE_CONFIG: config, AGF_RETROSPECTIVE_NO_SPAWN: "1" } });
    return { code: 0, stdout };
  }, "b".repeat(64));
  observer.start(f.ctx);
  observer.input({ text: "Read AGENTS.md", source: "interactive" }, f.ctx);
  const message = { role: "user", timestamp: 123, content: [{ type: "text", text: "Read AGENTS.md" }] };
  observer.prompt(message, f.ctx); f.entries.push({ type: "message", id: "real-input", message });
  const args = { path: "AGENTS.md", offset: 1000, limit: 1 };
  f.entries.push({ type: "message", id: "real-assistant", message: { role: "assistant", content: [{ type: "toolCall", id: "real-call", name: "read", arguments: args }] } });
  observer.toolStart({ toolName: "read", toolCallId: "real-call", args }, f.ctx);
  observer.toolEnd({ toolName: "read", toolCallId: "real-call", isError: true, result: { content: [{ type: "text", text: "Offset 1000 is beyond end of file" }] } }, f.ctx);
  const canary = "PI_PRIVATE_STORE_SECRET_ALIAS_CANARY";
  writeFileSync(path.join(f.root, ".env"), canary); symlinkSync(path.join(f.root, ".env"), path.join(f.root, "safe-looking-alias"));
  for (const boundary of ["terminal", "persisted"]) {
    const id = `secret-${boundary}`, args = { path: "safe-looking-alias" };
    f.entries.push({ type: "message", id: `${id}-assistant`, message: { role: "assistant", content: [{ type: "toolCall", id, name: "read", arguments: args }] } });
    observer.toolStart({ toolName: "read", toolCallId: id, args }, f.ctx);
    const content = [{ type: "text", text: readFileSync(path.join(f.root, args.path), "utf8") }];
    if (boundary === "terminal") observer.toolEnd({ toolName: "read", toolCallId: id, isError: false, result: { content } }, f.ctx);
    else f.entries.push({ type: "message", id: `${id}-result`, message: { role: "toolResult", toolCallId: id, content, isError: false } });
  }
  for (const [index, file] of credentialPaths.entries()) {
    placeholder(f.root, file, canary);
    const alias = `credential-alias-${index}`; symlinkSync(path.join(f.root, file), path.join(f.root, alias));
    for (const requested of [file, alias]) for (const boundary of ["terminal", "persisted"]) {
      const id = `credential-${index}-${requested === file ? "direct" : "alias"}-${boundary}`, args = { path: requested };
      f.entries.push({ type: "message", id: `${id}-assistant`, message: { role: "assistant", content: [{ type: "toolCall", id, name: "read", arguments: args }] } });
      observer.toolStart({ toolName: "read", toolCallId: id, args }, f.ctx);
      const content = [{ type: "text", text: readFileSync(path.join(f.root, requested), "utf8") }];
      if (boundary === "terminal") observer.toolEnd({ toolName: "read", toolCallId: id, isError: false, result: { content } }, f.ctx);
      else f.entries.push({ type: "message", id: `${id}-result`, message: { role: "toolResult", toolCallId: id, content, isError: false } });
      await observer.flush(f.ctx);
    }
  }
  observer.input({ text: "extension transport task", source: "extension" }, f.ctx);
  const extensionInput = { role: "user", timestamp: 455, content: [{ type: "text", text: "extension transport task" }] };
  observer.prompt(extensionInput, f.ctx); f.entries.push({ type: "message", id: "extension-store", message: extensionInput });
  observer.input({ text: "unproven-source", source: "unknown" }, f.ctx);
  const nonhuman = { role: "user", timestamp: 456, content: [{ type: "text", text: "unproven-source" }] };
  observer.prompt(nonhuman, f.ctx); f.entries.push({ type: "message", id: "nonhuman-store", message: nonhuman });
  await observer.flush(f.ctx);
  const files = readdirSync(path.join(state, "spool")).filter(p => p.endsWith(".json"));
  assert.equal(files.length, 1);
  const stored = JSON.parse(readFileSync(path.join(state, "spool", files[0]), "utf8"));
  assert.equal(stored.harness, "pi");
  assert.equal(stored.parts["real-call"].tool_status, "error");
  assert.equal(stored.read_brackets["real-call"].observation.runtime, "pi:1.0.0");
  assert.equal(stored.prompt_revisions["real-input"].prompt_sha256, sha("Read AGENTS.md"));
  assert.equal(stored.messages["extension-store"].role, "user");
  assert.equal(stored.messages["extension-store"].input_source, "extension");
  assert.equal(stored.parts["extension-store:text"].synthetic, undefined);
  assert.equal(stored.parts["extension-store:text"].text, "extension transport task");
  assert.equal(stored.prompt_revisions["extension-store"].prompt_sha256, sha("extension transport task"));
  assert.equal(stored.messages["nonhuman-store"].role, "context");
  assert.equal(stored.parts["nonhuman-store:text"].synthetic, true);
  assert.equal(stored.prompt_revisions["nonhuman-store"], undefined);
  assert.equal(wire.join("\n").includes(canary), false, "CLI stdin never receives secret target bytes");
  assert.equal(wire.join("\n").includes(sha(canary)), false, "CLI stdin never receives private file hashes");
  const storeFiles = (directory: string): string => readdirSync(directory, { withFileTypes: true }).map(e => e.isDirectory() ? storeFiles(path.join(directory, e.name)) : readFileSync(path.join(directory, e.name), "utf8")).join("\n");
  assert.equal(storeFiles(state).includes(canary), false, "no private Store file receives secret target bytes");
  assert.equal(storeFiles(state).includes(sha(canary)), false, "no private Store file receives private file hashes");
});
