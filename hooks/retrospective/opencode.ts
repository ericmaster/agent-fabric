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
// Version pinning: fixture-tested against OpenCode 2.0.24
// (@opencode-ai/plugin@1.18.35, fixtures/retrospective/opencode/). The
// adapter probes the server version once and stays inert (degradation, never
// fake coverage) for an unconfirmed version.
//
// Runtime constraints: the installed plugin file is self-contained; runtime
// imports are Node/Bun built-ins only, so no per-project package.json is
// required. Type-only imports are erased at load time.
import type { Hooks, PluginInput } from "@opencode-ai/plugin";
import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { readFileSync, statSync } from "node:fs";
import path from "node:path";

const ENVELOPE_VERSION = 1;
const FLUSH_THRESHOLD_RECORDS = 25;
const MAX_BUFFERED_EVENTS = 2000;
const SPAWN_TIMEOUT_MS = 30_000;
const HEALTH_TIMEOUT_MS = 5_000;
const SUPPORTED_MAJOR_VERSIONS = [2];
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
function observedPath(args: unknown): string | undefined {
  if (!args || typeof args !== "object") {
    return undefined;
  }
  const candidate = (args as { filePath?: unknown; path?: unknown; file?: unknown }).filePath;
  if (typeof candidate === "string" && candidate.length > 0 && path.isAbsolute(candidate)) {
    return candidate;
  }
  return undefined;
}

/** resolveCLI locates the packaged Go CLI binary (AGF_BIN or PATH/home). */
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
type RunIngest = (envelope: Envelope, cli: string) => Promise<number | null>;

/** spawnIngest pipes the envelope through stdin; argv stays static. */
const spawnIngest: RunIngest = (envelope, cli) =>
  new Promise<number>((resolve) => {
    let child: ReturnType<typeof spawn>;
    try {
      child = spawn(cli, ["retrospective", "ingest", "--harness", "opencode"], {
        stdio: ["pipe", "ignore", "ignore"],
        env: process.env,
      });
    } catch {
      resolve(1);
      return;
    }
    const timer = setTimeout(() => {
      child.kill("SIGKILL");
      resolve(1);
    }, SPAWN_TIMEOUT_MS);
    child.on("close", (code) => {
      clearTimeout(timer);
      resolve(typeof code === "number" ? code : 1);
    });
    child.on("error", () => {
      clearTimeout(timer);
      resolve(1);
    });
    const stdin = child.stdin;
    if (!stdin) {
      clearTimeout(timer);
      resolve(1);
      return;
    }
    stdin.on("error", () => {});
    stdin.end(JSON.stringify(envelope));
  });

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
      const major = Number.parseInt(version.split(".")[0] ?? "", 10);
      if (Number.isInteger(major) && SUPPORTED_MAJOR_VERSIONS.includes(major) && this.cli) {
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
    if (!(await this.ensureActive())) {
      return null;
    }
    const buffer = this.buffers.get(sessionID);
    if (!buffer || buffer.length === 0) {
      return trigger ? EXIT_OK : null;
    }
    const envelope: Envelope = {
      version: ENVELOPE_VERSION,
      harness: "opencode",
      session_id: sessionID,
      server_url: this.serverUrl.toString().replace(/\/$/, ""),
      sdk_version: this.observedSDK,
      trigger,
      project_dir: this.projectDir || undefined,
      records: buffer,
    };
    this.buffers.set(sessionID, []);
    const code = await this.run(envelope, this.cli ?? "");
    if (code === EXIT_DISABLED) {
      this.buffers.set(sessionID, []);
      this.mode = "inert";
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
