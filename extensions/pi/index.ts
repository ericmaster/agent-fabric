// Contract: docs/specs/agent-fabric.md — Pi
import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { getAgentDir, parseFrontmatter } from "@earendil-works/pi-coding-agent";
import type { ExtensionAPI, ExtensionContext } from "@earendil-works/pi-coding-agent";

type Mode = { name: string; description: string; tools: string[]; body: string; file: string };
type State = { name: string; baseline: string[] };
const stateType = "agent-fabric-mode";

// Use rendered definitions, never the canonical files with unresolved hooks.
export function loadModes(dir: string): Mode[] {
  let files: string[];
  try { files = readdirSync(dir).filter(f => f.endsWith(".md")).sort(); }
  catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return [];
    throw error;
  }
  return files.map(file => {
    const path = join(dir, file);
    const { frontmatter: fm, body } = parseFrontmatter<Record<string, unknown>>(readFileSync(path, "utf8"));
    const raw = typeof fm.tools === "string" ? fm.tools.split(",") : fm.tools;
    if (typeof fm.name !== "string" || !fm.name.trim() || fm.name === "off" ||
        typeof fm.description !== "string" || !body.trim() || !Array.isArray(raw) ||
        raw.length === 0 || raw.some(t => typeof t !== "string" || !t.trim())) {
      throw new Error(`Invalid Pi agent definition: ${path}`);
    }
    return { name: fm.name, description: fm.description, tools: raw.map(t => t.trim()), body, file: path };
  });
}

export default function fabricModes(pi: ExtensionAPI) {
  let mode: Mode | undefined;
  let baseline: string[] | undefined;
  let unavailable: string | undefined;

  pi.registerFlag("agf-mode", { description: "Fabric role for this session (or off)", type: "string" });

  function status(ctx: ExtensionContext) {
    if (ctx.hasUI) ctx.ui.setStatus("agent-fabric", unavailable ? `Fabric: unavailable (${unavailable})` : mode ? `Fabric: ${mode.name}` : undefined);
  }

  function select(name: string, ctx: ExtensionContext, save = true) {
    // Global only: project agent bodies are executable instructions. Do not
    // discover them implicitly or let a checkout shadow a trusted user role.
    const next = name === "off" ? undefined : loadModes(join(getAgentDir(), "agents")).find(m => m.name === name);
    if (name !== "off" && !next) throw new Error(`Unknown Fabric mode: ${name}`);
    if (next) {
      baseline ??= pi.getActiveTools();
      const registered = new Set(pi.getAllTools().map(t => t.name));
      const tools = next.tools.filter(t => registered.has(t));
      if (!tools.length) throw new Error(`No registered tools for ${name}`);
      pi.setActiveTools(tools);
    } else if (baseline) {
      pi.setActiveTools(baseline);
    }
    mode = next;
    unavailable = undefined;
    if (save) pi.appendEntry<State>(stateType, { name, baseline: baseline ?? pi.getActiveTools() });
    if (!next) baseline = undefined;
    status(ctx);
  }

  function restore(ctx: ExtensionContext, useFlag: boolean) {
    if (baseline) pi.setActiveTools(baseline);
    mode = undefined;
    baseline = undefined;
    unavailable = undefined;
    let saved: State | undefined;
    for (const entry of ctx.sessionManager.getBranch()) {
      if (entry.type === "custom" && entry.customType === stateType) {
        const value = entry.data as Partial<State> | undefined;
        saved = value && typeof value.name === "string" && Array.isArray(value.baseline) &&
          value.baseline.every(t => typeof t === "string") ? value as State : undefined;
      }
    }
    const flag = useFlag ? pi.getFlag("agf-mode") : undefined;
    const name = typeof flag === "string" ? flag : saved?.name ?? "off";
    baseline = saved?.baseline;
    try { select(name, ctx, typeof flag === "string"); }
    catch (error) {
      // Missing/invalid resumed roles must not silently resume unrestricted.
      unavailable = name;
      baseline ??= pi.getActiveTools();
      pi.setActiveTools([]);
      if (ctx.hasUI) ctx.ui.notify(String(error), "error");
      status(ctx);
    }
  }

  pi.on("session_start", (_event, ctx) => restore(ctx, true));
  pi.on("session_tree", (_event, ctx) => restore(ctx, false));

  pi.registerCommand("agf-mode", {
    description: "Select a rendered Fabric agent as the current role; off restores prior tools",
    getArgumentCompletions: prefix => [...loadModes(join(getAgentDir(), "agents")).map(m => m.name), "off"]
      .filter(name => name.startsWith(prefix)).map(name => ({ value: name, label: name })),
    handler: async (args, ctx) => {
      if (!ctx.isIdle()) { ctx.ui.notify("Wait until the current task is idle before switching roles", "warning"); return; }
      try {
        let name = args.trim();
        if (!name) {
          const modes = loadModes(join(getAgentDir(), "agents"));
          if (!ctx.hasUI) throw new Error("Usage: /agf-mode <agent-id|off>");
          const labels = modes.map(m => `${m.name} — ${m.description}`);
          const choice = await ctx.ui.select("Fabric role (same session; model unchanged)", [...labels, "off"]);
          if (!choice) return;
          name = choice === "off" ? "off" : modes[labels.indexOf(choice)].name;
        }
        select(name, ctx);
      } catch (error) { ctx.ui.notify(String(error), "error"); }
    },
  });

  pi.on("before_agent_start", event => {
    if (unavailable) throw new Error(`Fabric role ${unavailable} unavailable; choose /agf-mode <id|off>`);
    if (mode) return { systemPrompt: `${event.systemPrompt}\n\n## Active Fabric role: ${mode.name}\n${mode.body}` };
  });

  // Enforce the exact allowlist even for nested/codemode calls and tools another
  // extension activates later. No shell "read-only" regex or implicit MCP bypass.
  pi.on("tool_call", event => {
    if (unavailable || (mode && !mode.tools.includes(event.toolName))) {
      return { block: true, reason: `Tool ${event.toolName} is not allowed by Fabric role ${unavailable ?? mode?.name}` };
    }
  });
}
