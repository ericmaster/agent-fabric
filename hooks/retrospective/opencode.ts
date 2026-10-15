// Spec: docs/specs/task-retrospective.md — OpenCode observation adapter.
//
// THIN ADAPTER CONTRACT: this file is the only observation surface of the
// task-retrospective loop on OpenCode. It converts native session/message/
// tool/session-level events into a versioned observation envelope and hands
// the envelope JSON to the packaged Go CLI (`agf retrospective ingest
// --harness opencode`) over STDIN. Every business rule — success gating,
// evaluation, dispatch, ancestor suppression chains, deduplication and
// trigger policy — lives in Go, per docs/specs/task-retrospective.md. The
// adapter never interpolates transcript data into shell arguments (the
// envelope goes through stdin; argv carries static flags only), never posts
// into the user's source session, and never blocks a response on Jev.
//
// Version pinning: fixture-tested against OpenCode 2.0.24, 2.0.27 and 2.0.30
// (@opencode/plugin@2.0.24 default setup/events API). The v1 named hook export
// and @opencode-ai/plugin@1.18.35 types are replay-only compatibility. Native
// setup uses ctx.app.version and rejects an unconfirmed generation.
//
// Runtime constraints: the installed plugin file is self-contained; runtime
// imports are Node/Bun built-ins only, so no per-project package.json is
// required. Type-only imports are erased at load time.
import type { Hooks, PluginInput } from "@opencode-ai/plugin";
import type { Plugin as NativePlugin } from "@opencode/plugin";
import { spawn } from "node:child_process";
import { createHash, randomUUID } from "node:crypto";
import { readFileSync, statSync, realpathSync, mkdirSync, writeFileSync } from "node:fs";
import path from "node:path";

const ENVELOPE_VERSION = 1;
const FLUSH_THRESHOLD_RECORDS = 25;
const MAX_BUFFERED_EVENTS = 2000;
const SPAWN_TIMEOUT_MS = 30_000;
const HEALTH_TIMEOUT_MS = 5_000;
const SUPPORTED_NATIVE_VERSIONS = ["2.0.24", "2.0.27", "2.0.30"];
const LEARNING_ORIGIN_PREFIX = "agf-learning-origin:v1:";

/** Ingest exit codes; the adapter reacts without parsing any output. */
const EXIT_OK = 0;
const EXIT_DISABLED = 2;

interface EnvelopeRecord {
  kind: "session" | "message" | "part" | "removed" | "event" | "source" | "source_gap";
  session_id?: string;
  record_id?: string;
  parent_id?: string;
  message_id?: string;
  call_id?: string;
  role?: string;
  part_type?: string;
  title?: string;
  directory?: string;
  text?: string;
  synthetic?: boolean;
  reasoning?: boolean;
  tool?: string;
  tool_status?: string;
  input?: string;
  output?: string;
  error?: string;
  event?: string;
  status?: string;
  detail?: string;
  path?: string;
  sha256?: string;
}

export interface Envelope {
  version: number;
  harness: "opencode";
  session_id: string;
  server_url?: string;
  sdk_version?: string;
  trigger?: boolean;
  project_dir?: string;
  records: EnvelopeRecord[];
}

function clamp(value: unknown, max = 64 * 1024): string | undefined {
  if (typeof value !== "string" || value.length === 0) {
    return undefined;
  }
  return value.length > max ? value.slice(0, max) : value;
}

/** Bounded record field cap; the Go side records the truncation as a gap. */
const RECORD_FIELD_CAP = 64 * 1024;

/**
 * boundedField sets one record field clipped to the cap and marks the loss
 * in `detail` so the staged spool never sells a truncation as full coverage.
 */
function boundedField(record: EnvelopeRecord, field: "text" | "input" | "output" | "title" | "directory", value: string | undefined): void {
  if (typeof value !== "string" || value.length === 0) {
    return;
  }
  record[field] = value.length > RECORD_FIELD_CAP ? value.slice(0, RECORD_FIELD_CAP) : value;
  if (value.length > RECORD_FIELD_CAP) {
    record.detail = `truncated ${field} observed at ${value.length} bytes (field cap ${RECORD_FIELD_CAP})`;
  }
}

/** Bounded error identity (type names / short messages; never stacks). */
function errorName(value: unknown): string | undefined {
  if (typeof value === "string" && value.length > 0) {
    return value.length > 512 ? value.slice(0, 512) : value;
  }
  if (value && typeof value === "object") {
    const candidate = value as { name?: unknown; message?: unknown };
    if (typeof candidate.name === "string" && candidate.name.length > 0) {
      return candidate.name.length > 512 ? candidate.name.slice(0, 512) : candidate.name;
    }
    if (typeof candidate.message === "string" && candidate.message.length > 0) {
      return candidate.message.length > 512 ? candidate.message.slice(0, 512) : candidate.message;
    }
  }
  return undefined;
}

/** Tool-state terminal mapping (pending stays an unresolved candidate). */
function toolStatus(state: unknown): string | undefined {
  const status = (state as { status?: unknown } | undefined)?.status;
  if (status === "completed" || status === "error") {
    return status;
  }
  if (status === "pending" || status === "running" || status == null) {
    return "pending";
  }
  return undefined;
}

/** The observed absolute path of a source-loading tool call (if any). */
function observedPath(args: unknown, directory?: string): string | undefined {
  if (!args || typeof args !== "object") {
    return undefined;
  }
  const values = args as { filePath?: unknown; path?: unknown; file?: unknown };
  const candidate = values.filePath ?? values.path ?? values.file;
  if (typeof candidate === "string" && candidate.length > 0) {
    if (path.isAbsolute(candidate)) return candidate;
    if (directory) return path.resolve(directory, candidate);
  }
  return undefined;
}

/** Secret paths are neither read nor hashed for provenance. */
function secretSourcePath(file: string): boolean {
  return /(^|[\\/])(\.env|credentials|secrets|id_rsa|id_ed25519)/i.test(file) || /\.(env|pem|p12|pfx)$/i.test(path.basename(file));
}

/** Legacy replay resolver only; native setup never falls back. */
function resolveCLI(env: NodeJS.ProcessEnv): string | undefined {
  const fromEnv = env.AGF_BIN;
  if (typeof fromEnv === "string" && fromEnv.length > 0) {
    return fromEnv;
  }
  const home = env.HOME ?? env.USERPROFILE;
  const roots: string[] = [
    process.cwd(),
    typeof home === "string" ? path.join(home, ".opencode", "bin") : "",
    typeof home === "string" ? path.join(home, ".local", "bin") : "",
  ].filter((candidate) => candidate.length > 0);
  for (const root of roots) {
    for (const name of ["agent-fabric", "agf", "agent-fabric.exe", "agf.exe"]) {
      try {
        if (statSync(path.join(root, name)).isFile()) {
          return path.join(root, name);
        }
      } catch {
        // missing candidate, keep looking
      }
    }
  }
  return undefined;
}

/** RunIngest abstracts the CLI spawn for recorded replay tests. */
type RunIngest = (envelope: Envelope, cli: string, metadata?: Record<string, unknown>) => Promise<number | null>;

/** Native release stability fingerprint; no symlink into a mutable checkout. */
function bindingFingerprint(cli: string): string {
  if (!path.isAbsolute(cli) || realpathSync(cli) !== path.resolve(cli) || !process.getuid) throw new Error("unsafe binding");
  const file = statSync(cli), parent = statSync(path.dirname(cli)), uid = process.getuid();
  if (!file.isFile() || file.uid !== uid || parent.uid !== uid || (file.mode & 0o022) || (parent.mode & 0o022) || !(file.mode & 0o100)) throw new Error("unsafe binding");
  return `${file.dev}:${file.ino}:${file.mode}:${parent.dev}:${parent.ino}:${parent.mode}:${createHash("sha256").update(readFileSync(cli)).digest("hex")}`;
}

/** Safe declared metadata only, never inferred producer attribution. */
function observerMetadata(cli?: string, childPID?: number): Record<string, unknown> {
  let declaredObserverOrigin: string | undefined;
  try {
    const url = new URL(process.env.AGF_OBSERVER_SERVER ?? "");
    if (["http:", "https:"].includes(url.protocol) && !url.username && !url.password && !url.search && !url.hash && url.pathname === "/") declaredObserverOrigin = url.origin;
  } catch { /* no clean declared origin */ }
  return { observerPID: process.pid, timestamp: new Date().toISOString(), resolvedCLI: cli && cli.length <= 4096 ? cli : undefined, resolutionSource: "explicit_AGF_BIN", childPID, declaredObserverOrigin };
}

/** Nonmutating Go ingest parser capability probe, bounded even on hostile output. */
async function probeBinding(cli: string): Promise<{ compatible: boolean; childPID?: number }> {
  return new Promise((resolve) => {
    let child: ReturnType<typeof spawn>;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let settled = false, stdout = "", stderr = "";
    const finish = (compatible: boolean) => {
      if (settled) return;
      settled = true; if (timer) clearTimeout(timer);
      resolve({ compatible, childPID: child?.pid });
    };
    try { child = spawn(cli, ["retrospective", "ingest", "--harness", "opencode", "--help"], { stdio: ["ignore", "pipe", "pipe"] }); }
    catch { finish(false); return; }
    const collect = (kind: "stdout" | "stderr", chunk: Buffer) => {
      if (settled) return;
      if (kind === "stdout") stdout += chunk.toString(); else stderr += chunk.toString();
      if (stdout.length + stderr.length > 4096) { child.kill("SIGKILL"); finish(false); }
    };
    child.stdout?.on("data", (chunk) => collect("stdout", chunk));
    child.stderr?.on("data", (chunk) => collect("stderr", chunk));
    child.on("error", () => finish(false));
    child.on("close", (code) => finish(code === 1 && stdout === "" && stderr.trim() === "agent-fabric: flag: help requested"));
    timer = setTimeout(() => { child.kill("SIGKILL"); finish(false); }, HEALTH_TIMEOUT_MS);
  });
}

/** spawnIngest pipes the envelope through stdin; argv stays static. */
const spawnIngest: RunIngest = (envelope, cli, metadata) =>
  new Promise<number>((resolve) => {
    let settled = false;
    let stderr = "";
    let timer: ReturnType<typeof setTimeout> | undefined;
    const finish = (code: number) => {
      if (settled) return;
      settled = true;
      if (timer) clearTimeout(timer);
      if (code !== EXIT_OK && code !== EXIT_DISABLED) reportIngestFailure(code, stderr, undefined, metadata ? { ...metadata, childPID: child?.pid } : undefined);
      resolve(code);
    };
    let child: ReturnType<typeof spawn>;
    try {
      child = spawn(cli, ["retrospective", "ingest", "--harness", "opencode"], {
        stdio: ["pipe", "ignore", "pipe"],
        env: process.env,
      });
    } catch {
      stderr = "ingest spawn threw synchronously";
      finish(1);
      return;
    }
    child.stderr?.on("data", (chunk) => { stderr += chunk.toString(); });
    timer = setTimeout(() => {
      child.kill("SIGKILL");
      finish(1);
    }, SPAWN_TIMEOUT_MS);
    child.on("close", (code) => {
      finish(typeof code === "number" ? code : 1);
    });
    child.on("error", () => {
      finish(1);
    });
    const stdin = child.stdin;
    if (!stdin) {
      finish(1);
      return;
    }
    stdin.on("error", () => {});
    stdin.end(JSON.stringify(envelope));
  });

/** Full private diagnostic, not a transcript/environment dump. Never on disabled. */
function reportIngestFailure(code: number, stderr: string, detail = "Ingest failed; original batch retained for bounded idempotent retry. No stdin or environment retained.", metadata?: Record<string, unknown>): void {
  for (const name of ["FASTINO_API_KEY", "TYPESAFE_API_KEY", "OPENCODE_PASSWORD", "OPENCODE_SERVER_PASSWORD"]) {
    const value = process.env[name]; if (value) stderr = stderr.split(value).join("[REDACTED:credential]");
  }
  stderr = stderr.replace(/(Bearer\s+|Basic\s+)\S+/gi, "$1[REDACTED]").replace(/((?:token|password|api[_-]?key)\s*[=:]\s*)\S+/gi, "$1[REDACTED]");
  const home = process.env.HOME ?? process.env.USERPROFILE;
  const state = process.env.AGF_RETROSPECTIVE_STATE ?? (process.env.XDG_STATE_HOME ? path.join(process.env.XDG_STATE_HOME, "agent-fabric/retrospective") : home ? path.join(home, ".local/state/agent-fabric/retrospective") : undefined);
  if (!state) { console.error("AGF retrospective: ingest failed; no private diagnostic root available"); return; }
  try {
    const directory = path.join(state, "observer-errors"); mkdirSync(directory, { recursive: true, mode: 0o700 });
    const report = path.join(directory, `${randomUUID()}.json`);
    writeFileSync(report, JSON.stringify({ code, stderr, detail, ...metadata }), { mode: 0o600 });
    console.error(`AGF retrospective: observer failed (exit ${code}); full diagnostic: ${report}`);
  } catch { console.error("AGF retrospective: ingest failed; private diagnostic write failed (batch retained)"); }
}

/** basicAuthHeader builds the server's own HTTP Basic credentials from the
 * process env the plugin inherits (it runs inside the server). Returns
 * undefined when the server runs without an explicit password. */
function basicAuthHeader(env: NodeJS.ProcessEnv): string | undefined {
  const password = env.OPENCODE_PASSWORD || env.OPENCODE_SERVER_PASSWORD;
  if (!password) return undefined;
  const username = env.OPENCODE_SERVER_USERNAME || "opencode";
  return `Basic ${Buffer.from(`${username}:${password}`).toString("base64")}`;
}

/** healthProbe pins the installed server generation (GET /api/info on the
 * live 2.x server; Basic auth from the process env). */
async function healthProbe(base: URL): Promise<{ version?: string } | null> {
  const target = new URL("/api/info", base);
  const headers: Record<string, string> = {};
  const authorization = basicAuthHeader(process.env);
  if (authorization) headers.Authorization = authorization;
  const response = await fetch(target, {
    headers,
    signal: AbortSignal.timeout(HEALTH_TIMEOUT_MS),
  });
  if (!response.ok) {
    return null;
  }
  return (await response.json()) as { version?: string };
}

/**
 * Adapter is the per-server-instance observation state: bounded buffers per
 * session, the version gate, and the flush handoff to the packaged CLI.
 */
class Adapter {
  private buffers = new Map<string, EnvelopeRecord[]>();
  private mode: "active" | "inert" = "inert";
  private readonly cli: string | undefined;
  private readonly serverUrl: URL;
  private readonly run: RunIngest;
  private readonly health: () => Promise<{ version?: string } | null>;
  readonly projectDir: string;
  observedSDK: string | undefined;
  /** Last known identity of every observed session (title + parent). */
  private titles = new Map<string, string>();
  private parents = new Map<string, string>();
  private identities = new Map<string, EnvelopeRecord>();
  private flights = new Map<string, Promise<number | null>>();
  private terminalPending = new Set<string>();
  /** Session ids carrying the trusted learning-origin marker title. */
  private suppressed = new Set<string>();

  constructor(input: {
    serverUrl?: URL | string;
    projectDir?: string;
    run?: RunIngest;
    health?: () => Promise<{ version?: string } | null>;
    cli?: string;
    sdkVersion?: string;
  }) {
    this.serverUrl =
      input.serverUrl instanceof URL
        ? input.serverUrl
        : typeof input.serverUrl === "string"
          ? new URL(input.serverUrl)
          : new URL("http://127.0.0.1:0");
    this.projectDir = typeof input.projectDir === "string" ? input.projectDir : "";
    this.run = input.run ?? spawnIngest;
    this.health = input.health ?? (() => healthProbe(this.serverUrl));
    this.cli = input.cli ?? resolveCLI(process.env);
    this.observedSDK = input.sdkVersion;
  }

  /**
   * ensureActive probes the installed server generation once before the
   * first flush. AGF_PLUGIN_FAKE_HEALTH substitutes the health body for
   * recorded replay tests; a connection error or an untested major version
   * disables staging permanently for this instance (degradation, never a
   * partial fake trace).
   */
  async ensureActive(): Promise<boolean> {
    if (this.mode === "active") {
      return true;
    }
    try {
      const stub = process.env.AGF_PLUGIN_FAKE_HEALTH;
      const health = stub ? (JSON.parse(stub) as { version?: string }) : await this.health();
      const version = health?.version ?? "";
       if (SUPPORTED_NATIVE_VERSIONS.includes(version) && this.cli) {
        this.mode = "active";
        this.observedSDK = this.observedSDK ?? version;
      }
    } catch {
      this.mode = "inert";
    }
    return this.mode === "active";
  }

  /** bufferedCount returns the unflushed record count of one session. */
  bufferedCount(sessionID: string): number {
    return this.buffers.get(sessionID)?.length ?? 0;
  }

  /** isStaging reports whether the version gate passed for this instance. */
  isStaging(): boolean {
    return this.mode === "active";
  }

  /** observe stages one mapped record into the owner session buffer. */
  observe(sessionID: string, record: EnvelopeRecord): void {
    if (this.mode !== "active" || !sessionID) {
      return;
    }
    // Learning-origin pre-filter: the dispatch origins store stays
    // authoritative (the store is what the drain checks); this prefilter is
    // the immediate reciprocal staging cut for marker titles and their
    // observed descendants.
    if (this.suppressedChain(sessionID)) {
      this.buffers.delete(sessionID);
      return;
    }
    let buffer = this.buffers.get(sessionID);
    if (!buffer) {
      buffer = [];
      this.buffers.set(sessionID, buffer);
    }
    if (buffer.length >= MAX_BUFFERED_EVENTS) {
      buffer.shift();
      buffer.push({
        kind: "removed",
        session_id: sessionID,
        record_id: "plugin-buffer-overflow",
        detail: "oldest unflushed observation dropped by the plugin bound",
      });
      return;
    }
    buffer.push(record);
  }

  /** suppressedChain resolves the observed parent chain against markers. */
  private suppressedChain(sessionID: string): boolean {
    if (this.suppressed.has(sessionID)) {
      return true;
    }
    let current = this.parents.get(sessionID);
    let depth = 0;
    while (current && depth < 32) {
      if (this.suppressed.has(current)) {
        return true;
      }
      current = this.parents.get(current);
      depth++;
    }
    return false;
  }

  /**
   * observeSessionRow relays one session identity row to every active
   * session buffer on this server: parent chains, child aggregation, and
   * origin titles need the whole observed identity map in each spool. A
   * marker title suppresses that session and every observed descendant.
   */
  observeSessionRow(record: EnvelopeRecord): void {
    const id = record.session_id ?? "";
    const marker = Boolean(id && (record.title ?? "").includes(LEARNING_ORIGIN_PREFIX));
    if (id && marker) {
      this.suppressed.add(id);
    }
    if (id) {
      this.identities.set(id, { ...this.identities.get(id), ...record, directory: record.directory || this.identities.get(id)?.directory });
      this.titles.set(id, record.title ?? "");
      if (record.parent_id) {
        this.parents.set(id, record.parent_id);
      }
    }
    // Relay the identity row to every active buffer first, then drop every
    // buffered session (and the marker row's own buffer) whose observed
    // chain reaches a marker title.
    for (const sessionID of Array.from(this.buffers.keys())) {
      this.observe(sessionID, record);
    }
    this.observe(id, record);
    if (marker) {
      for (const sessionID of Array.from(this.buffers.keys())) {
        if (this.suppressedChain(sessionID)) {
          this.buffers.delete(sessionID);
        }
      }
      this.buffers.delete(id);
    }
  }

  /**
   * flush hands one session's staged records to ingest; `trigger` marks the
   * terminal candidate (idle). Returns the ingest exit code (null when the
   * flush was a no-op). A disabled verdict stops staging until the next
   * terminal trigger re-probes.
   */
  async flush(sessionID: string, trigger: boolean): Promise<number | null> {
    if (trigger) this.terminalPending.add(sessionID);
    const previous = this.flights.get(sessionID) ?? Promise.resolve(null);
    const flight = previous.then(async () => {
      let code = await this.flushOnce(sessionID);
      // Stable record IDs and jobs make one terminal retry idempotent.
      if (this.terminalPending.has(sessionID) && code !== EXIT_OK && code !== EXIT_DISABLED && code !== null) code = await this.flushOnce(sessionID);
      return code;
    });
    this.flights.set(sessionID, flight);
    try { return await flight; } finally { if (this.flights.get(sessionID) === flight) this.flights.delete(sessionID); }
  }

  private async flushOnce(sessionID: string): Promise<number | null> {
    const trigger = this.terminalPending.has(sessionID);
    if (!(await this.ensureActive())) {
      return null;
    }
    const buffer = this.buffers.get(sessionID);
    if (!buffer || buffer.length === 0) {
      return trigger ? EXIT_OK : null;
    }
    const identities = Array.from(this.identities.values());
    const identityRecords = identities.length > 128 ? [this.identities.get(sessionID), ...identities.slice(-127)].filter((row): row is EnvelopeRecord => Boolean(row)) : identities;
    if (identities.length > 128) identityRecords.push({ kind: "removed", session_id: sessionID, record_id: "plugin-identity-overflow", detail: "identity map exceeds envelope bound; parent/child coverage incomplete" });
    const envelope: Envelope = {
      version: ENVELOPE_VERSION,
      harness: "opencode",
      session_id: sessionID,
      server_url: this.serverUrl.port === "0" ? undefined : this.serverUrl.toString().replace(/\/$/, ""),
      sdk_version: this.observedSDK,
      trigger,
      project_dir: this.projectDir || undefined,
      records: [...identityRecords, ...buffer],
    };
    this.buffers.set(sessionID, []);
    let code: number | null;
    try { code = await this.run(envelope, this.cli ?? ""); } catch { code = 1; }
    if (code === EXIT_DISABLED) {
      this.buffers.set(sessionID, []);
      this.mode = "inert";
      this.terminalPending.delete(sessionID);
    } else if (code !== EXIT_OK) {
      const current = this.buffers.get(sessionID) ?? [];
      const retained = [...buffer, ...current];
      if (retained.length > MAX_BUFFERED_EVENTS) {
        retained.splice(0, retained.length - MAX_BUFFERED_EVENTS + 1);
        retained.unshift({ kind: "removed", session_id: sessionID, record_id: "plugin-buffer-overflow", detail: "failed ingest retention exceeded bound; evidence incomplete" });
      }
      this.buffers.set(sessionID, retained);
    } else if (trigger) {
      this.terminalPending.delete(sessionID);
    }
    return code;
  }
}

/**
 * RetrospectivePlugin is the single plugin export. The installed runtime
 * loads it like any local plugin; observation stays passive until both the
 * version gate passes and ingest reports the global enablement verdict.
 */
export const RetrospectivePlugin: (input: PluginInput) => Promise<Hooks> = async (input) => {
  const adapter = new Adapter({
    serverUrl: input.serverUrl,
    projectDir: typeof input.directory === "string" ? input.directory : "",
  });
  // The version gate precedes the first event: the adapter stays inert (no
  // staging) for an unconfirmed generation instead of guessing shapes.
  await adapter.ensureActive();
  return {
    event: async ({ event }: { event: any }) => {
      const record = eventRecord(event);
      if (!record) {
        return;
      }
      const sessionID = record.session_id;
      if (!sessionID) {
        return;
      }
      if (record.kind === "session") {
        adapter.observeSessionRow(record);
      } else {
        adapter.observe(sessionID, record);
      }
      const isIdle = record.kind === "event" && record.event === "session.idle";
      if (isIdle || adapter.bufferedCount(sessionID) >= FLUSH_THRESHOLD_RECORDS) {
        await adapter.flush(sessionID, isIdle);
      }
    },
    "tool.execute.after": async (toolInput: any, output: any) => {
      // Source-load provenance: hash the exact bytes the read observed at
      // use time. Never hashes secret-file candidates; runs in the same
      // process microseconds after the tool returned, and the dispatch
      // layer re-verifies the on-disk hash immediately before any write.
      const sessionID = typeof toolInput?.sessionID === "string" ? toolInput.sessionID : "";
      if (!sessionID || !adapter.isStaging()) {
        return;
      }
      const observed = observedPath(toolInput?.args);
      if (!observed) {
        return;
      }
      const base = path.basename(observed);
      const secretCandidate = /\.(env|pem|p12|pfx)$/i.test(base) || /(^|\/)(id_rsa|id_ed25519|credentials|secrets)(\/|$)/i.test(observed);
      if (secretCandidate) {
        adapter.observe(sessionID, {
          kind: "source_gap",
          session_id: sessionID,
          path: observed,
          detail: "secret-file candidate: content never collected",
        });
        return;
      }
      try {
        const stat = statSync(observed);
        if (!stat.isFile()) {
          return;
        }
        adapter.observe(sessionID, {
          kind: "source",
          session_id: sessionID,
          path: observed,
          sha256: createHash("sha256").update(readFileSync(observed)).digest("hex"),
        });
      } catch {
        adapter.observe(sessionID, {
          kind: "source_gap",
          session_id: sessionID,
          path: observed,
          detail: "observed read could not be hashed at use time",
        });
      }
    },
  };
};

/** Explicitly verified v2 native definition. Named v1 export above is replay-only. */
export default {
  id: "agf-retrospective",
  async setup(ctx: NativePlugin.Context) {
    const version = ctx.app.version;
    const supported = SUPPORTED_NATIVE_VERSIONS.includes(version);
    const capabilities = typeof ctx.session?.hook === "function" && typeof ctx.session?.get === "function" && typeof ctx.tool?.hook === "function" && typeof ctx.event?.subscribe === "function";
    if (!supported || !capabilities) {
      const message = `AGF retrospective: ${supported ? "missing native capabilities" : "unsupported native plugin generation"}; require OpenCode 2.0.24, 2.0.27 or 2.0.30 with session prompt/get, tool execute.after and event subscribe. See docs/runbooks/task-retrospective.md (Installed version pins in backing spec).`;
      reportIngestFailure(1, message, "Native setup refused before staging; use a verified runtime and required capabilities. No transcript or environment retained.");
      throw new Error(message);
    }
    // Binding refusal is per setup instance, before any capture/subscription.
    if (process.env.AGF_PLUGIN_FAKE_HEALTH) throw new Error("AGF retrospective: replay health stub forbidden in native setup");
    const cli = process.env.AGF_BIN;
    let fingerprint: string;
    const refuse = (reason: string, childPID?: number) => reportIngestFailure(1, reason, "Native binding refused; no further capture/ingest. Set explicit owned compatible AGF_BIN and reload. No probe output, transcript or environment retained.", observerMetadata(fingerprint ? cli : undefined, childPID));
    try { if (!cli) throw new Error("missing binding"); fingerprint = bindingFingerprint(cli); }
    catch { refuse("missing_or_unsafe_explicit_binding"); return async () => {}; }
    const probe = await probeBinding(cli);
    if (!probe.compatible) { refuse("incompatible_or_failed_ingest_help_probe", probe.childPID); return async () => {}; }
    try { if (bindingFingerprint(cli) !== fingerprint) throw new Error("changed binding"); }
    catch { refuse("explicit_binding_changed", probe.childPID); return async () => {}; }
    let bindingRefused = false;
    const run: RunIngest = async (envelope) => {
      if (bindingRefused) return EXIT_DISABLED;
      try { if (bindingFingerprint(cli) !== fingerprint) throw new Error("changed binding"); }
      catch { bindingRefused = true; refuse("explicit_binding_changed"); return EXIT_DISABLED; }
      return spawnIngest(envelope, cli, observerMetadata(cli));
    };
    const adapter = new Adapter({
      serverUrl: process.env.AGF_OBSERVER_SERVER,
      projectDir: ctx.location.directory,
      sdkVersion: version,
      health: async () => ({ version }),
      cli,
      run,
    });
    // Do not permit replay-only environment stubs to grant live authority.
    await adapter.ensureActive();
    let disposed = false;
    const users = new Map<string, string>();
    const stepOwners = new Map<string, string>();
	// Location plugins may be loaded after session.created on the fresh first
	// task. Prompt hooks run before generation: obtain current identity from the
	// in-process native domain, and capture the actual admitted user message.
	const intake = await ctx.session.hook("prompt", async (event) => {
	  if (disposed || bindingRefused || !adapter.isStaging()) return;
	  if (users.has(event.sessionID) || users.size < MAX_BUFFERED_EVENTS) users.set(event.sessionID, event.messageID);
	  try {
	    const session = await ctx.session.get({ sessionID: event.sessionID }, { signal: AbortSignal.timeout(2000) });
	    adapter.observeSessionRow({ kind: "session", session_id: session.id, parent_id: session.parentID, title: session.title, directory: session.location.directory });
	  } catch {
	    adapter.observe(event.sessionID, { kind: "removed", session_id: event.sessionID, record_id: "native-intake-identity-gap", detail: "native prompt identity unavailable; startup coverage incomplete" });
	  }
	  adapter.observe(event.sessionID, { kind: "message", session_id: event.sessionID, record_id: event.messageID, role: "user" });
	  const part: EnvelopeRecord = { kind: "part", session_id: event.sessionID, record_id: `${event.messageID}:text`, message_id: event.messageID, part_type: "text" };
	  boundedField(part, "text", event.prompt.text); adapter.observe(event.sessionID, part);
	});
	const provenance = await ctx.tool.hook("execute.after", async (event) => {
	  if (disposed || bindingRefused) return;
	  if (event.tool !== "read" || event.status !== "completed") return;
	  const file = observedPath(event.input, ctx.location.directory);
	  if (!file) return;
	  if (secretSourcePath(file)) {
	    adapter.observe(event.sessionID, { kind: "source_gap", session_id: event.sessionID, path: file, detail: "secret-file candidate: body never collected" }); return;
	  }
	  try { adapter.observe(event.sessionID, { kind: "source", session_id: event.sessionID, path: file, sha256: createHash("sha256").update(readFileSync(file)).digest("hex") }); }
	  catch { adapter.observe(event.sessionID, { kind: "source_gap", session_id: event.sessionID, path: file, detail: "native at-use read provenance unavailable" }); }
	});
    const abort = new AbortController();
    const tools = new Map<string, { name?: string; input?: string }>();
    const consume = (async () => {
      for await (const event of ctx.event.subscribe({ signal: abort.signal })) {
        if (bindingRefused) continue;
        const records = nativeRecords(event, tools, users, stepOwners);
        for (const record of records) {
          const id = record.session_id;
          if (!id) continue;
          if (record.kind === "session") adapter.observeSessionRow(record);
          else adapter.observe(id, record);
          const terminal = record.kind === "event" && record.event === "session.idle";
          if (terminal || adapter.bufferedCount(id) >= FLUSH_THRESHOLD_RECORDS) await adapter.flush(id, terminal);
        }
      }
    })().catch(() => { if (!abort.signal.aborted) console.error("AGF retrospective: native event subscription failed; collection incomplete"); });
    return async () => { abort.abort(); await intake.dispose(); await provenance.dispose(); await consume; disposed = true; };
  },
};

/** Normalize actual v2 durable/ephemeral events; never inspect reasoning text. */
function nativeRecords(event: any, tools: Map<string, { name?: string; input?: string }>, users: Map<string, string>, stepOwners: Map<string, string>): EnvelopeRecord[] {
  const d = event?.data ?? {};
  const session = d.sessionID;
  if (typeof session !== "string") return [];
  // v2 provider call IDs (e.g. functions.read:0) repeat in later messages.
  const call = `${d.assistantMessageID}:${d.id}`;
  const message = (id: string, role: string): EnvelopeRecord => ({ kind: "message", session_id: session, record_id: id, role });
  const part = (id: string, messageID: string, text: string): EnvelopeRecord => {
    const record: EnvelopeRecord = { kind: "part", session_id: session, record_id: id, message_id: messageID, part_type: "text" };
    boundedField(record, "text", text); return record;
  };
  switch (event.type) {
    case "session.created": return [{ kind: "session", session_id: session, title: d.title ?? "", parent_id: d.parentID ?? "", directory: d.location?.directory ?? "" }];
    case "session.renamed": return [{ kind: "session", session_id: session, title: d.title ?? "" }];
    case "session.inbox.enqueued":
      if (d.item?.type !== "user" || typeof d.item.payload?.text !== "string") return [];
      return [message(d.inboxID, "user"), part(`${d.inboxID}:text`, d.inboxID, d.item.payload.text)];
    case "session.step.started": {
      const key = `${session}:${d.assistantMessageID}`;
      // Preserve even absence at first observation: queued inbox events and
      // duplicate old steps cannot borrow a later admitted prompt's identity.
      if (!stepOwners.has(key) && stepOwners.size < MAX_BUFFERED_EVENTS) stepOwners.set(key, users.get(session) ?? "");
      return [{ ...message(d.assistantMessageID, "assistant"), parent_id: stepOwners.get(key) || undefined }];
    }
    case "session.text.ended": return [message(d.assistantMessageID, "assistant"), part(`${d.assistantMessageID}:text:${d.ordinal}`, d.assistantMessageID, d.text)];
    case "session.reasoning.ended": return [{ kind: "part", session_id: session, record_id: `${d.assistantMessageID}:reasoning:${d.ordinal}`, message_id: d.assistantMessageID, part_type: "reasoning", reasoning: true }];
    case "session.tool.input.started": tools.set(`${session}:${call}`, { name: d.name }); return [];
    case "session.tool.called": {
      const prior = tools.get(`${session}:${call}`) ?? {};
      prior.input = JSON.stringify(d.input); tools.set(`${session}:${call}`, prior);
      const record: EnvelopeRecord = { kind: "part", session_id: session, record_id: call, message_id: d.assistantMessageID, part_type: "tool", call_id: call, tool: prior.name, tool_status: "running" };
      boundedField(record, "input", prior.input); return [record];
    }
    case "session.tool.success":
    case "session.tool.failed": {
      const key = `${session}:${call}`;
      const prior = tools.get(key) ?? {};
      const record: EnvelopeRecord = { kind: "part", session_id: session, record_id: call, message_id: d.assistantMessageID, part_type: "tool", call_id: call, tool: prior.name, tool_status: event.type.endsWith("success") ? "completed" : "error", error: errorName(d.error) };
      boundedField(record, "input", prior.input);
      const content = Array.isArray(d.content) ? d.content : [];
      boundedField(record, "output", content.filter((item: any) => item.type === "text").map((item: any) => item.text).join("\n"));
      if (content.some((item: any) => item.type !== "text")) record.detail = "unsupported tool result content withheld";
      if (tools.size > MAX_BUFFERED_EVENTS) tools.delete(tools.keys().next().value!);
      const records: EnvelopeRecord[] = [record];
      // At-use provenance belongs only to execute.after. Failed reads load no
      // source; bus replay must never hash today's file as an old read version.
      return records;
    }
    case "session.execution.failed":
    case "session.step.failed": return [{ kind: "event", session_id: session, event: "session.error", detail: errorName(d.error) }];
    case "session.execution.interrupted": return [{ kind: "event", session_id: session, event: "session.error", detail: "MessageAbortedError" }, { kind: "event", session_id: session, event: "session.idle" }];
    case "session.execution.succeeded": return [{ kind: "event", session_id: session, event: "session.idle" }];
    case "session.status": return d.status?.type === "idle" ? [{ kind: "event", session_id: session, event: "session.idle" }] : [];
    case "session.compaction.started": return [{ kind: "event", session_id: session, event: "session.compacted" }];
    default: return [];
  }
}

/**
 * eventRecord maps one native bus event into the envelope record set.
 * Unknown event types return undefined: they are not task observables of
 * the pinned generation. Not exported: the loader instantiates plugin
 * exports only.
 */
function eventRecord(event: unknown): EnvelopeRecord | undefined {
  const raw = event as { type?: string; properties?: any };
  const type = typeof raw?.type === "string" ? raw.type : "";
  const properties = raw?.properties ?? {};
  switch (type) {
    case "session.created":
    case "session.updated": {
      const info = properties.info ?? {};
      if (typeof info.id !== "string" || info.id.length === 0) {
        return undefined;
      }
      return {
        kind: "session",
        session_id: info.id,
        parent_id: typeof info.parentID === "string" ? info.parentID : "",
        title: clamp(info.title) ?? "",
        directory: clamp(info.directory) ?? "",
      };
    }
    case "message.updated": {
      const info = properties.info ?? {};
      if (typeof info.id !== "string" || typeof info.sessionID !== "string") {
        return undefined;
      }
      return {
        kind: "message",
        record_id: info.id,
        session_id: info.sessionID,
        role: info.role === "user" || info.role === "assistant" ? info.role : undefined,
      };
    }
    case "message.part.updated": {
      const part = properties.part ?? {};
      if (
        typeof part.id !== "string" ||
        typeof part.sessionID !== "string" ||
        typeof part.messageID !== "string"
      ) {
        return undefined;
      }
      if (part.type === "text") {
        const record: EnvelopeRecord = {
          kind: "part",
          record_id: part.id,
          session_id: part.sessionID,
          message_id: part.messageID,
          part_type: "text",
          synthetic: part.synthetic === true,
        };
        boundedField(record, "text", part.text);
        return record;
      }
      if (part.type === "reasoning") {
        // Private reasoning is never collected (no text forwarded); the
        // record participates in coverage accounting only.
        return {
          kind: "part",
          record_id: part.id,
          session_id: part.sessionID,
          message_id: part.messageID,
          part_type: "reasoning",
          reasoning: true,
        };
      }
      if (part.type === "tool") {
        const state = part.state ?? {};
        const record: EnvelopeRecord = {
          kind: "part",
          record_id: part.id,
          session_id: part.sessionID,
          message_id: part.messageID,
          part_type: "tool",
          tool: typeof part.tool === "string" && part.tool.length > 0 ? part.tool : undefined,
          call_id: typeof part.callID === "string" && part.callID.length > 0 ? part.callID : undefined,
          tool_status: toolStatus(state),
          error: errorName(state.error),
        };
        if (state.input == null) {
          // The recorded input is absent in this state; the server
          // reconcile (drain) restores the full observable input later.
        } else {
          boundedField(record, "input", JSON.stringify(state.input));
        }
        boundedField(record, "output", state.output);
        return record;
      }
      // file/subtask/step/snapshot/patch/compaction parts are runtime
      // metadata, not task observables (spec mapping table).
      return undefined;
    }
    case "message.part.removed": {
      if (typeof properties.partID !== "string") {
        return undefined;
      }
      return {
        kind: "removed",
        record_id: properties.partID,
        message_id: properties.messageID,
        session_id: properties.sessionID,
      };
    }
    case "message.removed": {
      if (typeof properties.messageID !== "string") {
        return undefined;
      }
      return {
        kind: "removed",
        record_id: properties.messageID,
        message_id: properties.messageID,
        session_id: properties.sessionID,
      };
    }
    case "session.status": {
      if (typeof properties.sessionID !== "string") {
        return undefined;
      }
      return {
        kind: "event",
        event: "session.status",
        session_id: properties.sessionID,
        status: properties.status?.type,
      };
    }
    case "session.idle": {
      if (typeof properties.sessionID !== "string") {
        return undefined;
      }
      return {
        kind: "event",
        event: "session.idle",
        session_id: properties.sessionID,
      };
    }
    case "session.error": {
      if (typeof properties.sessionID !== "string") {
        return undefined;
      }
      return {
        kind: "event",
        event: "session.error",
        session_id: properties.sessionID,
        detail: errorName(properties.error),
      };
    }
    case "session.compacted": {
      if (typeof properties.sessionID !== "string") {
        return undefined;
      }
      return {
        kind: "event",
        event: "session.compacted",
        session_id: properties.sessionID,
      };
    }
    default:
      return undefined;
  }
}
