// Spec: docs/specs/task-retrospective.md — Contextual delivery (phase 3).
// Real disposable Go CLI + registered native callbacks. No native server/model;
// signing identities and task-A trace are protocol fixtures, not live review.
import test from "node:test";
import assert from "node:assert/strict";
import { createHash, generateKeyPairSync, sign } from "node:crypto";
import { execFileSync, spawnSync } from "node:child_process";
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, existsSync, readdirSync, statSync, unlinkSync } from "node:fs";
import path from "node:path";
import os from "node:os";
import nativePlugin from "../opencode";

test("accepted task A -> distinct task B native context callback; exclusion, opt-out, empty and corrupt memory", async (t) => {
  const previous = { ...process.env };
  const root = mkdtempSync(path.join(os.tmpdir(), "agf-context-"));
  const project = path.join(root, "project"), state = path.join(root, "state"), bin = path.join(root, "agf");
  mkdirSync(project, { mode: 0o700 });
  const repo = path.resolve(__dirname, "../../../..");
  execFileSync("go", ["build", "-o", bin, "./cmd/agent-fabric"], { cwd: repo, timeout: 120_000 });
  const settings = path.join(root, "settings.json"), bindingPath = path.join(root, "binding.json"), trustPath = path.join(root, "trust.json"), tracePath = path.join(root, "task-a.json"), envelopePath = path.join(root, "proposal.json");
  const write = (file: string, value: unknown) => writeFileSync(file, JSON.stringify(value), { mode: 0o600 });
  const hash = (bytes: string | Buffer) => createHash("sha256").update(bytes).digest("hex");
  const producer = generateKeyPairSync("ed25519"), reviewer = generateKeyPairSync("ed25519");
  const publicKey = (key: typeof producer.publicKey) => key.export({ format: "der", type: "spki" }).subarray(-32).toString("base64");
  write(trustPath, { schema_version: 1, identities: { producer: { role: "producer", public_key: publicKey(producer.publicKey), project }, reviewer: { role: "reviewer", public_key: publicKey(reviewer.publicKey), project } } });
  write(tracePath, { schema_version: 1, identities: { root_session_id: "task-a", session_id: "task-a", execution_root: project, source_revision: "v1" }, messages: [{ id: "intent", seq: 1, role: "user", text: "Read the declared catalog before selecting a price." }, { id: "done", seq: 3, role: "assistant", kind: "final", text: "Catalog read; selected the recorded price." }], calls: [{ id: "read", seq: 2, tool: "read", status: "success", input: "catalog.json", result: [{ id: "result", text: "SKU-A costs 7" }] }] });
  // Canonical typed candidate property order is the public Go signing contract.
  const candidate = { schema_version: 1, operation: "add", lesson_id: "catalog", expected_revision: 0, project, scope: "catalog-task", version: "v1", consumer: "executor", advice: "Read the declared catalog before selecting a price.", producer: "producer", origin_session: "task-a", evidence: { path: tracePath, sha256: hash(readFileSync(tracePath)), record_ids: ["read"] } };
  const digest = hash(JSON.stringify(candidate));
  write(envelopePath, { candidate, proposal_signature: sign(null, Buffer.from(`agf-lesson-proposal:v1\n${digest}`), producer.privateKey).toString("base64"), acceptance: { candidate_sha256: digest, reviewer: "reviewer", approved: true, signature: sign(null, Buffer.from(`agf-lesson-acceptance:v1\n${digest}\nreviewer\ntrue`), reviewer.privateKey).toString("base64") } });
  const query = { project, scope: "catalog-task", version: "v1", consumer: "executor" };
  const binding: any = { schema_version: 1, state_dir: state, trust: trustPath, sessions: { "task-b": query }, excluded_sessions: ["monitor"] };
  const rows: Record<string, any> = { "task-b": { id: "task-b", title: "Later catalog task", location: { directory: project } }, monitor: { id: "monitor", title: "Monitor", location: { directory: project } }, corrector: { id: "corrector", title: "agf-learning-origin:v1:fixture", location: { directory: project } } };
  let cleanup: (() => Promise<void> | void) | undefined;
  const nativeSpawn = require("node:child_process").spawn;
  const spawnLog = t.mock.method(require("node:child_process"), "spawn", (command: string, args: string[], options: unknown) => {
    console.log("NATIVE CALLBACK COMMAND", JSON.stringify([command, ...args]));
    return nativeSpawn(command, args, options);
  });
  try {
    process.env.AGF_BIN = bin; process.env.AGF_CONFIG = settings; process.env.AGF_LESSONS_CONTEXT = bindingPath;
    process.env.AGF_RETROSPECTIVE_STATE = state;
    delete process.env.AGF_PLUGIN_FAKE_HEALTH;
    write(settings, { retrospective: { enabled: true } }); write(bindingPath, binding);
    const applyArgs = ["retrospective", "lessons", "apply", "--state-dir", state, "--trust", trustPath, "--input", envelopePath];
    const applied = execFileSync(bin, applyArgs, { encoding: "utf8", timeout: 10_000 });
    console.log("COMMAND", JSON.stringify([bin, ...applyArgs]), "EXIT 0", applied.trim());
    assert.equal(JSON.parse(applied).status, "add");
    const hooks: Record<string, (event: any) => Promise<void>> = {};
    cleanup = await nativePlugin.setup({ app: { version: "2.0.24" }, location: { directory: project }, session: { async get({ sessionID }: any) { assert.ok(rows[sessionID], "fixture identity available"); return rows[sessionID]; }, async hook(name: string, fn: any) { hooks[name] = fn; return { async dispose() {} }; } }, tool: { async hook() { return { async dispose() {} }; } }, event: { async *subscribe() {} } } as any);
    assert.ok(hooks.context, "real native context extension point registered");
    const consume = async (id = "task-b") => {
      const event = { sessionID: id, agent: "implementor", model: { providerID: "unchanged", id: "unchanged" }, system: [{ type: "text", text: "Original authority" }], messages: [{ role: "user", content: "Select the price for SKU-B." }], tools: { read: {} }, options: { temperature: 0.42 } };
      const original = structuredClone(event);
      await hooks.context(event);
      assert.deepEqual(event.messages, original.messages); assert.deepEqual(event.model, original.model); assert.deepEqual(event.options, original.options); assert.deepEqual(event.tools, original.tools); assert.deepEqual(event.system[0], original.system[0]);
      return event.system;
    };
    const delivered = await consume();
    assert.equal(delivered.length, 2); assert.match(delivered[1].text, /NONHUMAN ADVISORY ONLY/); assert.match(delivered[1].text, /catalog revision 1/); assert.match(delivered[1].text, /Read the declared catalog/);
    const receiptPath = path.join(state, "lesson-deliveries/task-b.json");
    const receipt = JSON.parse(readFileSync(receiptPath, "utf8"));
    assert.equal(receipt.status, "context_hook_appended"); assert.equal(receipt.session_id, "task-b"); assert.deepEqual(receipt.context.lessons, [{ id: "catalog", revision: 1 }]); assert.equal(statSync(receiptPath).mode & 0o777, 0o600);
    console.log("CONTROLLED task-a accepted ingestion -> task-b registered context callback + receipt", JSON.stringify(receipt));
    // No precedent, wrong version/project/consumer, and no binding must not leak.
    for (const change of [{ scope: "unrelated-task" }, { version: "v2" }, { consumer: "corrector" }, { project: root }]) {
      binding.sessions["task-b"] = { ...query, ...change }; write(bindingPath, binding);
      assert.equal((await consume()).length, 1);
    }
    binding.sessions = {}; write(bindingPath, binding); assert.equal((await consume()).length, 1);
    binding.sessions = { "task-b": query }; write(bindingPath, binding);
    for (const parent of ["monitor", "corrector"]) {
      rows["task-b"].parentID = parent; assert.equal((await consume()).length, 1);
      binding.sessions[parent] = query; write(bindingPath, binding); assert.equal((await consume(parent)).length, 1);
    }
    delete rows["task-b"].parentID;
    writeFileSync(path.join(state, "origins/task-b"), "suppressed", { mode: 0o600 });
    assert.equal((await consume()).length, 1);
    // Use a fresh later session to avoid deleting even task-owned suppression.
    rows["task-c"] = { id: "task-c", title: "Another catalog task", location: { directory: project } }; binding.sessions["task-c"] = query; write(bindingPath, binding);
    write(settings, { retrospective: { enabled: false } }); assert.equal((await consume("task-c")).length, 1);
    write(settings, { retrospective: { enabled: true } });
    const disable = path.join(project, ".retrospective-disabled"); writeFileSync(disable, "fixture");
    assert.equal((await consume("task-c")).length, 1); unlinkSync(disable);
    binding.state_dir = path.join(root, "missing-state"); write(bindingPath, binding); assert.equal((await consume("task-c")).length, 1); assert.equal(existsSync(binding.state_dir), false);
    binding.state_dir = state; write(bindingPath, binding);
    const corrupt = Buffer.from('{"schema_version":1,"records":BROKEN'); writeFileSync(path.join(state, "lessons.json"), corrupt);
    assert.equal((await consume("task-c")).length, 1); assert.deepEqual(readFileSync(path.join(state, "lessons.json")), corrupt);
    assert.ok(readdirSync(path.join(state, "observer-errors")).length >= 3, "failures surfaced as diagnostics, not silent empty authority");
    assert.equal(existsSync(path.join(state, "lesson-deliveries/task-c.json")), false);
    const request = JSON.stringify({ session_id: "task-c", project, sessions: { "task-c": { title: "Catalog", directory: project } } });
    const failed = spawnSync(bin, ["retrospective", "lessons", "context", "--binding", bindingPath], { input: request, encoding: "utf8", timeout: 10_000 });
    assert.equal(failed.status, 1); assert.match(failed.stderr, /lesson context state/); assert.equal(failed.stdout, "");
    await cleanup?.(); cleanup = undefined;
    delete process.env.AGF_LESSONS_CONTEXT;
    const names: string[] = [];
    cleanup = await nativePlugin.setup({ app: { version: "2.0.24" }, location: { directory: project }, session: { async get() {}, async hook(name: string) { names.push(name); return { async dispose() {} }; } }, tool: { async hook() { return { async dispose() {} }; } }, event: { async *subscribe() {} } } as any);
    assert.deepEqual(names, ["prompt"], "unset binding never registers recall or reads memory");
    console.log("CONTROLLED unrelated/version/project/consumer/monitor/corrector/descendant/store-origin/opt-out/no-memory/corrupt-state exclusions PASS");
  } finally { await cleanup?.(); spawnLog.mock.restore(); process.env = previous; }
});
