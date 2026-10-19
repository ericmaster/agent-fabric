// Spec: docs/specs/task-retrospective.md — native binding public setup boundary.
import test from "node:test";
import assert from "node:assert/strict";
import { EventEmitter } from "node:events";
import { chmodSync, mkdtempSync, mkdirSync, readdirSync, readFileSync, statSync, symlinkSync, writeFileSync } from "node:fs";
import path from "node:path";
import os from "node:os";
import nativePlugin from "../opencode";

for (const version of ["2.0.27", "2.0.30"]) test(`native ${version} unbound setup never resolves CWD/HOME or registers repeated capture hooks`, async (t) => {
  const root = mkdtempSync(path.join(os.tmpdir(), "agf-binding-"));
  const previous = { ...process.env }, cwd = process.cwd();
  let children = 0, hooks = 0;
  const mock = t.mock.method(require("node:child_process"), "spawn", () => { children++; throw Error("unexpected child"); });
  try {
    process.chdir(root); process.env.HOME = root; delete process.env.AGF_BIN;
    process.env.AGF_RETROSPECTIVE_STATE = path.join(root, "state");
    for (const dir of [root, path.join(root, ".local/bin"), path.join(root, ".opencode/bin")]) {
      mkdirSync(dir, { recursive: true }); writeFileSync(path.join(dir, "agent-fabric"), "#!/bin/sh\nexit 1\n", { mode: 0o700 });
    }
    const cleanup = await nativePlugin.setup({ app: { version }, location: { directory: root },
      session: { async get() {}, async hook() { hooks++; return { async dispose() {} }; } }, tool: { async hook() { hooks++; return { async dispose() {} }; } },
      event: { async *subscribe() { for (let n = 0; n < 432; n++) yield { type: "session.execution.succeeded", data: { sessionID: "synthetic" } }; } },
    } as any);
    await cleanup(); await cleanup();
    assert.equal(children, 0); assert.equal(hooks, 0);
    const reports = readdirSync(path.join(root, "state/observer-errors")); assert.equal(reports.length, 1);
    const file = path.join(root, "state/observer-errors", reports[0]);
    assert.equal(statSync(file).mode & 0o777, 0o600);
    assert.match(readFileSync(file, "utf8"), /missing_or_unsafe_explicit_binding/);
  } finally { mock.mock.restore(); process.chdir(cwd); process.env = previous; }
});

test("native binding rejects relative, symlink, writable file/parent and non-owned paths without children", async (t) => {
  const previous = { ...process.env };
  const root = mkdtempSync(path.join(os.tmpdir(), "agf-binding-"));
  const bin = path.join(root, "agf"); writeFileSync(bin, "synthetic", { mode: 0o700 });
  const link = path.join(root, "link"); symlinkSync(bin, link);
  let children = 0;
  const mock = t.mock.method(require("node:child_process"), "spawn", () => { children++; throw Error("unexpected"); });
  const context: any = { app: { version: "2.0.27" }, location: { directory: root }, session: { async get() {}, async hook() { assert.fail("capture registered"); } }, tool: { async hook() {} }, event: { async *subscribe() {} } };
  try {
    process.env.AGF_RETROSPECTIVE_STATE = path.join(root, "state");
    for (const candidate of ["agf", link, "/bin/sh"]) { process.env.AGF_BIN = candidate; await (await nativePlugin.setup(context))(); }
    process.env.AGF_BIN = bin; chmodSync(bin, 0o720); await (await nativePlugin.setup(context))();
    chmodSync(bin, 0o700); chmodSync(root, 0o770); await (await nativePlugin.setup(context))();
    assert.equal(children, 0);
  } finally { mock.mock.restore(); process.env = previous; }
});

for (const version of ["2.0.27", "2.0.30"]) test(`native ${version} capability probe is bounded, private, once-only and cannot grant capture on failure`, async (t) => {
  const previous = { ...process.env };
  const root = mkdtempSync(path.join(os.tmpdir(), "agf-binding-"));
  const bin = path.join(root, "agf"); writeFileSync(bin, "synthetic", { mode: 0o700 });
  const cp = require("node:child_process");
  try {
    process.env.AGF_BIN = bin; process.env.AGF_RETROSPECTIVE_STATE = path.join(root, "state");
    process.env.AGF_OBSERVER_SERVER = "https://user:CANARY@example.invalid/?token=CANARY";
    for (const mode of ["old", "error", "throw", "timeout", "flood", "wrong-exit", "changed"]) {
      let children = 0, killed = 0;
      t.mock.timers.enable({ apis: ["setTimeout"] });
      const mock = t.mock.method(cp, "spawn", (cli: string, argv: string[], options: any) => {
        children++; assert.equal(cli, bin);
        assert.deepEqual(argv, ["retrospective", "ingest", "--harness", "opencode", "--help"]);
        assert.deepEqual(options.stdio, ["ignore", "pipe", "pipe"]);
        if (mode === "throw") throw Error("CANARY");
        const child: any = new EventEmitter(); child.pid = 123; child.stdout = new EventEmitter(); child.stderr = new EventEmitter();
        child.kill = () => { killed++; queueMicrotask(() => child.emit("close", null)); };
        queueMicrotask(() => {
          if (mode === "timeout") return;
          if (mode === "error") child.emit("error", Error("CANARY"));
          if (mode === "changed") writeFileSync(bin, "changed");
          child.stderr.emit("data", Buffer.from(mode === "flood" ? "CANARY".repeat(1000) : mode === "old" ? "agent-fabric: unexpected arguments: ingest --harness opencode --help CANARY" : "agent-fabric: flag: help requested\n"));
          child.emit("close", mode === "wrong-exit" ? 0 : 1);
        });
        return child;
      });
      try {
        const setup = nativePlugin.setup({ app: { version }, location: { directory: root },
          session: { async get() {}, async hook() { assert.fail("capture registered"); } }, tool: { async hook() { assert.fail("capture registered"); } }, event: { async *subscribe() { assert.fail("subscribed"); } },
        } as any);
        await new Promise(setImmediate); t.mock.timers.tick(5000);
        const cleanup = await setup; await cleanup(); await cleanup();
        assert.equal(children, 1, mode); assert.equal(killed, mode === "timeout" || mode === "flood" ? 1 : 0, mode);
      } finally { mock.mock.restore(); t.mock.timers.reset(); }
    }
    const reports = readdirSync(path.join(root, "state/observer-errors")); assert.equal(reports.length, 7);
    for (const report of reports) {
      const body = readFileSync(path.join(root, "state/observer-errors", report), "utf8");
      assert.ok(!body.includes("CANARY"));
      const metadata = JSON.parse(body); assert.equal(metadata.observerPID, process.pid); assert.equal(metadata.resolutionSource, "explicit_AGF_BIN"); assert.equal(metadata.declaredObserverOrigin, undefined); assert.ok(metadata.timestamp);
    }
  } finally { process.env = previous; }
});

test("compatible explicit binding uses static ingest argv, rejects later replacement once, reloads independently", async (t) => {
  const previous = { ...process.env }, root = mkdtempSync(path.join(os.tmpdir(), "agf-binding-"));
  const bin = path.join(root, "agf"); writeFileSync(bin, "synthetic", { mode: 0o700 });
  process.env.AGF_BIN = bin; process.env.AGF_RETROSPECTIVE_STATE = path.join(root, "state"); process.env.AGF_OBSERVER_SERVER = "http://127.0.0.1:4097";
  let probes = 0, ingests = 0, after: any, intake: any;
  const mock = t.mock.method(require("node:child_process"), "spawn", (cli: string, argv: string[]) => {
    assert.equal(cli, bin);
    const child: any = new EventEmitter(); child.stdout = new EventEmitter(); child.stderr = new EventEmitter(); child.stdin = new EventEmitter(); child.kill = () => {};
    if (argv.at(-1) === "--help") { probes++; queueMicrotask(() => { child.stderr.emit("data", Buffer.from("agent-fabric: flag: help requested\n")); child.emit("close", 1); }); }
    else { ingests++; assert.deepEqual(argv, ["retrospective", "ingest", "--harness", "opencode"]); child.stdin.end = (text: string) => { assert.ok(JSON.parse(text).records.length); queueMicrotask(() => child.emit("close", 0)); }; }
    return child;
  });
  const context: any = { app: { version: "2.0.27" }, location: { directory: root },
    session: { async get() { return { id: "synthetic", location: { directory: root } }; }, async hook(_name: string, fn: any) { intake = fn; return { async dispose() {} }; } },
    tool: { async hook(_name: string, fn: any) { after = fn; return { async dispose() {} }; } },
    event: { async *subscribe() {
      yield { type: "session.created", data: { sessionID: "synthetic" } };
      yield { type: "session.execution.succeeded", data: { sessionID: "synthetic" } };
      writeFileSync(bin, "replacement");
      for (let n = 0; n < 432; n++) { yield { type: "session.text.ended", data: { sessionID: "synthetic", assistantMessageID: "answer", text: "synthetic" } }; yield { type: "session.execution.succeeded", data: { sessionID: "synthetic" } }; }
    } },
  };
  try {
    const cleanup = await nativePlugin.setup(context); await cleanup();
    await intake({ sessionID: "synthetic", messageID: "disposed", prompt: { text: "must not capture" } });
    await after({ tool: "read", status: "completed", sessionID: "synthetic", input: { path: bin } });
    assert.equal(probes, 1); assert.equal(ingests, 1);
    const reports = readdirSync(path.join(root, "state/observer-errors")); assert.equal(reports.length, 1);
    const report = JSON.parse(readFileSync(path.join(root, "state/observer-errors", reports[0]), "utf8"));
    assert.equal(report.stderr, "explicit_binding_changed"); assert.equal(report.declaredObserverOrigin, "http://127.0.0.1:4097");
    await (await nativePlugin.setup({ ...context, event: { async *subscribe() {} } }))(); assert.equal(probes, 2);
  } finally { mock.mock.restore(); process.env = previous; }
});
