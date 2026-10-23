import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createRequire } from "node:module";
import { execFileSync } from "node:child_process";

// Reuse the installed host's loader/parser; no npm install or provider call.
const host = process.env.PI_PACKAGE || join(execFileSync("npm", ["root", "-g"], { encoding: "utf8" }).trim(), "@earendil-works/pi-coding-agent");
const require = createRequire(join(host, "package.json"));
const { createJiti } = require("jiti");
const jiti = createJiti(import.meta.url, { alias: { "@earendil-works/pi-coding-agent": join(host, "dist/index.js") } });
const { default: extension, loadModes } = await jiti.import("./index.ts");

test("Fabric role lifecycle, exact tool guard, branch restore and invalid definitions", async () => {
  const root = mkdtempSync(join(tmpdir(), "fabric-modes-"));
  const previous = process.env.PI_CODING_AGENT_DIR;
  process.env.PI_CODING_AGENT_DIR = root;
  try {
    mkdirSync(join(root, "agents"));
    const write = (name, tools, body = `Role ${name}`) => writeFileSync(join(root, "agents", `${name}.md`), `---\nname: ${name}\ndescription: Test ${name}\ntools: ${tools}\n---\n${body}\n`);
    write("reviewer", "read, grep");
    write("worker", "[read, edit, bash]");
    const handlers = {}, commands = {}, entries = [], notices = [];
    let active = ["read", "edit", "bash", "custom"];
    let branch = entries, flag, idle = true;
    const pi = {
      registerFlag() {}, getFlag: () => flag,
      on: (name, handler) => { handlers[name] = handler; },
      registerCommand: (name, command) => { commands[name] = command; },
      getActiveTools: () => [...active],
      getAllTools: () => ["read", "grep", "edit", "bash", "custom"].map(name => ({ name })),
      setActiveTools: tools => { active = [...tools]; },
      appendEntry: (customType, data) => entries.push({ type: "custom", customType, data }),
    };
    const ctx = { hasUI: true, isIdle: () => idle, sessionManager: { getBranch: () => branch },
      ui: { notify: msg => notices.push(msg), setStatus() {}, select: async () => undefined } };
    extension(pi);
    await handlers.session_start({}, ctx);
    assert.equal(active.length, 4);
    const command = name => commands["agf-mode"].handler(name, ctx);
    await command("reviewer");
    assert.deepEqual(active, ["read", "grep"]);
    assert.equal(handlers.tool_call({ toolName: "custom" }).block, true);
    assert.equal(handlers.tool_call({ toolName: "bash" }).block, true);
    assert.equal(handlers.tool_call({ toolName: "read" }), undefined);
    const prompt = handlers.before_agent_start({ systemPrompt: "Base" }).systemPrompt;
    assert.match(prompt, /^Base/);
    assert.match(prompt, /Role reviewer/);
    await command("worker");
    assert.deepEqual(active, ["read", "edit", "bash"]);
    assert.doesNotMatch(handlers.before_agent_start({ systemPrompt: "Base" }).systemPrompt, /Role reviewer/);
    await command("off");
    assert.deepEqual(active, ["read", "edit", "bash", "custom"]);
    assert.equal(handlers.before_agent_start({ systemPrompt: "Base" }), undefined);
    branch = entries.slice(0, 1);
    await handlers.session_tree({}, ctx);
    assert.deepEqual(active, ["read", "grep"]);
    await command("missing");
    assert.deepEqual(active, ["read", "grep"]);
    idle = false;
    await command("worker");
    assert.deepEqual(active, ["read", "grep"]);
    idle = true;
    branch = [];
    await handlers.session_start({}, ctx);
    assert.deepEqual(active, ["read", "edit", "bash", "custom"]);
    // An off entry owns its branch's baseline, not the departing branch's.
    branch = [{ type: "custom", customType: "agent-fabric-mode", data: { name: "off", baseline: ["read"] } }];
    await command("worker");
    await handlers.session_tree({}, ctx);
    assert.deepEqual(active, ["read"]);
    active = ["read", "bash", "custom"];
    extension(pi); // Fresh runtime must also restore a persisted off baseline.
    await handlers.session_start({}, ctx);
    assert.deepEqual(active, ["read"]);
    branch = [];
    flag = "worker";
    await handlers.session_start({}, ctx);
    assert.deepEqual(active, ["read", "edit", "bash"]);
    assert.equal(entries.at(-1).data.name, "worker");
    write("bad", "42");
    assert.throws(() => loadModes(join(root, "agents")), /Invalid Pi/);
    flag = "reviewer";
    await handlers.session_start({}, ctx);
    assert.deepEqual(active, []);
    assert.equal(handlers.tool_call({ toolName: "read" }).block, true);
    assert.ok(notices.length >= 3);
  } finally {
    if (previous === undefined) delete process.env.PI_CODING_AGENT_DIR;
    else process.env.PI_CODING_AGENT_DIR = previous;
    rmSync(root, { recursive: true, force: true });
  }
});
