// Contract: docs/specs/secret-guard.md — OpenCode runtime
import { execFile } from "node:child_process";
import { homedir } from "node:os";
import { join, resolve } from "node:path";

/** Placeholder replaced with an absolute helper path before load. Not a PATH lookup. */
export const HELPER_PATH = "__AGF_GUARD_HELPER__";
export const BLOCK_REASON = "guard_withheld";
export const HELPER_TIMEOUT_MS = 1000;
export const STOCK_READ = "read";

/**
 * shell.create.before carries env, cannot fail, and is not the final spawn.
 * A later hook or ShellTool.prepare can change command, cwd, and shell after it.
 */
export const SHELL_SUPPORT = "non-support";
export const SHELL_DIAGNOSTIC =
  "shell NON-SUPPORT: create.before is env-bearing, cannot fail, and a later rewrite changes command/cwd/shell before spawn";

/** Routes this entry does not intercept. Listing them is a non-claim, not protection. */
export const UNSUPPORTED = Object.freeze([
  "shell",
  "shell.create.before",
  "session-shell",
  "direct-subprocess",
  "native-persistence",
  "later-execute-before-rewrite",
  "alias-after-observation",
  "read-alternate-recovery",
  "grep",
  "mcp",
  "codemode",
  "all-subprocesses",
]);

export const SUPPORT = Object.freeze({
  read: "supported",
  shell: SHELL_SUPPORT,
  diagnostic: SHELL_DIAGNOSTIC,
  unsupported: UNSUPPORTED,
});

const READ_KEYS = "limit,offset,path";

type ReadRequest = {
  version: 1;
  op: "read";
  cwd: string;
  source: "opencode";
  paths: string[];
};

export function createSecretGuard(helperPath: string = HELPER_PATH) {
  const reads = new Set<string>([STOCK_READ]);
  const observe = (event: { tools?: Record<string, { input?: unknown }> } | undefined) => {
    const tools = event?.tools;
    if (!tools || typeof tools !== "object") return;
    for (const [name, tool] of Object.entries(tools)) {
      if (!name || name.includes("\0")) continue;
      if (name === STOCK_READ || isReadSchema(tool?.input)) reads.add(name);
    }
  };
  return {
    id: "agf-secret-guard",
    setup(ctx: {
      location?: { directory?: string };
      session: { hook: (name: string, callback: (event: { tools?: Record<string, { input?: unknown }> }) => void) => void };
      tool: { hook: (name: string, callback: (event: { tool?: string; input?: unknown }) => Promise<void> | void) => void };
    }) {
      ctx.session.hook("context", observe);
      ctx.session.hook("compaction", observe);
      ctx.session.hook("generate", observe);
      ctx.tool.hook("execute.before", async (event) => {
        try {
          if (!event?.tool || !reads.has(event.tool)) return;
          const directory = ctx.location?.directory;
          const target = nativeReadTarget(directory, stringField(event.input, "path"));
          if (!target || !directory) throw withheld();
          const decision = await callHelper(helperPath, {
            version: 1, op: "read", paths: [target], cwd: directory, source: "opencode",
          });
          if (decision !== "allow") throw withheld();
        } catch (error) {
          if (isWithheld(error)) throw error;
          throw withheld();
        }
      });
    },
  };
}

export default createSecretGuard(HELPER_PATH);

function withheld() {
  return new Error(BLOCK_REASON);
}

function isWithheld(error: unknown) {
  return error instanceof Error && error.message === BLOCK_REASON;
}

/** Stock read JSON schema only. Other path fields are not guessed. */
export function isReadSchema(schema: unknown) {
  if (!schema || typeof schema !== "object") return false;
  const record = schema as { type?: unknown; required?: unknown; properties?: Record<string, { type?: unknown }> };
  if (record.type !== "object" || !record.properties) return false;
  const keys = Object.keys(record.properties).sort().join(",");
  const required = Array.isArray(record.required) ? record.required : [];
  return keys === READ_KEYS && required.includes("path") && record.properties.path?.type === "string";
}

/** Same expansion as pinned FileAccess.resolvePath. A mismatch fixed-blocks in tests. */
export function nativeReadTarget(directory: string | undefined, input: string | null, home = process.env.OPENCODE_TEST_HOME ?? homedir()) {
  if (!directory || !directory.startsWith("/") || directory.includes("\0")) return null;
  if (!input || input.includes("\0")) return null;
  const normalized = windowsPath(input);
  const expanded = normalized === "~"
    ? home
    : normalized.startsWith("~/") || (process.platform === "win32" && normalized.startsWith("~\\"))
      ? join(home, normalized.slice(2))
      : normalized;
  const target = resolve(directory, expanded);
  if (!target.startsWith("/") || target.includes("\0")) return null;
  return target;
}

function windowsPath(input: string) {
  if (process.platform !== "win32") return input;
  return input
    .replace(/^\/([a-zA-Z]):(?:[\\/]|$)/, (_, drive) => `${drive.toUpperCase()}:/`)
    .replace(/^\/([a-zA-Z])(?:\/|$)/, (_, drive) => `${drive.toUpperCase()}:/`)
    .replace(/^\/cygdrive\/([a-zA-Z])(?:\/|$)/, (_, drive) => `${drive.toUpperCase()}:/`)
    .replace(/^\/mnt\/([a-zA-Z])(?:\/|$)/, (_, drive) => `${drive.toUpperCase()}:/`);
}

function stringField(input: unknown, key: string) {
  if (!input || typeof input !== "object" || !(key in input)) return null;
  const value = (input as Record<string, unknown>)[key];
  return typeof value === "string" ? value : null;
}

function callHelper(helper: string, request: ReadRequest): Promise<"allow" | "block"> {
  if (!helper.startsWith("/") || helper.includes("\0")) return Promise.resolve("block");
  return new Promise((resolveDecision) => {
    let settled = false;
    const done = (value: "allow" | "block") => {
      if (!settled) {
        settled = true;
        resolveDecision(value);
      }
    };
    const child = execFile(helper, ["guard", "check"], {
      timeout: HELPER_TIMEOUT_MS,
      maxBuffer: 65536,
      windowsHide: true,
    }, (error, stdout) => {
      if (error) {
        done("block");
        return;
      }
      try {
        const response = JSON.parse(String(stdout));
        done(response?.version === 1 && response?.decision === "allow" ? "allow" : "block");
      } catch {
        done("block");
      }
    });
    child.stdin.on("error", () => done("block"));
    child.stdin.end(JSON.stringify(request));
  });
}
