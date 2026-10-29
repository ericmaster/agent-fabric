import { test } from "node:test";
import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, readFileSync, writeFileSync, chmodSync, existsSync, rmSync, symlinkSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, sep } from "node:path";
import { pathToFileURL } from "node:url";
import { createRequire } from "node:module";

const repo = join(import.meta.dirname, "../..");
const declaredPi = "/home/ericmaster/.npm-global/lib/node_modules/@earendil-works/pi-coding-agent";
const declaredGo = "/home/ericmaster/.local/state/agent-fabric/tasks/minimal-guard-phase1/toolchains/go1.22.12/bin/go";
const host = process.env.PI_PACKAGE || (existsSync(declaredPi) ? declaredPi : join(execFileSync("npm", ["root", "-g"], { encoding: "utf8" }).trim(), "@earendil-works/pi-coding-agent"));
const piPkg = JSON.parse(readFileSync(join(host, "package.json"), "utf8"));
const { discoverAndLoadExtensions, ExtensionRunner } = await import(pathToFileURL(join(host, "dist/core/extensions/index.js")).href);
const { createBashTool, createReadTool, SettingsManager } = await import(pathToFileURL(join(host, "dist/index.js")).href);
const { runToolCall } = await import(pathToFileURL(join(host, "node_modules/@earendil-works/pi-agent-core/dist/agent-loop.js")).href);
const { resolveReadPathAsync } = await import(pathToFileURL(join(host, "dist/core/tools/path-utils.js")).href);
const require = createRequire(join(host, "package.json"));
const { createJiti } = require("jiti");
const jiti = createJiti(import.meta.url, { alias: { "@earendil-works/pi-coding-agent": join(host, "dist/index.js") } });
const { UNSUPPORTED, BLOCK_REASON, STOCK_TOOLS } = await jiti.import("./secret-guard.ts");

const helper = buildHelper();
const sourceDigest = execFileSync("sha256sum", [join(repo, "extensions/pi/secret-guard.ts")], { encoding: "utf8" }).split(" ", 1)[0];

function buildHelper() {
  if (process.env.AGF_GUARD_BIN && process.env.AGF_GUARD_BIN.startsWith("/")) return process.env.AGF_GUARD_BIN;
  const go = process.env.AGF_GO || (existsSync(declaredGo) ? declaredGo : "go");
  const dir = mkdtempSync(join(tmpdir(), "agf-guard-bin-"));
  const out = join(dir, "agf");
  execFileSync(go, ["build", "-o", out, "./cmd/agent-fabric"], { cwd: repo, stdio: "pipe" });
  chmodSync(out, 0o755);
  return out;
}

function renderEntry(agentDir, helperPath) {
  const source = readFileSync(join(repo, "extensions/pi/secret-guard.ts"), "utf8");
  const rendered = source.replace('"__AGF_GUARD_HELPER__"', JSON.stringify(helperPath));
  const dest = join(agentDir, "extensions", "agf-secret-guard");
  mkdirSync(dest, { recursive: true, mode: 0o755 });
  writeFileSync(join(dest, "index.ts"), rendered, { mode: 0o644 });
}

function writeWrapper(dir) {
  const record = join(dir, "requests.jsonl");
  const wrapper = join(dir, "agf-wrapper");
  writeFileSync(record, "", { mode: 0o600 });
  writeFileSync(wrapper, `#!/usr/bin/env node
import { spawn } from "node:child_process";
import { appendFileSync } from "node:fs";
const chunks = [];
for await (const chunk of process.stdin) chunks.push(chunk);
const input = Buffer.concat(chunks);
appendFileSync(process.env.AGF_GUARD_RECORD, input.toString("utf8") + "\\n", { mode: 0o600 });
const child = spawn(process.env.AGF_GUARD_REAL, process.argv.slice(2), { stdio: ["pipe", "pipe", "pipe"] });
child.stdout.pipe(process.stdout);
child.stderr.pipe(process.stderr);
child.stdin.end(input);
child.on("exit", (code, signal) => process.exit(code ?? (signal ? 1 : 0)));
`, { mode: 0o755 });
  return { wrapper, record };
}

function lastRequest(record) {
  const lines = readFileSync(record, "utf8").trim().split("\n").filter(Boolean);
  assert.ok(lines.length > 0, "helper was not called");
  return JSON.parse(lines.at(-1));
}

async function session(prefix) {
  const root = mkdtempSync(join(tmpdir(), "agf-pi-guard-"));
  const agentDir = join(root, "agent");
  const cwd = join(root, "cwd");
  const state = join(root, "state");
  mkdirSync(agentDir, { recursive: true, mode: 0o755 });
  mkdirSync(cwd, { recursive: true, mode: 0o755 });
  mkdirSync(state, { recursive: true, mode: 0o755 });
  if (prefix !== undefined) {
    writeFileSync(join(agentDir, "settings.json"), JSON.stringify({ shellCommandPrefix: prefix }) + "\n", { mode: 0o644 });
  }
  const { wrapper, record } = writeWrapper(root);
  renderEntry(agentDir, wrapper);
  const previous = {
    PI_CODING_AGENT_DIR: process.env.PI_CODING_AGENT_DIR,
    AGF_GUARD_REAL: process.env.AGF_GUARD_REAL,
    AGF_GUARD_RECORD: process.env.AGF_GUARD_RECORD,
    XDG_STATE_HOME: process.env.XDG_STATE_HOME,
    HOME: process.env.HOME,
  };
  process.env.PI_CODING_AGENT_DIR = agentDir;
  process.env.AGF_GUARD_REAL = helper;
  process.env.AGF_GUARD_RECORD = record;
  process.env.XDG_STATE_HOME = state;
  process.env.HOME = root;
  const loaded = await discoverAndLoadExtensions([], cwd, agentDir);
  assert.equal(loaded.errors.length, 0, JSON.stringify(loaded.errors));
  const guard = loaded.extensions.find(ext => ext.path.endsWith("/extensions/agf-secret-guard/index.ts"));
  assert.ok(guard, "native discovery missed the guard entry");
  const runner = new ExtensionRunner(loaded.extensions, loaded.runtime, cwd, { getBranch: () => [] }, {});
  return {
    root, agentDir, cwd, state, record, loaded, runner,
    async close() {
      for (const [key, value] of Object.entries(previous)) {
        if (value === undefined) delete process.env[key];
        else process.env[key] = value;
      }
      rmSync(root, { recursive: true, force: true });
    },
  };
}

function stockPrefix(cwd, agentDir) {
  return SettingsManager.create(cwd, agentDir).getShellCommandPrefix() || "";
}

async function dispatch(ctx, name, args, options = {}) {
  const prefix = stockPrefix(ctx.cwd, ctx.agentDir);
  const spawns = [];
  const reads = [];
  const bash = createBashTool(ctx.cwd, {
    commandPrefix: prefix,
    spawnHook: options.spawnHook,
    operations: { exec: async (command) => { spawns.push(command); return { exitCode: 0 }; } },
  });
  const read = createReadTool(ctx.cwd, {
    operations: {
      readFile: async (path) => { reads.push(path); return Buffer.from("ok"); },
      access: async () => {},
    },
  });
  const tool = name === "read" ? read : name === "bash" ? bash : options.tool;
  const outcome = await runToolCall({ type: "toolCall", id: "call-1", name, arguments: args }, {
    assistantMessage: { role: "assistant", content: [] },
    context: { cwd: ctx.cwd },
    tools: [tool],
    beforeToolCall: async ({ toolCall, args: validated }) => ctx.runner.emitToolCall({
      type: "tool_call",
      toolName: toolCall.name,
      toolCallId: toolCall.id,
      input: validated,
    }),
  });
  return { outcome, spawns, reads };
}

test("native discovery and dispatcher block stock bash and read before execute", async () => {
  assert.equal(piPkg.version, "1.4.0");
  assert.match(sourceDigest, /^[a-f0-9]{64}$/);
  assert.equal(typeof discoverAndLoadExtensions, "function");
  assert.equal(typeof runToolCall, "function");
  const ctx = await session();
  try {
    const denied = await dispatch(ctx, "bash", { command: "env" });
    assert.equal(denied.spawns.length, 0);
    assert.equal(denied.outcome.isError, true);
    assert.match(denied.outcome.result.content[0].text, new RegExp(BLOCK_REASON));
    const request = lastRequest(ctx.record);
    assert.equal(request.op, "shell");
    assert.equal(request.command, "env");
    assert.equal(request.shell, "bash");
    assert.equal(request.source, "pi");
    const file = join(ctx.cwd, "notes.txt");
    writeFileSync(file, "ok\n", { mode: 0o644 });
    const allowedRead = await dispatch(ctx, "read", { path: file });
    assert.equal(allowedRead.reads.length, 1);
    assert.equal(allowedRead.reads[0], file);
    assert.equal(lastRequest(ctx.record).paths[0], file);
    const proc = await dispatch(ctx, "read", { path: "/proc/self/environ" });
    assert.equal(proc.reads.length, 0);
    assert.equal(proc.outcome.isError, true);
  } finally {
    await ctx.close();
  }
});

test("safe stock bash proceeds and helper request equals synthetic spawn", async () => {
  const ctx = await session();
  try {
    const allowed = await dispatch(ctx, "bash", { command: "printf ok" });
    assert.equal(allowed.spawns.length, 1);
    assert.equal(allowed.outcome.isError, false);
    const request = lastRequest(ctx.record);
    assert.equal(request.command, "printf ok");
    assert.equal(allowed.spawns[0], request.command);
    const log = join(ctx.state, "agent-fabric", "guard", "denials.jsonl");
    assert.equal(existsSync(log), false);
  } finally {
    await ctx.close();
  }
});

test("effective stock prefix and alias-enabling prefix are checked with the spawn", async () => {
  const ctx = await session("shopt -s expand_aliases");
  try {
    const allowed = await dispatch(ctx, "bash", { command: "printf ok" });
    const request = lastRequest(ctx.record);
    assert.equal(request.command, "shopt -s expand_aliases\nprintf ok");
    assert.equal(allowed.spawns.length, 1);
    assert.equal(allowed.spawns[0], request.command);
  } finally {
    await ctx.close();
  }
  const unsafe = await session("shopt -s expand_aliases\nenv");
  try {
    const denied = await dispatch(unsafe, "bash", { command: "printf ok" });
    assert.equal(denied.spawns.length, 0);
    assert.equal(lastRequest(unsafe.record).command, "shopt -s expand_aliases\nenv\nprintf ok");
  } finally {
    await unsafe.close();
  }
  const alias = await session("alias dump=env");
  try {
    const expanded = await dispatch(alias, "bash", { command: "dump" });
    assert.equal(lastRequest(alias.record).command, "alias dump=env\ndump");
    assert.equal(expanded.spawns[0], lastRequest(alias.record).command);
    assert.ok(UNSUPPORTED.includes("shell-alias-expansion"));
  } finally {
    await alias.close();
  }
});

test("mode off, approval-free dispatch, and a benign extension still block", async () => {
  const ctx = await session();
  try {
    const benign = join(ctx.agentDir, "extensions", "aaa-benign");
    mkdirSync(benign, { recursive: true, mode: 0o755 });
    writeFileSync(join(benign, "index.ts"), "export default function (pi) { pi.on('tool_call', () => undefined); }\n", { mode: 0o644 });
    const modes = join(ctx.agentDir, "extensions", "fabric-modes");
    mkdirSync(modes, { recursive: true, mode: 0o755 });
    writeFileSync(join(modes, "index.ts"), readFileSync(join(repo, "extensions/pi/index.ts")), { mode: 0o644 });
    const loaded = await discoverAndLoadExtensions([], ctx.cwd, ctx.agentDir);
    assert.equal(loaded.errors.length, 0, JSON.stringify(loaded.errors));
    const paths = loaded.extensions.map(ext => ext.path);
    assert.ok(paths.some(path => path.endsWith("/agf-secret-guard/index.ts")));
    assert.ok(paths.some(path => path.includes("aaa-benign")));
    assert.ok(paths.some(path => path.includes("fabric-modes")));
    const runner = new ExtensionRunner(loaded.extensions, loaded.runtime, ctx.cwd, { getBranch: () => [] }, {});
    const active = ["read", "bash", "edit", "write"];
    runner.bindCore({
      sendMessage() {}, sendUserMessage() {}, appendEntry() {}, setSessionName() {}, getSessionName() {},
      setLabel() {}, getActiveTools: () => active, getAllTools: () => active.map(name => ({ name })),
      getSettings: () => ({}), setActiveTools(tools) { active.splice(0, active.length, ...tools); },
      refreshTools() {}, getCommands: () => [], setModel: async () => false, getThinkingLevel: () => "off",
      setThinkingLevel() {},
    }, {
      getModel: () => undefined, getScopedModels: () => [], isIdle: () => true, isProjectTrusted: () => true,
      getSignal: () => undefined, abort() {}, hasPendingMessages: () => false, shutdown() {},
      getContextUsage: () => undefined, compact() {}, getSystemPrompt: () => "", executeTool: async () => {
        throw new Error("nested execute is not this test");
      },
    });
    runner.setFlagValue("agf-mode", "off");
    await runner.emit({ type: "session_start", reason: "startup" });
    ctx.runner = runner;
    const denied = await dispatch(ctx, "bash", { command: "env" });
    assert.equal(denied.spawns.length, 0);
    assert.equal(denied.outcome.isError, true);
  } finally {
    await ctx.close();
  }
});

test("helper missing, error, and timeout return a fixed block and do not execute", async () => {
  const ctx = await session();
  try {
    renderEntry(ctx.agentDir, "/tmp/opencode/agf-phase2-missing-helper");
    const missing = await discoverAndLoadExtensions([], ctx.cwd, ctx.agentDir);
    ctx.runner = new ExtensionRunner(missing.extensions, missing.runtime, ctx.cwd, { getBranch: () => [] }, {});
    const missed = await dispatch(ctx, "bash", { command: "printf ok" });
    assert.equal(missed.spawns.length, 0);
    assert.match(missed.outcome.result.content[0].text, new RegExp(BLOCK_REASON));

    const bad = join(ctx.root, "bad-helper");
    writeFileSync(bad, "#!/bin/sh\nprintf '%s\\n' SENTINEL_PHASE2_SHOULD_NOT_LEAK\nexit 2\n", { mode: 0o755 });
    renderEntry(ctx.agentDir, bad);
    const errored = await discoverAndLoadExtensions([], ctx.cwd, ctx.agentDir);
    ctx.runner = new ExtensionRunner(errored.extensions, errored.runtime, ctx.cwd, { getBranch: () => [] }, {});
    const broken = await dispatch(ctx, "bash", { command: "printf ok" });
    assert.equal(broken.spawns.length, 0);
    assert.equal(broken.outcome.result.content[0].text.includes("SENTINEL_PHASE2_SHOULD_NOT_LEAK"), false);

    const slow = join(ctx.root, "slow-helper");
    writeFileSync(slow, "#!/bin/sh\nsleep 30\n", { mode: 0o755 });
    renderEntry(ctx.agentDir, slow);
    const timed = await discoverAndLoadExtensions([], ctx.cwd, ctx.agentDir);
    ctx.runner = new ExtensionRunner(timed.extensions, timed.runtime, ctx.cwd, { getBranch: () => [] }, {});
    const started = Date.now();
    const timeout = await dispatch(ctx, "bash", { command: "printf ok" });
    assert.equal(timeout.spawns.length, 0);
    assert.ok(Date.now() - started < 5000, "timeout did not return");
    assert.match(timeout.outcome.result.content[0].text, new RegExp(BLOCK_REASON));
  } finally {
    await ctx.close();
  }
});

test("separate child loads the guard offline without parent handlers", async () => {
  const ctx = await session();
  try {
    const child = join(ctx.root, "child.mjs");
    writeFileSync(child, `import { pathToFileURL } from "node:url";
const host = process.env.PI_PACKAGE;
const { discoverAndLoadExtensions, ExtensionRunner } = await import(pathToFileURL(host + "/dist/core/extensions/index.js").href);
const { createBashTool } = await import(pathToFileURL(host + "/dist/index.js").href);
const { runToolCall } = await import(pathToFileURL(host + "/node_modules/@earendil-works/pi-agent-core/dist/agent-loop.js").href);
const agentDir = process.env.PI_CODING_AGENT_DIR;
const cwd = process.env.CWD;
const loaded = await discoverAndLoadExtensions([], cwd, agentDir);
const runner = new ExtensionRunner(loaded.extensions, loaded.runtime, cwd, { getBranch: () => [] }, {});
const spawns = [];
const bash = createBashTool(cwd, { operations: { exec: async (command) => { spawns.push(command); return { exitCode: 0 }; } } });
const outcome = await runToolCall({ type: "toolCall", id: "child", name: "bash", arguments: { command: "env" } }, {
  assistantMessage: { role: "assistant", content: [] }, context: {}, tools: [bash],
  beforeToolCall: async ({ toolCall, args }) => runner.emitToolCall({ type: "tool_call", toolName: toolCall.name, toolCallId: toolCall.id, input: args }),
});
process.stdout.write(JSON.stringify({ errors: loaded.errors.length, paths: loaded.extensions.map(ext => ext.path), spawns: spawns.length, blocked: outcome.isError }));
`, { mode: 0o644 });
    const result = spawnSync(process.execPath, [child], {
      env: {
        PATH: process.env.PATH,
        HOME: ctx.root,
        PI_PACKAGE: host,
        PI_CODING_AGENT_DIR: ctx.agentDir,
        CWD: ctx.cwd,
        AGF_GUARD_REAL: helper,
        AGF_GUARD_RECORD: ctx.record,
        XDG_STATE_HOME: ctx.state,
      },
      encoding: "utf8",
    });
    assert.equal(result.status, 0, result.stderr);
    const report = JSON.parse(result.stdout);
    assert.equal(report.errors, 0);
    assert.ok(report.paths.some(path => path.endsWith("/agf-secret-guard/index.ts")));
    assert.equal(report.spawns, 0);
    assert.equal(report.blocked, true);
    assert.equal(Object.keys(result).includes("env"), false);
  } finally {
    await ctx.close();
  }
});

test("custom spawnHook, replaced tools, user bash, and pi.exec are unsupported", async () => {
  const ctx = await session();
  try {
    const hooked = await dispatch(ctx, "bash", { command: "printf ok" }, {
      spawnHook: (spawn) => ({ ...spawn, command: "env" }),
    });
    assert.equal(lastRequest(ctx.record).command, "printf ok");
    assert.equal(hooked.spawns[0], "env");
    assert.notEqual(hooked.spawns[0], lastRequest(ctx.record).command);
    const replaced = {
      name: "shell",
      label: "shell",
      parameters: createBashTool(ctx.cwd, { operations: { exec: async () => ({ exitCode: 0 }) } }).parameters,
      execute: async () => {
        replaced.ran = true;
        return { content: [{ type: "text", text: "ran" }], details: {} };
      },
    };
    const custom = await dispatch(ctx, "shell", { command: "env" }, { tool: replaced });
    assert.equal(replaced.ran, true);
    assert.equal(custom.outcome.isError, false);
    const userBash = await ctx.runner.emitUserBash({ type: "user_bash", command: "env", excludeFromContext: false, cwd: ctx.cwd });
    assert.equal(userBash, undefined);
    for (const route of ["spawnHook", "replaced-tools", "pi.exec", "user-bash", "unobservable-prefix"]) {
      assert.ok(UNSUPPORTED.includes(route), route);
    }
    assert.deepEqual(STOCK_TOOLS, { bash: "shell", read: "read" });
  } finally {
    await ctx.close();
  }
});

function requestCount(record) {
  return readFileSync(record, "utf8").trim().split("\n").filter(Boolean).length;
}

test("native-normalized read target matches helper and synthetic read", async () => {
  const ctx = await session();
  try {
    const native = (input) => resolveReadPathAsync(input, ctx.cwd);
    for (const input of ["@/proc/self/environ", "file:///proc/self/environ"]) {
      const resolved = await native(input);
      assert.equal(resolved, "/proc/self/environ");
      assert.notEqual(resolved, input);
      const denied = await dispatch(ctx, "read", { path: input });
      assert.equal(denied.reads.length, 0);
      assert.equal(denied.outcome.isError, true);
      assert.match(denied.outcome.result.content[0].text, new RegExp(BLOCK_REASON));
      assert.equal(lastRequest(ctx.record).paths[0], resolved);
      const log = join(ctx.state, "agent-fabric", "guard", "denials.jsonl");
      const text = readFileSync(log, "utf8");
      assert.equal(text.includes(input), false);
      assert.equal(text.includes("/proc/"), false);
      const row = JSON.parse(text.trim().split("\n").at(-1));
      assert.equal(row.rule, "proc_environ_read");
      assert.equal(row.source, "pi");
      assert.deepEqual(Object.keys(row).sort(), ["event_id", "layer", "policy_version", "rule", "source", "timestamp"]);
    }

    const tilde = "~/tilde-safe.txt";
    const tildeResolved = await native(tilde);
    assert.notEqual(tildeResolved, tilde);
    assert.ok(tildeResolved.endsWith(`${sep}tilde-safe.txt`), tildeResolved);
    if (tildeResolved.startsWith(ctx.root + sep)) writeFileSync(tildeResolved, "ok\n", { mode: 0o644 });
    const tildeRead = await dispatch(ctx, "read", { path: tilde });
    assert.equal(tildeRead.reads.length, 1);
    assert.equal(tildeRead.reads[0], tildeResolved);
    assert.equal(lastRequest(ctx.record).paths[0], tildeRead.reads[0]);
    assert.equal(tildeRead.outcome.isError, false);

    const spaced = join(ctx.cwd, "notes .txt");
    writeFileSync(spaced, "ok\n", { mode: 0o644 });
    const unicode = "notes\u00A0.txt";
    const unicodeResolved = await native(unicode);
    assert.equal(unicodeResolved, spaced);
    const unicodeRead = await dispatch(ctx, "read", { path: unicode });
    assert.equal(unicodeRead.reads.length, 1);
    assert.equal(unicodeRead.reads[0], unicodeResolved);
    assert.equal(lastRequest(ctx.record).paths[0], unicodeRead.reads[0]);

    const linked = join(ctx.cwd, "linked.txt");
    const link = join(ctx.cwd, "safe-link");
    writeFileSync(linked, "ok\n", { mode: 0o644 });
    symlinkSync(linked, link);
    const linkResolved = await native("safe-link");
    assert.equal(linkResolved, link);
    const linkRead = await dispatch(ctx, "read", { path: "safe-link" });
    assert.equal(linkRead.reads.length, 1);
    assert.equal(linkRead.reads[0], linkResolved);
    assert.equal(lastRequest(ctx.record).paths[0], linkRead.reads[0]);

    const procLink = join(ctx.cwd, "proc-link");
    symlinkSync("/proc/self/environ", procLink);
    const procLinkResolved = await native("proc-link");
    assert.equal(procLinkResolved, procLink);
    const procLinked = await dispatch(ctx, "read", { path: "proc-link" });
    assert.equal(procLinked.reads.length, 0);
    assert.match(procLinked.outcome.result.content[0].text, new RegExp(BLOCK_REASON));
    assert.equal(lastRequest(ctx.record).paths[0], procLinkResolved);

    const before = requestCount(ctx.record);
    const unresolvable = await dispatch(ctx, "read", { path: "file:///%" });
    assert.equal(unresolvable.reads.length, 0);
    assert.match(unresolvable.outcome.result.content[0].text, new RegExp(BLOCK_REASON));
    assert.equal(requestCount(ctx.record), before);
    await assert.rejects(() => native("file:///%"));
  } finally {
    await ctx.close();
  }
});

test("denial audit is metadata only", async () => {
  const ctx = await session();
  try {
    const canary = "PHASE2CANARY";
    const denied = await dispatch(ctx, "bash", { command: `printenv ${canary}` });
    assert.equal(denied.spawns.length, 0);
    const log = join(ctx.state, "agent-fabric", "guard", "denials.jsonl");
    const text = readFileSync(log, "utf8");
    assert.equal(text.includes(canary), false);
    assert.equal(text.includes("printenv"), false);
    const row = JSON.parse(text.trim().split("\n").at(-1));
    assert.deepEqual(Object.keys(row).sort(), ["event_id", "layer", "policy_version", "rule", "source", "timestamp"]);
    assert.equal(row.source, "pi");
    assert.equal(row.rule, "environment_dump");
    assert.equal(row.layer, "deterministic");
  } finally {
    await ctx.close();
  }
});

