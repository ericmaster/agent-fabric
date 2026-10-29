// Contract: docs/specs/secret-guard.md — Pi runtime
import { execFile } from "node:child_process";
import { basename, join } from "node:path";
import { pathToFileURL } from "node:url";
import { SettingsManager, getAgentDir, getPackageDir } from "@earendil-works/pi-coding-agent";
import type { ExtensionAPI, ExtensionContext } from "@earendil-works/pi-coding-agent";

/** Placeholder replaced with an absolute helper path before load. Not a PATH lookup. */
export const HELPER_PATH = "__AGF_GUARD_HELPER__";
export const BLOCK_REASON = "guard_withheld";
export const HELPER_TIMEOUT_MS = 1000;

/** Stock tool_call names this entry checks. Other names are not guessed. */
export const STOCK_TOOLS = Object.freeze({ bash: "shell", read: "read" });

/**
 * Routes this entry does not intercept. Listing them is a non-claim, not protection.
 * shell-alias-expansion is a Phase 1 checker limit even when the prefix text is checked.
 */
export const UNSUPPORTED = Object.freeze([
  "spawnHook",
  "replaced-tools",
  "unobservable-prefix",
  "pi.exec",
  "user-bash",
  "shell-alias-expansion",
]);

const BLOCK = { block: true as const, reason: BLOCK_REASON };

type Request = {
  version: 1;
  op: "shell" | "read";
  cwd: string;
  source: "pi";
  command?: string;
  shell?: "bash" | "posix";
  paths?: string[];
};

export function createSecretGuard(helperPath: string = HELPER_PATH) {
  return function secretGuard(pi: ExtensionAPI) {
    pi.on("tool_call", async (event, ctx) => {
      try {
        if (event.toolName === "bash") return await checkShell(helperPath, event.input, ctx);
        if (event.toolName === "read") return await checkRead(helperPath, event.input, ctx);
      } catch {
        return BLOCK;
      }
    });
  };
}

export default createSecretGuard(HELPER_PATH);

function stockSettings(cwd: string) {
  if (!cwd.startsWith("/")) return null;
  try {
    return SettingsManager.create(cwd, getAgentDir());
  } catch {
    return null;
  }
}

/** Empty string is the stock default. null means the prefix cannot be read. */
export function stockCommandPrefix(cwd: string): string | null {
  const settings = stockSettings(cwd);
  if (!settings) return null;
  const prefix = settings.getShellCommandPrefix();
  if (prefix == null || prefix === "") return "";
  return typeof prefix === "string" ? prefix : null;
}

/** Stock unset shellPath is bash. A non-bash/sh shellPath is an unproven dialect. */
export function stockShellDialect(cwd: string): "bash" | "posix" | null {
  const settings = stockSettings(cwd);
  if (!settings) return null;
  const shellPath = settings.getShellPath();
  if (shellPath == null || shellPath === "") return "bash";
  if (typeof shellPath !== "string") return null;
  const name = basename(shellPath);
  if (name === "bash") return "bash";
  if (name === "sh") return "posix";
  return null;
}

async function checkShell(helper: string, input: unknown, ctx: ExtensionContext) {
  const command = stringField(input, "command");
  if (command === null) return BLOCK;
  const prefix = stockCommandPrefix(ctx.cwd);
  const shell = stockShellDialect(ctx.cwd);
  if (prefix === null || shell === null) return BLOCK;
  const effective = prefix ? `${prefix}\n${command}` : command;
  const decision = await callHelper(helper, {
    version: 1, op: "shell", command: effective, cwd: ctx.cwd, shell, source: "pi",
  });
  return decision === "allow" ? undefined : BLOCK;
}

type ReadResolver = (filePath: string, cwd: string) => Promise<string>;

let readResolver: Promise<ReadResolver | null> | undefined;

function nativeReadResolver(): Promise<ReadResolver | null> {
  readResolver ??= loadNativeReadResolver();
  return readResolver;
}

/** Actual pinned resolver via the public package root. Missing or throwing resolution fixed-blocks. */
async function loadNativeReadResolver(): Promise<ReadResolver | null> {
  try {
    const root = getPackageDir();
    if (!root.startsWith("/") || root.includes("\0")) return null;
    const href = pathToFileURL(join(root, "dist/core/tools/path-utils.js")).href;
    const mod = await import(href);
    return typeof mod.resolveReadPathAsync === "function" ? mod.resolveReadPathAsync : null;
  } catch {
    return null;
  }
}

async function resolvedReadTarget(raw: string, cwd: string): Promise<string | null> {
  const resolve = await nativeReadResolver();
  if (!resolve) return null;
  try {
    const target = await resolve(raw, cwd);
    if (typeof target !== "string" || !target.startsWith("/") || target.includes("\0")) return null;
    return target;
  } catch {
    return null;
  }
}

async function checkRead(helper: string, input: unknown, ctx: ExtensionContext) {
  const raw = stringField(input, "path");
  if (!raw || !ctx.cwd.startsWith("/")) return BLOCK;
  const target = await resolvedReadTarget(raw, ctx.cwd);
  if (!target) return BLOCK;
  const decision = await callHelper(helper, {
    version: 1, op: "read", paths: [target], cwd: ctx.cwd, source: "pi",
  });
  return decision === "allow" ? undefined : BLOCK;
}

function stringField(input: unknown, key: string): string | null {
  if (!input || typeof input !== "object" || !(key in input)) return null;
  const value = (input as Record<string, unknown>)[key];
  return typeof value === "string" ? value : null;
}

function callHelper(helper: string, request: Request): Promise<"allow" | "block"> {
  if (!helper.startsWith("/") || helper.includes("\0")) {
    return Promise.resolve("block");
  }
  return new Promise((resolve) => {
    let settled = false;
    const done = (value: "allow" | "block") => {
      if (!settled) {
        settled = true;
        resolve(value);
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
