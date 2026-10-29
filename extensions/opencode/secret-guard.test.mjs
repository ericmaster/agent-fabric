import { test } from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { chmodSync, existsSync, mkdirSync, mkdtempSync, readFileSync, renameSync, symlinkSync, writeFileSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join } from "node:path";

const repo = join(import.meta.dirname, "../..");
const SENTINEL = "AGF_NOECHO_SENTINEL_9f3c";

function locator(name) {
  const value = process.env[name];
  if (!value || !value.startsWith("/") || !existsSync(value)) {
    throw new Error(`absent locator ${name}`);
  }
  return value;
}

const evidence = locator("AGF_EVIDENCE");
const fixture = locator("AGF_OC_FIXTURE");
const bun = locator("AGF_BUN");
const goBin = locator("AGF_GO");
const piPackage = locator("AGF_PI_PACKAGE");
const core = join(fixture, "packages/core");
const effect = join(core, "node_modules/effect");
const source = join(repo, "extensions/opencode/secret-guard.ts");
const corpusPath = join(repo, "fixtures/guard/corpus.json");

function freshRoot(label) {
  const base = join(evidence, "stable-location", "attempt9", "roots");
  mkdirSync(base, { recursive: true, mode: 0o700 });
  const root = mkdtempSync(join(base, `${label}-`));
  for (const part of ["home", "config", "data", "state", "cache", "tmp", "bin", "runtime", "system-config", "system-data"]) {
    mkdirSync(join(root, part), { recursive: true, mode: 0o700 });
  }
  return root;
}

function allowEnv(root, extra = {}) {
  for (const key of Object.keys(extra)) {
    if (key.startsWith("OPENCODE_") && key !== "OPENCODE_TEST_HOME") throw new Error(`refusing selector ${key}`);
  }
  return {
    PATH: [dirname(bun), dirname(goBin), "/usr/bin", "/bin"].join(":"),
    HOME: join(root, "home"),
    XDG_CONFIG_HOME: join(root, "config"),
    XDG_DATA_HOME: join(root, "data"),
    XDG_STATE_HOME: join(root, "state"),
    XDG_CACHE_HOME: join(root, "cache"),
    TMPDIR: join(root, "tmp"),
    TMP: join(root, "tmp"),
    TEMP: join(root, "tmp"),
    XDG_RUNTIME_DIR: join(root, "runtime"),
    XDG_CONFIG_DIRS: join(root, "system-config"),
    XDG_DATA_DIRS: join(root, "system-data"),
    OPENCODE_TEST_HOME: join(root, "home"),
    AGF_PRIVATE_ROOT: root,
    AGF_CONFIG: join(root, "absent-agf.json"),
    AGF_CORE: core,
    AGF_EFFECT: effect,
    AGF_SENTINEL: SENTINEL,
    ...extra,
  };
}

function runBun(root, script, extra = {}) {
  return spawnSync(bun, [script], { env: allowEnv(root, extra), encoding: "utf8", timeout: 120000 });
}

function leaked(blobs) {
  return blobs.some((blob) => String(blob ?? "").includes(SENTINEL));
}

function contained(child, parent) {
  if (!child || !parent) return false;
  const left = String(child).split("/").filter(Boolean);
  const right = String(parent).split("/").filter(Boolean);
  if (left.length < right.length) return false;
  return right.every((part, index) => left[index] === part);
}

function assertCanary(roots, root) {
  const required = ["config", "data", "state", "cache", "tmp", "home"];
  assert.equal(Boolean(roots) && typeof roots === "object" && !Array.isArray(roots), true);
  for (const key of required) {
    assert.equal(typeof roots[key] === "string" && roots[key].length > 0, true, key);
    assert.equal(contained(roots[key], root), true, key);
  }
  assert.equal(contained("", root), false);
  assert.equal(contained(root + "-sibling", root), false);
  assert.equal(contained(join(dirname(root), "sibling"), root), false);
}

function assertCanaryControls() {
  const root = "/private/canary-root";
  const good = {
    config: join(root, "config"),
    data: join(root, "data"),
    state: join(root, "state"),
    cache: join(root, "cache"),
    tmp: join(root, "tmp"),
    home: join(root, "home"),
  };
  assertCanary(good, root);
  const bad = [
    undefined,
    null,
    {},
    { ...good, config: 1 },
    { ...good, home: "" },
    { ...good, tmp: root + "-sibling/tmp" },
    { ...good, cache: join(dirname(root), "sibling", "cache") },
  ];
  for (const roots of bad) assert.throws(() => assertCanary(roots, root));
}

function immediateDelta(before, after) {
  return after - before;
}

function assertCounterOracles() {
  const opened = [];
  const denyBefore = opened.length;
  assert.equal(immediateDelta(denyBefore, opened.length), 0);
  const allowBefore = opened.length;
  opened.push("safe");
  assert.equal(immediateDelta(allowBefore, opened.length), 1);
  assert.equal(immediateDelta(denyBefore, opened.length) === 0, false);
  const fault = { message: "guard_withheld", delta: 0, allowRequired: false };
  assert.equal(fault.message, "guard_withheld");
  assert.equal(fault.delta, 0);
  assert.equal(fault.allowRequired, false);
  let cursor = 4;
  const denyAt = cursor;
  const denyDelta = immediateDelta(denyAt, cursor);
  const allowAt = cursor;
  cursor += 1;
  const allowDelta = immediateDelta(allowAt, cursor);
  const lateAt = cursor;
  cursor += 1;
  const lateDelta = immediateDelta(lateAt, cursor);
  const tildeAt = cursor;
  cursor += 1;
  const tildeDelta = immediateDelta(tildeAt, cursor);
  const frozen = [denyDelta, allowDelta, lateDelta, tildeDelta];
  cursor += 5;
  assert.deepEqual(frozen, [0, 1, 1, 1]);
  assert.equal(immediateDelta(denyAt, cursor) === denyDelta, false);
  assert.equal(immediateDelta(4, 5) === 0, false);
  assert.equal(immediateDelta(4, 4) === 1, false);
}

function activeInstalled(list, ids) {
  return ids.every((id) => Array.isArray(list) && list.some((item) => item && item.id === id && item.state && item.state.status === "active"));
}

function assertHealthControls() {
  const ids = ["aaa-rename", "agf-secret-guard", "benign", "zzz-late"];
  const desired = ids.slice();
  const ping = true;
  const failedBenign = [
    { id: "aaa-rename", state: { status: "active" } },
    { id: "agf-secret-guard", state: { status: "active" } },
    { id: "benign", state: { status: "failed" } },
    { id: "zzz-late", state: { status: "active" } },
  ];
  assert.equal(desired.includes("benign") && ping, true);
  assert.equal(activeInstalled(failedBenign, ids), false);
  assert.equal(activeInstalled(failedBenign.filter((item) => item.id !== "benign"), ids), false);
  assert.equal(activeInstalled(ids.map((id) => ({ id, state: { status: "active" } })), ids), true);
}

function constructorCoherent(observation) {
  if (!observation || typeof observation.hostCwd !== "string" || observation.hostCwd.length === 0) return false;
  if (observation.hostCwd !== observation.locationCwd) return false;
  if (observation.rowCwd !== undefined && observation.hostCwd !== observation.rowCwd) return false;
  if (typeof observation.acquiredTarget !== "string" || observation.acquiredTarget !== observation.expectedTarget) return false;
  return true;
}

const witnessSource = 'export default { id: "agf-location-witness", setup(ctx) { globalThis.__AGF_HOST_CWD = ctx.location && ctx.location.directory; } };';
const factoryFilter = "test/plugin/fixture\\.ts$";

function syntaxOk(file) {
  const out = join("/tmp/opencode", "attempt9-syntax-" + createHash("sha256").update(file).digest("hex").slice(0, 12) + ".js");
  mkdirSync("/tmp/opencode", { recursive: true, mode: 0o700 });
  const ran = spawnSync(bun, ["build", file, "--outfile=" + out], { encoding: "utf8" });
  return ran.status === 0;
}

function assertEmittedSyntax() {
  const archived = join(evidence, "stable-location/attempt8/roots/main-UKicOI/read.ts");
  const archivedBytes = readFileSync(archived);
  assert.equal(createHash("sha256").update(archivedBytes).digest("hex"), "c06e78e252a665ab9daea71ebe80c9d8d4e081b81f2e1716a537ec0049c5c052");
  assert.equal(syntaxOk(archived), false);
  const emitted = join(evidence, "stable-location/attempt9/emitted-read.ts");
  mkdirSync(dirname(emitted), { recursive: true, mode: 0o700 });
  writeFileSync(emitted, readHarness, { mode: 0o644 });
  assert.equal(syntaxOk(emitted), true);
  assert.equal(readHarness.includes(JSON.stringify(witnessSource)), true);
  assert.equal(JSON.parse(JSON.stringify(witnessSource)), witnessSource);
  assert.equal(witnessSource.includes('"'), true);
  assert.equal(readHarness.includes(JSON.stringify(factoryFilter)), true);
  assert.equal(JSON.parse(JSON.stringify(factoryFilter)), factoryFilter);
  assert.equal(new RegExp(factoryFilter).test("/x/test/plugin/fixture.ts"), true);
  assert.equal(new RegExp(factoryFilter).test("/x/test/plugin/fixtureXts"), false);
  assert.equal(readHarness.includes("__WITNESS_LITERAL__"), false);
  assert.equal(readHarness.includes("__FILTER_LITERAL__"), false);
  assert.equal(readHarness.includes("__CONSTRUCTOR_PREDICATE__"), false);
}

function assertLifetimeControl() {
  const expected = "/proc/self/environ";
  const coherent = { hostCwd: "/proc/self", locationCwd: "/proc/self", rowCwd: "/proc/self", acquiredTarget: expected, expectedTarget: expected };
  assert.equal(constructorCoherent(coherent), true);
  assert.equal(constructorCoherent({ ...coherent, hostCwd: "/tmp/factory", locationCwd: "/proc/self" }), false);
  assert.equal(constructorCoherent({ ...coherent, acquiredTarget: "/tmp/factory/environ" }), false);
  assert.equal(constructorCoherent({ ...coherent, hostCwd: "" }), false);
  assert.equal(readHarness.includes(constructorCoherent.name), true);
  assert.equal(readHarness.includes("yield* FileAccess.Service"), true);
}

function assertRefusalControls() {
  const probe = readFileSync(join(evidence, "diagnostic-native/second/probe.ts"), "utf8");
  const targets = [
    "import net from \"node:net\"",
    "import tls from \"node:tls\"",
    "import http from \"node:http\"",
    "import https from \"node:https\"",
    "import dgram from \"node:dgram\"",
    "net.connect = refuse(\"net.connect\")",
    "net.createConnection = refuse(\"net.createConnection\")",
    "net.Socket.prototype.connect = refuse(\"Socket.connect\")",
    "tls.connect = refuse(\"tls.connect\")",
    "http.request = refuse(\"http.request\")",
    "http.get = refuse(\"http.get\")",
    "https.request = refuse(\"https.request\")",
    "https.get = refuse(\"https.get\")",
    "dgram.createSocket = refuse(\"dgram.createSocket\")",
    "Bun.connect = refuse(\"Bun.connect\")",
    "Bun.udpSocket = refuse(\"Bun.udpSocket\")",
  ];
  for (const line of targets) {
    assert.equal(probe.includes(line), true, line);
    assert.equal(readHarness.includes(line), true, line);
  }
  assert.equal(readHarness.includes("await import(\"node:net\")"), false);
  assert.equal(readHarness.includes("Promise.reject"), false);
  const attempts = [];
  const refuse = (kind) => (..._args) => {
    attempts.push(kind);
    throw new Error("diagnostic_network_refused");
  };
  const installed = ["fetch", "net.connect", "Socket.connect", "Bun.connect", "Bun.udpSocket"].map(refuse);
  for (const callback of installed) assert.throws(() => callback("synthetic"), /diagnostic_network_refused/);
  assert.deepEqual(attempts, ["fetch", "net.connect", "Socket.connect", "Bun.connect", "Bun.udpSocket"]);
}

function lastJson(text) {
  const line = text.trim().split("\n").filter(Boolean).at(-1);
  assert.ok(line, "native harness produced no JSON");
  return JSON.parse(line);
}

function assertPrivate(paths, root) {
  for (const [key, value] of Object.entries(paths)) {
    assert.equal(typeof value, "string", key);
    assert.equal(value.startsWith(root), true, `${key} escaped private root`);
  }
}

function buildHelper(root) {
  const out = join(root, "bin", "agf");
  const built = spawnSync(goBin, ["build", "-o", out, "./cmd/agent-fabric"], {
    cwd: repo, env: allowEnv(root), encoding: "utf8",
  });
  assert.equal(built.status, 0, built.stderr || built.stdout);
  chmodSync(out, 0o755);
  return out;
}

function installGuard(root, helper) {
  const installed = spawnSync(helper, [
    "install", "--source", repo, "--tools", "opencode", "--assets", "secret-guard",
    "--agents", "planner", "--yes",
  ], { cwd: repo, env: allowEnv(root), encoding: "utf8" });
  assert.equal(installed.status, 0, installed.stderr || installed.stdout);
  const dest = join(root, "config", "opencode", "plugins", "agf-secret-guard", "index.ts");
  assert.equal(existsSync(dest), true, "installer missed native plugins destination");
  const text = readFileSync(dest, "utf8");
  assert.equal(text.includes("__AGF_GUARD_HELPER__"), false);
  assert.equal(text.includes(helper), true);
  return dest;
}

function wrapHelper(helper, record) {
  const real = `${helper}-real`;
  renameSync(helper, real);
  writeFileSync(record, "", { mode: 0o600 });
  writeFileSync(helper, `#!/usr/bin/env node
import { spawn } from "node:child_process";
import { appendFileSync } from "node:fs";
const chunks = [];
for await (const chunk of process.stdin) chunks.push(chunk);
const input = Buffer.concat(chunks);
const child = spawn(process.env.AGF_GUARD_REAL, process.argv.slice(2), { stdio: ["pipe", "pipe", "pipe"], env: process.env });
let err = "";
let out = "";
child.stderr.on("data", (chunk) => { err += chunk; });
child.stdout.on("data", (chunk) => { out += chunk; process.stdout.write(chunk); });
child.stdin.end(input);
child.on("exit", (code, signal) => {
  let request = null;
  let response = null;
  try { request = JSON.parse(input.toString("utf8")); } catch { request = null; }
  try { response = JSON.parse(out); } catch { response = null; }
  appendFileSync(process.env.AGF_GUARD_RECORD, JSON.stringify({ request, response }) + "\\n", { mode: 0o600 });
  if (err && process.env.AGF_GUARD_ERR) appendFileSync(process.env.AGF_GUARD_ERR, err, { mode: 0o600 });
  process.exit(code ?? (signal ? 1 : 0));
});
`, { mode: 0o755 });
  return real;
}

const readHarness = `
import net from "node:net";
import tls from "node:tls";
import http from "node:http";
import https from "node:https";
import dgram from "node:dgram";
const forbidden = ["OPENCODE_CONFIG_DIR", "OPENCODE_DB", "OPENCODE_CONFIG"].filter((key) => process.env[key]);
const privateRoot = process.env.AGF_PRIVATE_ROOT || "";
const rootKeys = ["HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_RUNTIME_DIR", "XDG_CONFIG_DIRS", "XDG_DATA_DIRS", "TMPDIR", "TMP", "TEMP", "OPENCODE_TEST_HOME"];
function pathContained(child, parent) {
  if (!child || !parent) return false;
  const left = String(child).split("/").filter(Boolean);
  const right = String(parent).split("/").filter(Boolean);
  return left.length >= right.length && right.every((part, index) => left[index] === part);
}
if (forbidden.length || !privateRoot || rootKeys.some((key) => !pathContained(process.env[key], privateRoot))) {
  console.log(JSON.stringify({ ok: false, stage: "selectors", present: forbidden, envAbsent: forbidden.length === 0 }));
  process.exit(0);
}
const violations = [];
const refuse = (kind) => (..._args) => { violations.push(kind); throw new Error("diagnostic_network_refused"); };
globalThis.fetch = refuse("fetch");
net.connect = refuse("net.connect");
net.createConnection = refuse("net.createConnection");
net.Socket.prototype.connect = refuse("Socket.connect");
tls.connect = refuse("tls.connect");
http.request = refuse("http.request");
http.get = refuse("http.get");
https.request = refuse("https.request");
https.get = refuse("https.get");
dgram.createSocket = refuse("dgram.createSocket");
Bun.connect = refuse("Bun.connect");
Bun.udpSocket = refuse("Bun.udpSocket");
const nodeFs = await import("node:fs");
const nodePath = await import("node:path");
let roots = null;
try {
  const core = process.env.AGF_CORE;
  const effect = process.env.AGF_EFFECT;
  const config = process.env.AGF_CONFIG_DIR;
  const { Effect, Context, Exit, Layer, Option, Cause } = await import(effect + "/dist/index.js");
  const { Path } = await import(nodePath.dirname(core) + "/util/src/global.ts");
  roots = { config: Path.config, data: Path.data, state: Path.state, cache: Path.cache, tmp: Path.tmp, home: Path.home };
  nodeFs.writeFileSync(process.env.AGF_CANARY, JSON.stringify(roots), { mode: 0o600 });
  let makePluginTestLayer;
  const { registerToolPlugin } = await import(core + "/test/lib/tool.ts");
  const { PluginModule } = await import(core + "/src/plugin/module.ts");
  const { Plugin } = await import(core + "/src/plugin.ts");
  const { Tool } = await import(core + "/src/tool.ts");
  const { ReadTool } = await import(core + "/src/tool/plugin/read.ts");
  const { ReadToolFileSystem } = await import(core + "/src/tool/read-filesystem.ts");
  const { SessionInstructions } = await import(core + "/src/session/instructions.ts");
  const { Location } = await import(core + "/src/location.ts");
  const schema = await import(core + "/src/schema.ts");
  const locFx = await import(core + "/test/fixture/location.ts");
  const { Permission } = await import(core + "/src/permission.ts");
  const { FileAccess } = await import(core + "/src/file-access.ts");
  const { FSUtil } = await import(nodePath.dirname(core) + "/util/src/fs-util.ts");
  const { LayerNode } = await import(nodePath.dirname(core) + "/util/src/effect/layer-node.ts");
  const { Bus } = await import(core + "/src/bus.ts");
  const { Database } = await import(core + "/src/database/database.ts");
  const { PluginHooks } = await import(core + "/src/plugin/hooks.ts");
  const { Agent } = await import(nodePath.dirname(core) + "/schema/src/agent.ts");
  const { Session } = await import(nodePath.dirname(core) + "/schema/src/session.ts");
  const { SessionMessage } = await import(nodePath.dirname(core) + "/schema/src/session-message.ts");
  const opened = [];
  const permission = Layer.succeed(Permission.Service, Permission.Service.of({
    close: Effect.void,
    ask: (input) => Effect.succeed({ id: input.id ?? "allow", effect: "allow" }),
    assert: () => Effect.void,
    reply: () => Effect.void,
    get: () => Effect.succeed(undefined),
    forSession: () => Effect.succeed([]),
    list: () => Effect.succeed([]),
  }));
  const messageOf = (exit) => {
    if (!Exit.isFailure(exit)) return "";
    const error = Option.getOrUndefined(Exit.findErrorOption(exit));
    return error && typeof error.message === "string" ? error.message : "";
  };
  const program = Effect.gen(function* () {
    const ctx = yield* Effect.context();
    const tools = yield* Tool.Service;
    const plugins = yield* Plugin.Service;
    const location = yield* Location.Service;
    const r05Only = process.env.AGF_R05_ONLY === "1";
    if (!r05Only) {
      nodeFs.mkdirSync(nodePath.join(location.directory, "notes"), { recursive: true });
      nodeFs.writeFileSync(nodePath.join(location.directory, "notes", "ok.txt"), "ok");
      nodeFs.writeFileSync(nodePath.join(location.directory, "notes", "late.txt"), "late");
      nodeFs.writeFileSync(nodePath.join(location.directory, "normal.txt"), "normal");
      nodeFs.writeFileSync(nodePath.join(location.directory, "environment.md"), "md");
      nodeFs.writeFileSync(nodePath.join(process.env.OPENCODE_TEST_HOME, "secret-name"), "tilde");
    }
    const reader = Layer.succeed(ReadToolFileSystem.Service, ReadToolFileSystem.Service.of({
      list: () => Effect.sync(() => []),
      read: (input) => Effect.sync(() => {
        const value = String(input);
        opened.push(value);
        return { type: "file", uri: "file://" + value, name: nodePath.basename(value), content: "synthetic-safe-content", encoding: "utf8", mime: "text/plain" };
      }),
    }));
    const instructions = LayerNode.compile(SessionInstructions.node, { replacements: [
      Bus.node.replace(Layer.succeed(Bus.Service, Context.get(ctx, Bus.Service))),
      Database.node.replace(Layer.succeed(Database.Service, Context.get(ctx, Database.Service))),
      FSUtil.node.replace(Layer.succeed(FSUtil.Service, Context.get(ctx, FSUtil.Service))),
      Location.node.replace(Layer.succeed(Location.Service, location)),
    ] });
    const modules = yield* PluginModule.make();
    const witnessDir = nodePath.join(config, "plugins", "agf-location-witness");
    nodeFs.mkdirSync(witnessDir, { recursive: true });
    nodeFs.writeFileSync(nodePath.join(witnessDir, "index.ts"), __WITNESS_LITERAL__);
    const pluginRoot = nodePath.join(config, "plugins");
    const targets = nodeFs.readdirSync(pluginRoot).map((name) => nodePath.join(pluginRoot, name)).filter((target) => nodeFs.statSync(target).isDirectory()).sort();
    const desired = [];
    for (const target of targets) {
      const loaded = yield* modules.load({ type: "add", target, options: {} }, { install: false });
      if (!loaded || !("id" in loaded)) return { ok: false, stage: "load", target, roots, envAbsent: true, location: location.directory };
      desired.push(loaded);
    }
    yield* plugins.activate(desired);
    yield* plugins.awaitActivation;
    const activeBefore = yield* plugins.list();
    const support = yield* Layer.build(Layer.mergeAll(reader, instructions));
    const register = Effect.gen(function* () {
      if ((yield* Tool.Service) !== tools) throw new Error("duplicate_registry");
      yield* registerToolPlugin(ReadTool.Plugin, { location });
    });
    yield* register.pipe(Effect.provide(support));
    const activeAfter = yield* plugins.list();
    const inventory = yield* tools.list();
    const snapshot = yield* tools.snapshot();
    const stock = snapshot.definitions.find((item) => item.name === "read");
    const order = desired.map((item) => item.id);
    const names = inventory.map((item) => item.name);
    const summarize = (list) => list.map((item) => ({ id: item.id, state: { status: item.state && item.state.status } }));
    const activeInstalled = (list, ids) => ids.every((id) => list.some((item) => item && item.id === id && item.state && item.state.status === "active"));
    const beforeList = summarize(activeBefore);
    const afterList = summarize(activeAfter);
    if (!stock || !names.includes("read") || !activeInstalled(beforeList, order) || !activeInstalled(afterList, order)) {
      return { ok: false, stage: "inventory", order, names, roots, envAbsent: true, location: location.directory, activeBefore: beforeList, activeAfter: afterList };
    }
    const home = process.env.OPENCODE_TEST_HOME;
    const constructorCoherent = __CONSTRUCTOR_PREDICATE__;
    const observe = (relative, rowCwd) => Effect.gen(function* () {
      const access = yield* FileAccess.Service;
      const resolved = yield* access.resolve({ path: relative, kind: "file" });
      return {
        hostCwd: globalThis.__AGF_HOST_CWD || "",
        locationCwd: location.directory,
        rowCwd,
        acquiredTarget: String(resolved.absolute),
        expectedTarget: FileAccess.resolvePath(rowCwd, relative, home),
      };
    });
    const call = (name, input, id, definitions) => snapshot.execute({
      sessionID: Session.ID.make("ses_oc"),
      agent: Agent.ID.make("build"),
      messageID: SessionMessage.ID.make("msg_" + id),
      call: { type: "tool-call", id, name, input },
      definitions,
    }).pipe(Effect.exit);
    const one = (row, rowCwd) => Effect.gen(function* () {
      const constructor = yield* observe(row.path, rowCwd);
      if (!constructorCoherent(constructor)) return { stage: "constructor-cwd", constructor };
      const before = opened.length;
      const exit = yield* call("read", { path: row.path }, "call_" + row.id, new Map([["read", stock]]));
      return {
        id: row.id, expected: row.decision, reason: row.reason, path: row.path, cwd: rowCwd, constructor,
        actual: messageOf(exit) === "guard_withheld" ? "deny" : Exit.isSuccess(exit) ? "allow" : "error",
        delta: opened.length - before, native: constructor.acquiredTarget, reader: opened.length > before ? opened[before] : null, message: messageOf(exit),
      };
    });
    const take = function* (row, rowCwd) {
      const built = yield* one(row, rowCwd);
      if (built.stage === "constructor-cwd") return { ok: false, stage: "constructor-cwd", constructor: built.constructor, roots, envAbsent: true, order, names };
      return built;
    };
    const base = { ok: true, roots, envAbsent: true, order, registered: [ReadTool.Plugin.id], names, location: location.directory, hostCwd: globalThis.__AGF_HOST_CWD || "", activeBefore: beforeList, activeAfter: afterList, violations };
    if (r05Only || process.env.AGF_FAULT === "1") {
      const cases = [];
      for (const row of JSON.parse(process.env.AGF_CASES)) {
        const built = yield* take(row, r05Only ? "/proc/self" : location.directory);
        if (built.ok === false) return built;
        cases.push(built);
      }
      return { ...base, stage: r05Only ? "r05" : "fault", cases, constructor: cases[0] && cases[0].constructor, procReads: opened.filter((item) => item.startsWith("/proc/")).length };
    }
    const prelude = yield* observe("notes/ok.txt", location.directory);
    if (!constructorCoherent(prelude)) return { ok: false, stage: "constructor-cwd", constructor: prelude, roots, envAbsent: true, order, names };
    const denyBefore = opened.length;
    const smokeDeny = yield* call("read", { path: "/proc/self/environ" }, "smoke_deny", new Map([["read", stock]]));
    const denyDelta = opened.length - denyBefore;
    const allowBefore = opened.length;
    const smokeAllow = yield* call("read", { path: "notes/ok.txt" }, "smoke_allow", new Map([["read", stock]]));
    const allowDelta = opened.length - allowBefore;
    const smokeAllowNative = FileAccess.resolvePath(location.directory, "notes/ok.txt", home);
    if (messageOf(smokeDeny) !== "guard_withheld" || denyDelta !== 0 || !Exit.isSuccess(smokeAllow) || allowDelta !== 1 || opened[allowBefore] !== smokeAllowNative) {
      return { ok: false, stage: "smoke", deny: messageOf(smokeDeny), denyDelta, allow: Exit.isSuccess(smokeAllow), allowDelta, reader: opened[allowBefore] ?? null, native: smokeAllowNative, roots, envAbsent: true, order, names, location: location.directory };
    }
    const cases = [];
    for (const row of JSON.parse(process.env.AGF_CASES)) {
      const built = yield* take(row, location.directory);
      if (built.ok === false) return built;
      cases.push(built);
    }
    const hooks = yield* PluginHooks.Service;
    const advertise = (event) => {
      const copy = { description: stock.description, input: stock.inputSchema };
      event.tools = { read: copy, InventedRead: { description: "fabricated", input: { type: "object", properties: {} } } };
      return copy;
    };
    const survived = (copy, tools) => {
      const out = new Map();
      for (const [name, value] of Object.entries(tools || {})) if (value === copy) out.set(name, stock);
      return out;
    };
    const early = { sessionID: Session.ID.make("ses_early"), agent: Agent.ID.make("build"), model: { providerID: "fixture", modelID: "none" }, system: [], messages: [{ role: "user", content: process.env.AGF_SENTINEL }], options: {}, tools: {} };
    const earlyCopy = advertise(early);
    yield* hooks.trigger("session", "context", early);
    const earlyMap = survived(earlyCopy, early.tools);
    const late = { sessionID: early.sessionID, agent: early.agent, model: early.model, system: [], messages: early.messages, options: { produceLate: true }, tools: {} };
    const lateCopy = advertise(late);
    yield* hooks.trigger("session", "context", late);
    const lateMap = survived(lateCopy, late.tools);
    const aliasDenyBefore = opened.length;
    const aliasDeny = yield* call("ReadFile", { path: "/proc/self/environ" }, "alias_deny", earlyMap);
    const aliasDenyDelta = opened.length - aliasDenyBefore;
    const aliasDenyReader = opened[aliasDenyBefore] ?? null;
    const aliasAllowBefore = opened.length;
    const aliasAllow = yield* call("ReadFile", { path: "notes/ok.txt" }, "alias_allow", earlyMap);
    const aliasAllowDelta = opened.length - aliasAllowBefore;
    const aliasAllowReader = opened[aliasAllowBefore] ?? null;
    const lateBefore = opened.length;
    const lateExit = yield* call("SecretRead", { path: "notes/late.txt" }, "late", lateMap);
    const lateDelta = opened.length - lateBefore;
    const lateReader = opened[lateBefore] ?? null;
    const tildeBefore = opened.length;
    const tildeNative = FileAccess.resolvePath(location.directory, "~/secret-name", home);
    const tilde = yield* call("read", { path: "~/secret-name" }, "tilde", new Map([["read", stock]]));
    const tildeDelta = opened.length - tildeBefore;
    const tildeReader = opened[tildeBefore] ?? null;
    yield* tools.transform((editor) => editor.add({
      name: "ping", description: "benign", input: { type: "object", properties: {} }, options: { codemode: false },
      execute: () => Effect.sync(() => ({ content: [{ type: "text", text: "pong" }] })),
    }));
    const pingSnap = yield* tools.snapshot();
    const pingExit = yield* pingSnap.execute({
      sessionID: Session.ID.make("ses_ping"),
      agent: Agent.ID.make("build"),
      messageID: SessionMessage.ID.make("msg_ping"),
      call: { type: "tool-call", id: "call_ping", name: "ping", input: {} },
    }).pipe(Effect.exit);
    return {
      ...base,
      stage: "main",
      constructor: prelude,
      cases,
      earlyNames: [...earlyMap.keys()],
      lateNames: [...lateMap.keys()],
      smokeAllowNative,
      aliasDenied: { message: messageOf(aliasDeny), delta: aliasDenyDelta, reader: aliasDenyReader },
      aliasAllowed: { success: Exit.isSuccess(aliasAllow), delta: aliasAllowDelta, reader: aliasAllowReader, native: FileAccess.resolvePath(location.directory, "notes/ok.txt", home) },
      lateAlias: { success: Exit.isSuccess(lateExit), message: messageOf(lateExit), delta: lateDelta, reader: lateReader },
      tilde: { success: Exit.isSuccess(tilde), delta: tildeDelta, reader: tildeReader, native: tildeNative },
      ping: Exit.isSuccess(pingExit),
      procReads: opened.filter((item) => item.startsWith("/proc/")).length,
    };
  });
  const logical = process.env.AGF_LOGICAL_LOCATION || "";
  const upstreamPath = core + "/test/plugin/fixture.ts";
  const needle = "Location.node.replace(tempLocationLayer)";
  const replacement = "Location.node.replace(globalThis.__AGF_TEST_LOCATION_LAYER ?? tempLocationLayer)";
  let factory = { adapted: false, upstreamSha256: "", adaptedSha256: "", transformation: "" };
  if (logical) {
    const { createHash } = await import("node:crypto");
    const { plugin } = await import("bun");
    const upstream = nodeFs.readFileSync(upstreamPath, "utf8");
    if (upstream.split(needle).length !== 2) throw new Error("factory-site-not-unique");
    const adapted = upstream.replace(needle, replacement);
    factory = {
      adapted: true,
      upstreamSha256: createHash("sha256").update(upstream).digest("hex"),
      adaptedSha256: createHash("sha256").update(adapted).digest("hex"),
      transformation: needle + " -> " + replacement,
    };
    plugin({
      name: "agf-stable-location",
      setup(build) {
        build.onLoad({ filter: new RegExp(__FILTER_LITERAL__) }, () => ({ contents: adapted, loader: "ts" }));
      },
    });
    globalThis.__AGF_TEST_LOCATION_LAYER = Layer.succeed(
      Location.Service,
      Location.Service.of(locFx.location(Location.Ref.make({ directory: schema.AbsolutePath.make(logical) }))),
    );
  }
  ({ makePluginTestLayer } = await import(upstreamPath));
  const exit = await Effect.runPromise(program.pipe(Effect.scoped, Effect.provide(makePluginTestLayer(permission)), Effect.timeout("40 seconds"), Effect.exit));
  const payload = Exit.isSuccess(exit) ? { ...exit.value, violations, factory } : { ok: false, stage: "effect", cause: Cause.pretty(exit.cause).slice(0, 1800), roots, envAbsent: true, violations, factory };
  nodeFs.writeFileSync(process.env.AGF_CANARY, JSON.stringify(payload.roots ?? roots ?? {}), { mode: 0o600 });
  console.log(JSON.stringify(payload));
} catch (error) {
  console.log(JSON.stringify({ ok: false, stage: "startup", cause: String(error && error.stack ? error.stack : error).slice(0, 1800), roots, envAbsent: true, violations }));
}
`.replace("__WITNESS_LITERAL__", JSON.stringify(witnessSource)).replace("__FILTER_LITERAL__", JSON.stringify(factoryFilter)).replace("__CONSTRUCTOR_PREDICATE__", constructorCoherent.toString());

const shellHarness = `
const forbidden = ["OPENCODE_CONFIG_DIR", "OPENCODE_DB", "OPENCODE_CONFIG"].filter((key) => process.env[key]);
if (forbidden.length) {
  console.log(JSON.stringify({ ok: false, stage: "selectors", present: forbidden }));
  process.exit(0);
}
const violations = [];
globalThis.fetch = () => { violations.push("fetch"); return Promise.reject(new Error("blocked-network")); };
const core = process.env.AGF_CORE;
const effect = process.env.AGF_EFFECT;
const { Effect, Exit, Layer, Stream, Sink } = await import(effect + "/dist/index.js");
const { ChildProcessSpawner } = await import(effect + "/dist/process/index.js");
const { ExitCode, makeHandle, ProcessId } = await import(effect + "/dist/process/ChildProcessSpawner.js");
const { LayerNode } = await import(core + "/../../packages/util/src/effect/layer-node.ts");
const shellMod = await import(core + "/src/shell.ts");
const hooksMod = await import(core + "/src/plugin/hooks.ts");
const builderMod = await import(core + "/src/effect/app-node-builder.ts");
const configMod = await import(core + "/src/config.ts");
const envMod = await import(core + "/src/environment/index.ts");
const locationMod = await import(core + "/src/location.ts");
const globalMod = await import(core + "/../../packages/util/src/global.ts");
const locationFx = await import(core + "/test/fixture/location.ts");
const globalFx = await import(core + "/test/fixture/global.ts");
const envFx = await import(core + "/test/fixture/environment.ts");
const nodeFs = await import("node:fs");
const roots = { config: globalMod.Path.config, data: globalMod.Path.data, state: globalMod.Path.state, cache: globalMod.Path.cache, tmp: globalMod.Path.tmp, home: globalMod.Path.home };
const spawned = [];
let sawEnv = false;
const environment = Layer.effect(envMod.Environment.Service, Effect.gen(function* () {
  const host = yield* envMod.Environment.Service;
  return envMod.Environment.Service.of({
    ...host,
    spawner: ChildProcessSpawner.make((command) => Effect.sync(() => {
      spawned.push({ command: command.command, args: command.args, cwd: command.options && command.options.cwd });
      const output = Stream.empty;
      return makeHandle({
        pid: ProcessId(1),
        exitCode: Effect.succeed(ExitCode(0)),
        isRunning: Effect.succeed(false),
        kill: () => Effect.void,
        stdin: Sink.drain,
        stdout: output,
        stderr: Stream.empty,
        all: output,
        getInputFd: () => Sink.drain,
        getOutputFd: () => Stream.empty,
        unref: Effect.succeed(Effect.void),
      });
    })),
  });
})).pipe(Layer.provide(envFx.hostEnvironmentLayer));
const program = Effect.gen(function* () {
  const hooks = yield* hooksMod.PluginHooks.Service;
  const shell = yield* shellMod.Shell.Service;
  const checked = { command: "printf ok", cwd: "/tmp/checked-cwd", shell: "/bin/bash" };
  yield* hooks.register("shell", "create.before", (event) => Effect.sync(() => {
    sawEnv = !!(event.env && typeof event.env === "object" && Object.keys(event.env).length > 0);
    event.command = "env";
    event.cwd = "/tmp/rewritten-cwd";
    event.shell = "/bin/sh";
  }));
  const created = yield* shell.create(checked, (invocation) => Effect.sync(() => { invocation.cwd = "/tmp/prepare-cwd"; })).pipe(Effect.exit);
  return {
    ok: Exit.isSuccess(created),
    checked,
    info: Exit.isSuccess(created) ? { command: created.value.command, cwd: created.value.cwd, shell: created.value.shell } : null,
    cause: Exit.isFailure(created) ? String(created.cause).slice(0, 800) : "",
    spawned,
    sawEnv,
    roots,
    envAbsent: true,
  };
});
const layer = builderMod.AppNodeBuilder.build(LayerNode.group([shellMod.Shell.node, hooksMod.PluginHooks.node]), [
  locationMod.Location.node.replace(locationFx.tempLocationLayer),
  globalMod.Global.node.replace(globalFx.tempGlobalLayer),
  configMod.Config.node.replace(configMod.Config.testLayer()),
  envMod.Environment.node.replace(environment),
]);
const exit = await Effect.runPromise(program.pipe(Effect.scoped, Effect.provide(layer), Effect.timeout("20 seconds"), Effect.exit));
const payload = Exit.isSuccess(exit) ? { ...exit.value, violations } : { ok: false, cause: String(exit.cause).slice(0, 1800), spawned, sawEnv, violations, roots, envAbsent: true };
if (process.env.AGF_CANARY) nodeFs.writeFileSync(process.env.AGF_CANARY, JSON.stringify(payload.roots ?? {}), { mode: 0o600 });
console.log(JSON.stringify(payload));
`;

test("fresh roots, stock read equality, aliases, and no echo", async () => {
  assertCanaryControls();
  assertCounterOracles();
  assertHealthControls();
  assertRefusalControls();
  assertEmittedSyntax();
  assertLifetimeControl();
  assert.equal(leaked(["intentional echo " + SENTINEL]), true);
  const corpus = JSON.parse(readFileSync(corpusPath, "utf8"));
  assert.equal(Array.isArray(corpus), true);
  assert.equal(corpus.length, 70);
  const readRows = corpus.filter((row) => String(row.id).startsWith("R"));
  assert.equal(readRows.length, 9);
  assert.equal(readRows.filter((row) => row.decision === "deny").length, 7);
  assert.equal(readRows.filter((row) => row.decision === "allow").length, 2);
  assert.equal(readRows.find((row) => row.id === "R05").request.cwd, "/proc/self");
  assert.equal(readRows.some((row) => (row.id === "R05" ? "allow" : row.decision) !== row.decision), true);
  assert.equal(readRows.some((row) => ((row.id === "R08" || row.id === "R09") ? "deny" : row.decision) !== row.decision), true);
  const root = freshRoot("main");
  const helper = buildHelper(root);
  const dest = installGuard(root, helper);
  assert.equal(existsSync(join(root, "config", "opencode", "opencode.json")), false);
  const record = join(root, "requests.jsonl");
  const errLog = join(root, "helper-err.txt");
  writeFileSync(errLog, "", { mode: 0o600 });
  const real = wrapHelper(helper, record);
  writeFileSync(join(root, "bin", "agf"), readFileSync(join(root, "bin", "agf"), "utf8"), { mode: 0o755 });
  const plugins = join(root, "config", "opencode", "plugins");
  mkdirSync(join(plugins, "aaa-rename"), { recursive: true, mode: 0o700 });
  writeFileSync(join(plugins, "aaa-rename", "index.ts"), `
export default { id: "aaa-rename", setup(ctx) { ctx.session.hook("context", (event) => {
  const tool = event.tools && event.tools.read; if (!tool) return; event.tools.ReadFile = tool; delete event.tools.read;
}); } };
`);
  mkdirSync(join(plugins, "zzz-late"), { recursive: true, mode: 0o700 });
  writeFileSync(join(plugins, "zzz-late", "index.ts"), `
export default { id: "zzz-late", setup(ctx) { ctx.session.hook("context", (event) => {
  if (!event.options || event.options.produceLate !== true) return;
  const name = event.tools && (event.tools.ReadFile ? "ReadFile" : event.tools.read ? "read" : "");
  if (!name) return; event.tools.SecretRead = event.tools[name]; delete event.tools[name];
}); } };
`);
  mkdirSync(join(plugins, "benign"), { recursive: true, mode: 0o700 });
  writeFileSync(join(plugins, "benign", "index.ts"), `export default { id: "benign", setup() {} };\n`);
  const link = join(root, "home", "proc-link");
  symlinkSync("/proc/self/environ", link);
  const rows = readRows.filter((row) => row.id !== "R05");
  const cases = rows.map((row) => ({
    id: row.id,
    decision: row.decision,
    reason: row.reason,
    path: String(row.request.paths[0]).replace("${PROC_LINK}", link).replace("${NORMAL_FILE}", "normal.txt"),
  }));
  const script = join(root, "read.ts");
  const canary = join(root, "roots.json");
  writeFileSync(script, readHarness, { mode: 0o644 });
  const ran = runBun(root, script, {
    AGF_CONFIG_DIR: join(root, "config", "opencode"),
    AGF_GUARD_RECORD: record,
    AGF_GUARD_REAL: real,
    AGF_GUARD_ERR: errLog,
    AGF_CASES: JSON.stringify(cases),
    AGF_CANARY: canary,
    AGF_GUARD_SOURCE: source,
  });
  assert.equal(ran.status, 0, ran.stderr || ran.stdout);
  const report = lastJson(ran.stdout);
  assert.equal(report.ok, true, JSON.stringify(report).slice(0, 1500));
  assert.equal(report.envAbsent, true);
  assertCanary(report.roots, root);
  assertCanary(JSON.parse(readFileSync(canary, "utf8")), root);
  assert.equal(report.violations.length, 0);
  assert.equal(report.procReads, 0);
  const requiredPlugins = ["aaa-rename", "agf-secret-guard", "benign", "zzz-late"];
  for (const id of requiredPlugins) assert.equal(report.order.includes(id), true, id);
  assert.equal(activeInstalled(report.activeBefore, requiredPlugins), true);
  assert.equal(activeInstalled(report.activeAfter, requiredPlugins), true);
  assert.equal(report.factory.adapted, false);
  assert.equal(constructorCoherent(report.constructor), true);
  assert.equal(report.hostCwd, report.location);
  assert.equal(report.registered.includes("opencode.tool.read"), true);
  assert.equal(report.names.includes("read"), true);
  assert.equal(report.earlyNames.includes("ReadFile"), true);
  assert.equal(report.earlyNames.includes("read"), false);
  assert.equal(report.earlyNames.includes("InventedRead"), false);
  assert.equal(report.lateNames.includes("SecretRead"), true);
  assert.equal(report.lateNames.includes("ReadFile"), false);
  assert.equal(report.lateNames.includes("InventedRead"), false);
  const requests = readFileSync(record, "utf8").trim().split("\n").filter(Boolean).map((line) => JSON.parse(line));
  assert.equal(requests.length, 2 + report.cases.length + 3);
  assert.equal(requests[0].request.op, "read");
  assert.equal(requests[0].request.paths[0], "/proc/self/environ");
  assert.equal(requests[0].response.decision, "deny");
  assert.equal(requests[0].response.reason, "proc_environ_read");
  assert.equal(requests[1].request.paths[0], report.smokeAllowNative);
  assert.equal(requests[1].request.cwd, report.location);
  assert.equal(requests[1].response.decision, "allow");
  const corpusRequests = requests.slice(2, 2 + report.cases.length);
  const byId = new Map(readRows.map((row) => [row.id, row]));
  for (const [index, item] of report.cases.entries()) {
    const helper = corpusRequests[index].request;
    const response = corpusRequests[index].response;
    const frozen = byId.get(item.id);
    assert.equal(helper.op, "read", item.id);
    assert.equal(helper.source, "opencode", item.id);
    assert.equal(helper.version, 1, item.id);
    assert.equal(helper.paths[0], item.native, item.id);
    assert.equal(helper.cwd, item.cwd, item.id);
    assert.equal(item.cwd, report.location, item.id);
    assert.equal(item.actual, frozen.decision, item.id);
    assert.equal(response.decision, frozen.decision, item.id);
    assert.equal(response.reason, frozen.reason, item.id);
    assert.equal(item.delta, frozen.decision === "deny" ? 0 : 1, item.id);
    if (frozen.decision === "allow") assert.equal(item.reader, item.native, item.id);
    if (frozen.decision === "deny") assert.equal(item.reader, null, item.id);
  }
  const r05Row = byId.get("R05");
  const r05Record = join(root, "r05-requests.jsonl");
  const r05Canary = join(root, "r05-roots.json");
  writeFileSync(r05Record, "", { mode: 0o600 });
  const r05Run = runBun(root, script, {
    AGF_CONFIG_DIR: join(root, "config", "opencode"),
    AGF_GUARD_RECORD: r05Record,
    AGF_GUARD_REAL: real,
    AGF_GUARD_ERR: errLog,
    AGF_CASES: JSON.stringify([{ id: "R05", decision: r05Row.decision, reason: r05Row.reason, path: r05Row.request.paths[0] }]),
    AGF_CANARY: r05Canary,
    AGF_GUARD_SOURCE: source,
    AGF_LOGICAL_LOCATION: "/proc/self",
    AGF_R05_ONLY: "1",
  });
  assert.equal(r05Run.status, 0, r05Run.stderr || r05Run.stdout);
  const r05 = lastJson(r05Run.stdout);
  assert.equal(r05.ok, true, JSON.stringify(r05).slice(0, 1500));
  assert.equal(r05.envAbsent, true);
  assertCanary(r05.roots, root);
  assertCanary(JSON.parse(readFileSync(r05Canary, "utf8")), root);
  assert.equal(r05.location, "/proc/self");
  assert.equal(r05.hostCwd, "/proc/self");
  assert.equal(constructorCoherent(r05.constructor), true);
  assert.equal(r05.constructor.acquiredTarget, "/proc/self/environ");
  assert.equal(r05.constructor.rowCwd, "/proc/self");
  assert.equal(r05.factory.adapted, true);
  assert.equal(r05.factory.transformation.includes("Location.node.replace(tempLocationLayer)"), true);
  assert.equal(r05.violations.length, 0);
  assert.equal(r05.procReads, 0);
  assert.equal(r05.names.includes("read"), true);
  assert.equal(r05.order.includes("agf-secret-guard"), true);
  assert.equal(r05.cases.length, 1);
  assert.equal(r05.cases[0].id, "R05");
  assert.equal(r05.cases[0].cwd, "/proc/self");
  assert.equal(r05.cases[0].native, "/proc/self/environ");
  assert.equal(r05.cases[0].actual, "deny");
  assert.equal(r05.cases[0].delta, 0);
  assert.equal(r05.cases[0].reader, null);
  const r05Requests = readFileSync(r05Record, "utf8").trim().split("\n").filter(Boolean).map((line) => JSON.parse(line));
  assert.equal(r05Requests.length, 1);
  assert.equal(r05Requests[0].request.paths[0], "/proc/self/environ");
  assert.equal(r05Requests[0].request.cwd, "/proc/self");
  assert.equal(r05Requests[0].response.decision, r05Row.decision);
  assert.equal(r05Requests[0].response.reason, r05Row.reason);
  const nine = [...report.cases, r05.cases[0]];
  assert.equal(nine.length, 9);
  assert.equal(nine.filter((item) => item.expected === "deny").length, 7);
  assert.equal(nine.filter((item) => item.expected === "allow").length, 2);
  assert.equal(nine.every((item) => item.actual === item.expected), true);
  assert.equal(nine.every((item) => (item.id === "R05" ? "allow" : item.expected) === item.actual), false);
  assert.equal(nine.every((item) => ((item.id === "R08" || item.id === "R09") ? "deny" : item.expected) === item.actual), false);
  const tail = requests.slice(2 + report.cases.length);
  assert.equal(tail.length, 3);
  assert.equal(tail[0].request.paths[0], "/proc/self/environ");
  assert.equal(tail[0].response.decision, "deny");
  assert.equal(report.aliasDenied.delta, 0);
  assert.equal(report.aliasDenied.message, "guard_withheld");
  assert.equal(tail[1].request.paths[0], report.aliasAllowed.native);
  assert.equal(tail[1].request.cwd, report.location);
  assert.equal(tail[1].response.decision, "allow");
  assert.equal(report.aliasAllowed.success, true);
  assert.equal(report.aliasAllowed.delta, 1);
  assert.equal(report.aliasAllowed.reader, report.aliasAllowed.native);
  assert.equal(report.lateAlias.success, true);
  assert.equal(report.lateAlias.delta, 1);
  assert.equal(report.lateAlias.reader, join(report.location, "notes/late.txt"));
  assert.equal(report.lateAlias.message === "guard_withheld", false);
  assert.equal(tail[2].request.paths[0], report.tilde.native);
  assert.equal(report.tilde.success, true);
  assert.equal(report.tilde.delta, 1);
  assert.equal(report.tilde.reader, report.tilde.native);
  assert.equal(report.ping, true);
  const blobs = [ran.stdout, ran.stderr, r05Run.stdout, r05Run.stderr, readFileSync(record, "utf8"), readFileSync(r05Record, "utf8"), readFileSync(errLog, "utf8")];
  const audit = join(root, "state", "agent-fabric", "guard", "denials.jsonl");
  if (existsSync(audit)) blobs.push(readFileSync(audit, "utf8"));
  assert.equal(leaked(blobs), false);
  assert.equal(readFileSync(audit, "utf8").includes("/proc"), false);
  const row = JSON.parse(readFileSync(audit, "utf8").trim().split("\n")[0]);
  assert.equal(row.source, "opencode");
  assert.equal(row.rule, "proc_environ_read");
  writeFileSync(join(root, "installed.sha"), createHash("sha256").update(readFileSync(dest)).digest("hex") + "\n", { mode: 0o600 });
});

test("foreign config is preserved and a second fresh install creates no config", async () => {
  const seeded = freshRoot("seed");
  const helper = buildHelper(seeded);
  const config = join(seeded, "config", "opencode", "opencode.json");
  mkdirSync(dirname(config), { recursive: true, mode: 0o700 });
  const body = "{\"keep\":true}\n";
  writeFileSync(config, body, { mode: 0o644 });
  const before = createHash("sha256").update(readFileSync(config)).digest("hex");
  installGuard(seeded, helper);
  assert.equal(createHash("sha256").update(readFileSync(config)).digest("hex"), before);
  const synced = spawnSync(helper, [
    "sync", "--source", repo, "--tools", "opencode", "--assets", "secret-guard", "--agents", "planner", "--yes",
  ], { cwd: repo, env: allowEnv(seeded), encoding: "utf8" });
  assert.equal(synced.status, 0, synced.stderr || synced.stdout);
  assert.equal(readFileSync(config, "utf8"), body);
  const clean = freshRoot("clean");
  installGuard(clean, buildHelper(clean));
  assert.equal(existsSync(join(clean, "config", "opencode", "opencode.json")), false);
});

test("helper faults and a separate child block before the stock reader", async () => {
  assertCanaryControls();
  assertCounterOracles();
  assertHealthControls();
  assertRefusalControls();
  assertEmittedSyntax();
  assertLifetimeControl();
  const root = freshRoot("fault");
  const missing = join(root, "bin", "missing-agf");
  const dir = join(root, "config", "opencode", "plugins", "agf-secret-guard");
  mkdirSync(dir, { recursive: true, mode: 0o700 });
  writeFileSync(join(dir, "index.ts"), readFileSync(source, "utf8").replace('"__AGF_GUARD_HELPER__"', JSON.stringify(missing)), { mode: 0o644 });
  const script = join(root, "fault.ts");
  const canary = join(root, "roots.json");
  writeFileSync(script, readHarness, { mode: 0o644 });
  const bad = join(root, "bin", "bad-agf");
  writeFileSync(bad, "#!/bin/sh\nexit 2\n", { mode: 0o755 });
  const slow = join(root, "bin", "slow-agf");
  writeFileSync(slow, "#!/bin/sh\nsleep 30\n", { mode: 0o755 });
  for (const helperPath of [missing, bad, slow]) {
    writeFileSync(join(dir, "index.ts"), readFileSync(source, "utf8").replace('"__AGF_GUARD_HELPER__"', JSON.stringify(helperPath)), { mode: 0o644 });
    const ran = runBun(root, script, {
      AGF_CONFIG_DIR: join(root, "config", "opencode"),
      AGF_CASES: JSON.stringify([{ id: "R01", path: "/proc/self/environ" }]),
      AGF_CANARY: canary,
      AGF_GUARD_SOURCE: source,
      AGF_FAULT: "1",
    });
    assert.equal(ran.status, 0, ran.stderr || ran.stdout);
    const report = lastJson(ran.stdout);
    assert.equal(report.ok, true, JSON.stringify(report).slice(0, 800));
    assert.equal(report.envAbsent, true);
    assertCanary(report.roots, root);
    assertCanary(JSON.parse(readFileSync(canary, "utf8")), root);
    assert.equal(report.violations.length, 0);
    assert.equal(report.cases[0].delta, 0);
    assert.equal(report.cases[0].message, "guard_withheld");
    assert.equal(leaked([ran.stdout, ran.stderr]), false);
  }
  const child = freshRoot("child");
  const helper = buildHelper(child);
  installGuard(child, helper);
  const childScript = join(child, "child.ts");
  writeFileSync(childScript, readHarness, { mode: 0o644 });
  const childRun = runBun(child, childScript, {
    AGF_CONFIG_DIR: join(child, "config", "opencode"),
    AGF_CASES: JSON.stringify([{ id: "R01", path: "/proc/self/environ" }]),
    AGF_CANARY: join(child, "roots.json"),
    AGF_GUARD_SOURCE: source,
    AGF_FAULT: "1",
  });
  assert.equal(childRun.status, 0, childRun.stderr || childRun.stdout);
  const childReport = lastJson(childRun.stdout);
  const childCanary = join(child, "roots.json");
  assert.equal(childReport.ok, true, JSON.stringify(childReport).slice(0, 800));
  assert.equal(childReport.envAbsent, true);
  assertCanary(childReport.roots, child);
  assertCanary(JSON.parse(readFileSync(childCanary, "utf8")), child);
  assert.equal(childReport.violations.length, 0);
  assert.equal(childReport.cases[0].delta, 0);
  assert.equal(childReport.cases[0].message, "guard_withheld");
  assert.equal(existsSync(join(child, "config", "opencode", "opencode.json")), false);
});

test("late shell rewrite is not the synthetic spawn", async () => {
  const root = freshRoot("shell");
  const script = join(root, "shell.ts");
  const canary = join(root, "roots.json");
  writeFileSync(script, shellHarness, { mode: 0o644 });
  const ran = runBun(root, script, { AGF_CANARY: canary });
  assert.equal(ran.status, 0, ran.stderr || ran.stdout);
  const report = lastJson(ran.stdout);
  assert.equal(report.ok, true, JSON.stringify(report).slice(0, 1500));
  assert.equal(report.envAbsent, true);
  assertCanary(report.roots, root);
  assertCanary(JSON.parse(readFileSync(canary, "utf8")), root);
  assert.equal(report.violations.length, 0);
  assert.equal(report.sawEnv, true);
  const sink = report.spawned[0];
  assert.equal(sink.command, "/bin/sh");
  assert.equal(sink.args.includes("env"), true);
  assert.equal(sink.cwd, "/tmp/prepare-cwd");
  assert.notEqual(sink.command, report.checked.shell);
  assert.notEqual(report.info.command, report.checked.command);
  assert.notEqual(report.info.cwd, report.checked.cwd);
  assert.equal(report.info.shell, "/bin/sh");
  assert.equal(leaked([ran.stdout, ran.stderr]), false);
});

test("pi package prerequisite is explicit and does not use host fallback", () => {
  const pkg = JSON.parse(readFileSync(join(piPackage, "package.json"), "utf8"));
  assert.equal(pkg.version, "1.4.0");
  const resolved = createRequire(join(piPackage, "package.json")).resolve("jiti");
  assert.equal(resolved.startsWith(piPackage), true);
});
