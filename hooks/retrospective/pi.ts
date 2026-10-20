// Native Pi 1.0.0 observer. No provider, model, tool or session mutation.
import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { constants, openSync, closeSync, fstatSync, readSync, realpathSync, statSync } from "node:fs";
import path from "node:path";

type Row = Record<string, any>;
type Context = { cwd: string; sessionManager: { getSessionId(): string; getEntries(): Row[]; getHeader(): Row | null; getSessionName(): string | undefined } };
type API = { on(name: string, handler: (event: any, ctx: Context) => any): unknown };
export type PiEnvelope = { version: 1; harness: "pi"; sdk_version: "1.0.0"; session_id: string; project_dir: string; trigger: boolean; records: Row[] };
type Snapshot = { path: string; sha256: string; metadata: string[] };
type ObservedTool = { message: string; args: Row; name: string; readBefore?: Snapshot };
type Transport = (args: string[], input: unknown) => Promise<{ code: number; stdout: string; stderr?: string }>;
const digest = (value: string | Buffer) => createHash("sha256").update(value).digest("hex");
const text = (content: any): string => typeof content === "string" ? content : Array.isArray(content) ? content.filter(p => p?.type === "text" && typeof p.text === "string").map(p => p.text).join("\n") : "";
const secret = (file: string) => /(^|[\\/])(\.env|credentials|secrets|id_rsa|id_ed25519)/i.test(file) || file.split(/[\\/]/).some(segment =>
  /^(?:\.npmrc|\.netrc|\.git-credentials|\.pypirc)(?:[.-]|$)/i.test(segment) ||
  /^(?:\.?keys|\.?tokens|\.ssh|\.aws|\.azure)$/i.test(segment) ||
  /^(?:\.?auth|\.?tokens?|access[-_]?tokens?|refresh[-_]?tokens?|api[-_]?keys?|private[-_]?keys?)(?:\.(?:json|jsonl|yaml|yml|toml|ini|conf|txt))?(?:\.(?:bak|old|backup))?$/i.test(segment) ||
  /\.(?:env|pem|p12|pfx|key|keys|token|tokens)$/i.test(segment));
const marker = "agf-learning-origin:v1:";
const LIMIT = 512;

// Exact-root, owner-checked, non-symlink observations only; no secret/config discovery.
function bytes(file: string, limit: number): { data: Buffer; metadata: string[] } | undefined {
  let fd: number | undefined;
  try {
    const resolved = realpathSync(file);
    if (secret(file) || secret(resolved) || resolved !== path.resolve(file)) return;
    fd = openSync(file, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
    const a = fstatSync(fd, { bigint: true });
    if (!a.isFile() || a.size > BigInt(limit) || a.nlink !== 1n || a.uid !== BigInt(process.getuid!())) return;
    const meta = (s: typeof a) => [s.dev, s.ino, s.mode, s.uid, s.gid, s.size, s.mtimeNs, s.ctimeNs].map(String);
    const data = Buffer.alloc(Number(a.size) + 1), count = readSync(fd, data, 0, data.length, 0);
    if (count !== Number(a.size) || JSON.stringify(meta(a)) !== JSON.stringify(meta(fstatSync(fd, { bigint: true }))) || realpathSync(file) !== path.resolve(file) || JSON.stringify(meta(a)) !== JSON.stringify(meta(statSync(file, { bigint: true })))) return;
    return { data: data.subarray(0, count), metadata: meta(a) };
  } catch { return; } finally { if (fd !== undefined) closeSync(fd); }
}

export function piRevision(directory: string): string | undefined {
  const read = (file: string) => bytes(file, 4096)?.data.toString("utf8").trim();
  try {
    if (realpathSync(directory) !== path.resolve(directory)) return;
    let git = path.join(directory, ".git");
    if (!statSync(git).isDirectory()) {
      const pointer = read(git);
      if (!pointer?.startsWith("gitdir: ") || pointer.includes("\n")) return;
      git = path.resolve(directory, pointer.slice(8));
    }
    if (realpathSync(git) !== git || statSync(git).uid !== process.getuid!()) return;
    const head = read(path.join(git, "HEAD"));
    let revision = head;
    if (head?.startsWith("ref: ")) {
      const ref = head.slice(5);
      if (!/^refs\/heads\/[A-Za-z0-9_./-]+$/.test(ref) || ref.split("/").some(p => !p || p === "." || p === "..")) return;
      let common = git;
      const commonFile = path.join(git, "commondir");
      try { statSync(commonFile); const value = read(commonFile); if (!value) return; common = path.resolve(git, value); }
      catch (e: any) { if (e.code !== "ENOENT") return; }
      revision = read(path.join(common, ref));
    }
    if (revision && /^[a-f0-9]{40}(?:[a-f0-9]{24})?$/.test(revision) && read(path.join(git, "HEAD")) === head) return revision;
  } catch { return; }
}

export function piSnapshot(args: Row, directory: string): Snapshot | undefined {
  if (!args || typeof args.path !== "string" || Object.keys(args).some(k => !["path", "offset", "limit"].includes(k)) || [args.offset, args.limit].some(v => v !== undefined && (!Number.isSafeInteger(v) || v < 1))) return;
  try {
    const root = realpathSync(directory), file = path.resolve(directory, args.path), resolved = realpathSync(file);
    if (root !== path.resolve(directory) || !file.startsWith(root + path.sep) || secret(directory) || secret(args.path) || secret(file) || secret(resolved)) return;
    const observed = bytes(file, 65536);
    if (observed) return { path: file, sha256: digest(observed.data), metadata: observed.metadata };
  } catch { return; }
}

/** Static argv, bounded subprocess; stdout is parsed only for the context protocol. */
export function piTransport(cli: string, fingerprint: string): Transport {
  return async (args, input) => {
    if (piBinaryFingerprint(cli) !== fingerprint) throw Error("AGF binding changed");
    return new Promise((resolve, reject) => {
      const child = spawn(cli, args, { stdio: ["pipe", "pipe", "pipe"] });
      let output = "", stderr = "", size = 0, done = false;
      const finish = (error?: Error, code = 1) => { if (done) return; done = true; clearTimeout(timer); error ? reject(error) : resolve({ code, stdout: output, stderr }); };
      const timer = setTimeout(() => { child.kill("SIGKILL"); finish(Error("AGF command timeout")); }, 5000);
      child.stdout.on("data", (chunk: Buffer) => { size += chunk.length; if (size > 32768) { child.kill("SIGKILL"); finish(Error("AGF output bound")); } else output += chunk.toString(); });
      child.stderr.on("data", (chunk: Buffer) => { size += chunk.length; if (size > 32768) { child.kill("SIGKILL"); finish(Error("AGF output bound")); } else stderr += chunk.toString(); });
      child.on("error", () => finish(Error("AGF spawn failed")));
      child.on("close", code => finish(piBinaryFingerprint(cli) !== fingerprint ? Error("AGF binding changed") : undefined, code ?? 1));
      child.stdin.on("error", () => finish(Error("AGF stdin failed")));
      child.stdin.end(JSON.stringify(input));
    });
  };
}

export function piBinaryFingerprint(cli: string): string | undefined {
  try {
    if (!path.isAbsolute(cli) || realpathSync(cli) !== cli) return;
    const s = statSync(cli, { bigint: true }), parent = statSync(path.dirname(cli), { bigint: true });
    if (!s.isFile() || s.uid !== BigInt(process.getuid!()) || parent.uid !== s.uid || (s.mode & 0o022n) !== 0n || (parent.mode & 0o022n) !== 0n || (s.mode & 0o100n) === 0n) return;
    const observed = bytes(cli, 64 * 1024 * 1024);
    if (!observed) return;
    return digest([cli, ...observed.metadata, parent.dev, parent.ino, parent.mode, digest(observed.data)].join(":"));
  } catch { return; }
}

export class PiObserver {
  private session = "";
  private records: Row[] = [];
  private seen = new Set<string>();
  private prompts = new Map<string, { revision?: string; sha256: string; synthetic: boolean; inputSource?: string }>();
  private inputs: { text: string; source: string }[] = [];
  private reads = new Map<string, Row>();
  private tools = new Map<string, ObservedTool>();
  private suppressed = false;
  private disabled = false;
  private lost = false;
  private pendingTrigger = false;
  private serial: Promise<unknown> = Promise.resolve();
  constructor(private run: Transport, private binding: string, private contextBinding?: string) {}

  private gap(detail: string, id = "pi-coverage-gap") { this.add({ kind: "removed", record_id: id, detail }); }
  private add(row: Row) {
    if (this.suppressed || this.disabled) return;
    if (this.records.length >= LIMIT) { this.lost = true; return; }
    for (const key of ["text", "input", "output", "error"]) if (typeof row[key] === "string" && Buffer.byteLength(row[key]) > 65536) { row[key] = Buffer.from(row[key]).subarray(0, 65533).toString("utf8"); row.detail = "truncated Pi field; coverage incomplete"; }
    this.records.push({ ...row, session_id: this.session });
  }

  start(ctx: Context) {
    const abandoned = this.records.length > 0;
    if (abandoned) console.error("AGF Pi session changed with unpersisted capture; previous native JSONL must be replayed with an explicit coverage gap");
    this.session = ctx.sessionManager.getSessionId();
    this.records = []; this.seen.clear(); this.prompts.clear(); this.reads.clear(); this.tools.clear(); this.inputs = []; this.lost = false; this.pendingTrigger = false;
    const header = ctx.sessionManager.getHeader();
    // ParentSession is a file locator, not a native ID. Never infer ancestry by title.
    this.suppressed = Boolean(header?.parentSession) || (ctx.sessionManager.getSessionName() ?? "").includes(marker) || ctx.sessionManager.getEntries().some(e => e.type === "custom" && e.customType === "agf-learning-origin");
    this.add({ kind: "session", title: ctx.sessionManager.getSessionName() ?? "", directory: ctx.cwd });
    if (abandoned) this.gap("previous session capture could not be persisted; no replay completeness claimed");
    const existing = ctx.sessionManager.getEntries();
    for (const entry of existing) this.seen.add(entry.id);
    if (existing.some(e => e.type === "message" || e.type === "compaction")) this.gap("observer attached after native history; historical capture unavailable");
  }

  input(event: Row, ctx: Context) {
    if (!this.ready(ctx)) return;
    if (this.inputs.length >= 128) { this.inputs.shift(); this.gap("native input identity bound exceeded"); }
    this.inputs.push({ text: event.text, source: event.source });
  }

  private ready(ctx: Context): boolean {
    if (this.session !== ctx.sessionManager.getSessionId()) this.start(ctx);
    if (ctx.sessionManager.getHeader()?.parentSession || (ctx.sessionManager.getSessionName() ?? "").includes(marker) || ctx.sessionManager.getEntries().some(e => e.type === "custom" && e.customType === "agf-learning-origin")) { this.suppressed = true; this.records = []; }
    return !this.suppressed && !this.disabled;
  }

  prompt(message: Row, ctx: Context) {
    if (!this.ready(ctx) || message.role !== "user") return;
    if (this.prompts.size >= 128) { this.gap("prompt identity observation overflow"); return; }
    const content = text(message.content), candidates = this.inputs.filter(i => i.text === content);
    // SDK source identifies transport, not a human or permission authority.
    // PiChamber 1.0.5 sends ordinary UI prompts through source=extension.
    const inputSource = candidates.length === 1 && ["interactive", "rpc", "extension"].includes(candidates[0].source) ? candidates[0].source : undefined;
    const index = this.inputs.findIndex(i => i.text === content);
    if (index >= 0) this.inputs.splice(index, 1);
    // Expanded/transformed/image-normalized prompts cannot be safely joined to
    // an input source by prose. Retain them as nonhuman, with explicit loss.
    if (!inputSource) this.gap("known native input source not proven by unique interactive/rpc/extension join");
    this.prompts.set(this.identity(message), { revision: piRevision(ctx.cwd), sha256: digest(content), synthetic: !inputSource, inputSource });
  }
  private identity(message: Row) { return digest(JSON.stringify([message.role, message.timestamp, message.content])); }

  sync(ctx: Context) {
    if (!this.ready(ctx)) return;
    // Native entries, not context projection: compaction never fabricates replacement intent.
    for (const entry of ctx.sessionManager.getEntries()) {
      if (this.seen.has(entry.id)) continue;
      if (this.seen.size >= 10000) { this.gap("native identity bound exceeded"); return; }
      this.seen.add(entry.id);
      if (entry.type === "compaction") { this.add({ kind: "event", event: "session.compacted", record_id: entry.id }); continue; }
      if (entry.type !== "message") continue;
      const m = entry.message;
      if (m.role === "user" || m.role === "assistant") {
        const content = text(m.content), observedPrompt = m.role === "user" ? this.prompts.get(this.identity(m)) : undefined;
        const nonhuman = m.role === "user" && (!observedPrompt || observedPrompt.synthetic);
        this.add({ kind: "message", record_id: entry.id, role: nonhuman ? "context" : m.role, ...(m.role === "user" && !nonhuman ? { input_source: observedPrompt!.inputSource } : {}) });
        if (content) this.add({ kind: "part", record_id: `${entry.id}:text`, message_id: entry.id, part_type: "text", text: content, synthetic: nonhuman });
        if (m.role === "user") {
          const identity = this.identity(m), observed = this.prompts.get(identity);
          this.prompts.delete(identity);
          if (observed && !observed.synthetic && observed.revision && observed.sha256 === digest(content)) this.add({ kind: "source_revision", record_id: entry.id, message_id: entry.id, revision: observed.revision, sha256: observed.sha256 });
          else this.gap("native prompt-start revision unavailable", `${entry.id}:revision-gap`);
        }
        if (m.role === "assistant") for (const part of m.content ?? []) {
          if (part.type === "thinking") this.add({ kind: "part", record_id: `${entry.id}:reasoning`, message_id: entry.id, part_type: "reasoning", reasoning: true });
          if (part.type !== "toolCall") continue;
          if (this.tools.has(part.id)) { this.gap("duplicate native tool call ID", `${part.id}:duplicate`); continue; }
          if (this.tools.size >= 10000) { this.gap("native tool identity bound exceeded"); break; }
          this.tools.set(part.id, { message: entry.id, args: part.arguments, name: part.name });
          this.add({ kind: "part", record_id: part.id, message_id: entry.id, part_type: "tool", tool: part.name, call_id: part.id, tool_status: "pending", input: JSON.stringify(part.arguments) });
        }
      } else if (m.role === "toolResult") {
        const call = this.tools.get(m.toolCallId);
        if (!call) { this.gap("native tool result has no observed assistant call", entry.id); continue; }
        this.toolResult(m.toolCallId, call, m.content, m.isError, ctx);
      }
    }
  }

  private toolResult(id: string, call: ObservedTool, content: any, isError: boolean, ctx: Context) {
    // Output safety is independent of provenance admission. A supplied safe-looking
    // alias is not evidence of its target: require the exact same bounded, nonsecret,
    // non-symlink regular file observed before use AND at each result boundary.
    const after = call.name === "read" ? piSnapshot(call.args, ctx.cwd) : undefined;
    const safe = call.name !== "read" || Boolean(call.readBefore && after && JSON.stringify(call.readBefore) === JSON.stringify(after));
    if (!safe) this.gap("read output withheld: nonsecret at-use target safety unavailable", `${id}:output-safety-gap`);
    this.add({ kind: "part", record_id: id, message_id: call.message, part_type: "tool", call_id: id, tool: call.name, input: JSON.stringify(call.args), tool_status: isError ? "error" : "completed", output: safe ? text(content) : undefined, error: isError ? safe ? text(content) || "native Pi tool error" : "native Pi read error; unsafe or unresolved target output withheld" : undefined });
  }

  toolStart(event: Row, ctx: Context) {
    this.sync(ctx);
    if (!this.ready(ctx)) return;
    if (event.parentToolCallId) { this.gap("nested native tool call has no top-level message identity", event.toolCallId); return; }
    if (event.toolName !== "read") return;
    if (Buffer.byteLength(JSON.stringify(event.args) ?? "") > 8192) { this.gap("native read arguments exceed bracket bound"); return; }
    if (this.reads.has(event.toolCallId)) { this.reads.get(event.toolCallId)!.before = undefined; const call = this.tools.get(event.toolCallId); if (call) call.readBefore = undefined; return; }
    if (this.reads.size >= 128) { this.gap("read observation bound exceeded"); return; }
    const before = piSnapshot(event.args, ctx.cwd), call = this.tools.get(event.toolCallId);
    if (call && call.name === "read" && JSON.stringify(call.args) === JSON.stringify(event.args)) call.readBefore = before;
    this.reads.set(event.toolCallId, { tool: "read", input: JSON.stringify(event.args), runtime: "pi:1.0.0", binding: this.binding, directory: ctx.cwd, before, before_revision: piRevision(ctx.cwd) });
  }

  toolEnd(event: Row, ctx: Context) {
    if (!this.ready(ctx)) return;
    this.sync(ctx);
    const observedCall = this.tools.get(event.toolCallId);
    if (observedCall && observedCall.name === event.toolName) {
      this.toolResult(event.toolCallId, observedCall, event.result?.content, event.isError, ctx);
    } else this.gap("native terminal tool identity unavailable", event.toolCallId);
    if (event.toolName !== "read") return;
    const call = this.tools.get(event.toolCallId), bracket = this.reads.get(event.toolCallId);
    this.reads.delete(event.toolCallId);
    if (!call || !bracket || call.name !== "read" || bracket.input !== JSON.stringify(call.args)) { this.gap("read start/call identity unavailable", event.toolCallId); return; }
    bracket.after = piSnapshot(call.args, ctx.cwd); bracket.after_revision = piRevision(ctx.cwd);
    const id = event.toolCallId; // Pi's native ID, same key as the observed tool part.
    this.add({ kind: "read_bracket", record_id: id, message_id: call.message, call_id: id, read_bracket: bracket });
    const stable = bracket.before && bracket.after && bracket.before.sha256 === bracket.after.sha256 && JSON.stringify(bracket.before.metadata) === JSON.stringify(bracket.after.metadata);
    if (stable && !event.isError) this.add({ kind: "source", path: bracket.after.path, sha256: bracket.after.sha256 });
    else if (!stable) this.add({ kind: "source_gap", path: typeof call.args?.path === "string" ? path.resolve(ctx.cwd, call.args.path) : undefined, detail: "stable bounded read provenance unavailable" });
  }

  flush(ctx: Context, trigger = false): Promise<void> {
    const task = async () => {
      this.sync(ctx);
      if (!this.ready(ctx)) return;
      if (this.lost || (trigger && this.records.length >= LIMIT - 1)) { this.lost = true; this.records = this.records.slice(0, LIMIT - 2); this.gap("Pi buffer overflow; evidence permanently incomplete", "pi-buffer-overflow"); }
      if (trigger) this.add({ kind: "event", event: "session.idle", detail: "Pi 1.0.0 agent_settled; not task success" });
      this.pendingTrigger ||= trigger;
      const records = this.records.slice();
      if (!records.length) return;
      const envelope: PiEnvelope = { version: 1, harness: "pi", sdk_version: "1.0.0", session_id: this.session, project_dir: ctx.cwd, trigger: this.pendingTrigger, records };
      try {
        const result = await this.run(["retrospective", "ingest", "--harness", "pi"], envelope);
        if (result.code === 2) { this.disabled = true; this.records = []; }
        else if (result.code === 0) {
          this.records.splice(0, records.length); this.pendingTrigger = false;
          try { if (JSON.parse(result.stdout).suppressed === true) { this.suppressed = true; this.records = []; } } catch { /* no transcript/output interpretation */ }
        }
        else console.error("AGF Pi ingest failed; batch retained, native JSONL remains authoritative");
      } catch { console.error("AGF Pi ingest unavailable; batch retained, native JSONL remains authoritative"); }
    };
    const result = this.serial.then(task, task); this.serial = result; return result;
  }

  async context(event: Row, ctx: Context): Promise<Row | undefined> {
    this.sync(ctx);
    if (!this.contextBinding || !this.ready(ctx)) return;
    const request = { session_id: this.session, project: ctx.cwd, sessions: { [this.session]: { title: ctx.sessionManager.getSessionName() ?? "", directory: ctx.cwd } } };
    try {
      const args = ["retrospective", "lessons", "context", "--binding", this.contextBinding];
      const result = await this.run(args, request);
      if (result.code !== 0) return;
      const payload = JSON.parse(result.stdout);
      if (typeof payload.block !== "string" || Buffer.byteLength(payload.block) > 8192 || digest(payload.block) !== payload.sha256 || !payload.block) return;
      // Request-local system message only: no durable custom_message/user insertion.
      const messages = [...event.messages];
      const index = messages.findIndex((m: Row) => m.role === "system");
      if (index < 0 || typeof messages[index].content !== "string") return;
      messages[index] = { ...messages[index], content: `${messages[index].content}\n\n${payload.block}` };
      const original = event.messages;
      event.messages = messages;
      try {
        const ack = await this.run([...args, "--ack", payload.sha256], request);
        if (ack.code !== 0 || JSON.parse(ack.stdout).sha256 !== payload.sha256) { event.messages = original; return; }
      } catch (error) { event.messages = original; throw error; }
      return { messages };
    } catch { console.error("AGF Pi advisory context refused; ordinary request unchanged"); }
  }
}

export function registerPiObserver(pi: API, observer: PiObserver) {
  pi.on("session_start", async (_event, ctx) => { observer.start(ctx); await observer.flush(ctx); });
  pi.on("input", (event, ctx) => observer.input(event, ctx));
  pi.on("message_start", (event, ctx) => observer.prompt(event.message, ctx));
  pi.on("tool_execution_start", async (event, ctx) => { observer.toolStart(event, ctx); await observer.flush(ctx); });
  pi.on("tool_execution_end", async (event, ctx) => { observer.toolEnd(event, ctx); await observer.flush(ctx); });
  pi.on("turn_end", async (_event, ctx) => observer.flush(ctx));
  pi.on("context_with_system", async (event, ctx) => { await observer.flush(ctx); return observer.context(event, ctx); });
  // agent_end is deliberately not an idle or completion signal.
  pi.on("agent_settled", async (_event, ctx) => observer.flush(ctx, true));
  pi.on("session_shutdown", async (_event, ctx) => observer.flush(ctx));
}

export default async function retrospective(pi: API) {
  const cli = process.env.AGF_BIN, fingerprint = cli && piBinaryFingerprint(cli);
  if (!cli || !fingerprint) { console.error("AGF Pi requires explicit owned executable AGF_BIN; capture disabled"); return; }
  // Pi's native loader aliases this module to its own SDK, including PiChamber's
  // bundled SDK. Do not resolve the unrelated global CLI or trust an env version.
  const packageName = "@earendil-works/pi-coding-agent";
  const sdk = await import(packageName);
  if (sdk.VERSION !== "1.0.0") { console.error("AGF Pi SDK not allowlisted; capture disabled"); return; }
  const run = piTransport(cli, fingerprint);
  const probe = await run(["retrospective", "ingest", "--harness", "pi", "--help"], {});
  if (probe.code !== 1 || probe.stdout !== "" || probe.stderr?.trim() !== "agent-fabric: flag: help requested") { console.error("AGF Pi ingest capability unavailable; capture disabled"); return; }
  const binding = digest(JSON.stringify(["read-bracket:v1", "pi:1.0.0", process.version, process.execPath, process.getuid?.(), fingerprint]));
  registerPiObserver(pi, new PiObserver(run, binding, process.env.AGF_LESSONS_CONTEXT));
}
