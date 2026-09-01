/**
 * Agent Fabric Landing Page Interactive Engine
 */

document.addEventListener('DOMContentLoaded', () => {
  initInstallTabs();
  initCopyButtons();
  initCompilerTabs();
  initCliExplorer();
  initFaqAccordion();
});

/* ==========================================================================
   1. Install Tabs & Copy Mechanics
   ========================================================================== */
function initInstallTabs() {
  const tabs = document.querySelectorAll('.install-tab');
  const cmdDisplay = document.getElementById('install-cmd-text');

  tabs.forEach(tab => {
    tab.addEventListener('click', () => {
      tabs.forEach(t => t.classList.remove('active'));
      tab.classList.add('active');
      const cmd = tab.getAttribute('data-cmd');
      if (cmdDisplay && cmd) {
        cmdDisplay.textContent = cmd;
      }
    });
  });
}

function initCopyButtons() {
  const copyBtn = document.getElementById('copy-btn');
  const cmdDisplay = document.getElementById('install-cmd-text');

  if (copyBtn && cmdDisplay) {
    copyBtn.addEventListener('click', async () => {
      const textToCopy = cmdDisplay.textContent.trim();
      try {
        await navigator.clipboard.writeText(textToCopy);
        showToast('✓ Command copied to clipboard!');
        copyBtn.classList.add('copied');
        setTimeout(() => copyBtn.classList.remove('copied'), 2000);
      } catch (err) {
        // Fallback for older browsers
        const textarea = document.createElement('textarea');
        textarea.value = textToCopy;
        document.body.appendChild(textarea);
        textarea.select();
        document.execCommand('copy');
        document.body.removeChild(textarea);
        showToast('✓ Command copied to clipboard!');
      }
    });
  }
}

function showToast(message) {
  const toast = document.getElementById('toast');
  if (!toast) return;
  toast.textContent = message;
  toast.classList.add('show');
  setTimeout(() => {
    toast.classList.remove('show');
  }, 2800);
}

/* ==========================================================================
   2. Multi-Harness Compiler Matrix Data & Switcher
   ========================================================================== */
const harnessData = {
  canonical: {
    path: 'agents/planner.md',
    badge: 'Canonical SSOT (Schema v1)',
    code: `---
description: Authors grounded, independently reviewed implementation plans as executable vertical slices
mode: primary
hooks: [load-task, pre-plan, post-plan, decompose]
x-agent-fabric:
  schema: 1
  profile: planner
  effort: high
  visibility: public
  isolation: sandbox
  permissions:
    edit: allow
    bash: allow
    task: allow
---
# Planner

You author and refine grounded implementation plans. You do not execute plan
phases, deploy, or treat a proposal as approval.

<system-reminder>
# Planner Mode - System Reminder
Planner mode is active. Do not execute implementation phases or deploy.
Write the canonical implementation plan at the proposal boundary.
</system-reminder>

## Operating Modes
- Author: create a new canonical plan from a goal or task seed.
- Refine: update an eligible draft plan in place.
- Approval/projection: invoke decompose hook with validated slices.`
  },
  opencode: {
    path: '.opencode/agents/planner.md',
    badge: 'Rendered for OpenCode',
    code: `---
name: planner
description: Authors grounded, independently reviewed implementation plans as executable vertical slices
mode: primary
model: openai/gpt-5.6-terra-pro
effort: high
sandbox: workspace
permission:
  edit: allow
  bash: allow
  task: allow
hooks:
  - load-task
  - pre-plan
  - post-plan
  - decompose
---
# Planner

You author and refine grounded implementation plans. You do not execute plan
phases, deploy, or treat a proposal as approval.

## Operating Modes
- Author: create a new canonical plan from a goal or task seed.
- Refine: update an eligible draft plan in place.
- Approval/projection: invoke decompose hook with validated slices.`
  },
  kilo: {
    path: '.kilo/agents/planner.md',
    badge: 'Rendered for Kilo Code',
    code: `---
name: planner
description: Authors grounded, independently reviewed implementation plans as executable vertical slices
mode: primary
model: anthropic/claude-3-7-sonnet
effort: high
sandbox: workspace
permission:
  edit: allow
  bash: allow
  task: allow
hooks:
  - load-task
  - pre-plan
  - post-plan
  - decompose
---
# Planner

You author and refine grounded implementation plans. You do not execute plan
phases, deploy, or treat a proposal as approval.

## Operating Modes
- Author: create a new canonical plan from a goal or task seed.
- Refine: update an eligible draft plan in place.`
  },
  antigravity: {
    path: '~/.gemini/config/agents/planner/agent.md',
    badge: 'Rendered for Antigravity CLI',
    code: `---
name: planner
description: Authors grounded, independently reviewed implementation plans as executable vertical slices
mode: primary
model: gemini-3.1-pro
variant: high
sandbox: workspace
visibility: public
isolation: sandbox
permission:
  edit: allow
  bash: allow
  task: allow
hooks:
  - load-task
  - pre-plan
  - post-plan
  - decompose
---
# Planner

You author and refine grounded implementation plans. You do not execute plan
phases, deploy, or treat a proposal as approval.

## Operating Modes
- Author: create a new canonical plan from a goal or task seed.
- Refine: update an eligible draft plan in place.`
  },
  codex: {
    path: '.codex/agents/planner.toml',
    badge: 'Rendered for Codex (TOML format)',
    code: `name = "planner"
description = "Authors grounded, independently reviewed implementation plans as executable vertical slices"
model = "openai/gpt-5.6-terra-pro"
sandbox_mode = "workspace"

developer_instructions = """
You author and refine grounded implementation plans. You do not execute plan
phases, deploy, or treat a proposal as approval.

## Operating Modes
- Author: create a new canonical plan from a goal or task seed.
- Refine: update an eligible draft plan in place.
- Approval/projection: invoke decompose hook with validated slices.
"""`
  },
  claude: {
    path: '.claude/agents/planner.md',
    badge: 'Rendered for Claude Code',
    code: `---
name: planner
description: Authors grounded, independently reviewed implementation plans as executable vertical slices
model: anthropic/claude-3-7-sonnet
tools:
  - Edit
  - Bash
  - Task
---
# Planner

You author and refine grounded implementation plans. You do not execute plan
phases, deploy, or treat a proposal as approval.

## Operating Modes
- Author: create a new canonical plan from a goal or task seed.
- Refine: update an eligible draft plan in place.`
  }
};

function initCompilerTabs() {
  const tabs = document.querySelectorAll('.compiler-tab');
  const pathDisplay = document.getElementById('compiler-file-path');
  const badgeDisplay = document.getElementById('compiler-target-badge');
  const codeDisplay = document.getElementById('compiler-code-block');

  tabs.forEach(tab => {
    tab.addEventListener('click', () => {
      tabs.forEach(t => t.classList.remove('active'));
      tab.classList.add('active');

      const target = tab.getAttribute('data-target');
      const data = harnessData[target];

      if (data && pathDisplay && badgeDisplay && codeDisplay) {
        pathDisplay.textContent = data.path;
        badgeDisplay.textContent = data.badge;
        codeDisplay.textContent = data.code;
      }
    });
  });
}

/* ==========================================================================
   3. Interactive CLI Explorer Simulation
   ========================================================================== */
const cliOutputs = {
  install: {
    label: 'agf install --all --tools opencode,kilo,agy,codex,claude --yes',
    status: 'Exit 0 • 12ms',
    content: `[INFO] Agent Fabric v1.0.0 (linux/amd64)
[INFO] Resolving canonical source definitions from bundled assets...
[INFO] Found 8 canonical agents: planner, plan-reviewer, plan-supervisor, implementor, expert-debugger, qa-runner, code-reviewer, loop-supervisor
[INFO] Resolving target adapters: opencode, kilo, antigravity, codex, claude
[INFO] Resolving hooks from ~/.agent-hooks/ (4 installed, 2 skipped)
[OK] Rendered 8 agents -> ~/.opencode/agents/
[OK] Rendered 8 agents -> ~/.kilo/agents/
[OK] Rendered 8 agents -> ~/.gemini/config/agents/
[OK] Rendered 8 agents -> ~/.codex/agents/
[OK] Rendered 8 agents -> ~/.claude/agents/
[OK] Manifest recorded at ~/.config/agent-fabric/.agent-fabric-manifest.json
[SUCCESS] Installed 40 target configurations successfully. Run 'agf doctor' to verify.`
  },
  sync: {
    label: 'agf sync --all --tools opencode,kilo --yes',
    status: 'Exit 0 • 8ms',
    content: `[INFO] Agent Fabric v1.0.0 (linux/amd64)
[INFO] Loading manifest: ~/.config/agent-fabric/.agent-fabric-manifest.json
[INFO] Checking 16 tracked target files against SHA-256 hashes...
[OK] Verified 15 untouched files (hashes match) -> Synced fresh definitions
[NOTICE] 1 file locally modified by user: ~/.opencode/agents/implementor.md (PRESERVED)
[OK] Manifest updated atomically.
[SUCCESS] Sync completed in 8ms. 1 user edit protected from overwrite.`
  },
  doctor: {
    label: 'agf doctor',
    status: 'Exit 0 • 4ms',
    content: `[DOCTOR] Agent Fabric Health Check
✔ Binary version: v1.0.0 (linux/amd64)
✔ Manifest integrity: ~/.config/agent-fabric/.agent-fabric-manifest.json (VALID)
✔ Tracked files: 40 managed entries
✔ Hash collisions: 0 detected
✔ File permissions: All directories readable & writable
✔ Adapter mappings: 5 targets available (opencode, kilo, antigravity, codex, claude)
✔ Hook directory: ~/.agent-hooks/ (Found load-task.sh, pre-plan.md, decompose.sh)
[RESULT] Everything looks healthy! Zero configuration errors.`
  },
  validate: {
    label: 'agf validate',
    status: 'Exit 0 • 3ms',
    content: `[VALIDATE] Inspecting Canonical Source & Adapter Mappings...
✔ agents/planner.md: Schema v1 valid, profile 'planner', 4 hooks declared
✔ agents/plan-reviewer.md: Schema v1 valid, profile 'reviewer'
✔ agents/plan-supervisor.md: Schema v1 valid, profile 'supervisor'
✔ agents/implementor.md: Schema v1 valid, profile 'worker'
✔ agents/expert-debugger.md: Schema v1 valid, profile 'worker'
✔ agents/qa-runner.md: Schema v1 valid, profile 'reviewer'
✔ agents/code-reviewer.md: Schema v1 valid, profile 'reviewer'
✔ agents/loop-supervisor.md: Schema v1 valid, profile 'supervisor'
✔ Adapters: 5 adapter schemas parsed with zero lint errors
[SUCCESS] 8 canonical definitions and 5 target adapters are 100% compliant.`
  },
  list: {
    label: 'agf list',
    status: 'Exit 0 • 2ms',
    content: `Available Canonical Agents (Source: Bundled v1.0.0):

ID                PROFILE       EFFORT    ISOLATION    DESCRIPTION
planner           planner       high      sandbox      Authors grounded, independently reviewed plans as vertical slices
plan-reviewer     reviewer      high      read-only    Critiques proposed plans for grounded evidence and boundary checks
plan-supervisor   supervisor    high      sandbox      Orchestrates discovery, grilling, and human approval gates
implementor       worker        high      sandbox      Executes single vertical slices test-first with DoD adherence
expert-debugger   worker        high      sandbox      Isolates hard bugs, test failures, and race conditions
qa-runner         reviewer      high      read-only    Executes test suites and builds reproduction cases for regressions
code-reviewer     reviewer      high      read-only    Dual-axis review: Standards conventions and Spec adherence
loop-supervisor   supervisor    high      sandbox      Drives autonomous implementation loops and subagent delegation`
  },
  hub: {
    label: 'agf hub install https://github.com/ericmaster/agent-hub --tools opencode,kilo --yes',
    status: 'Exit 0 • 142ms',
    content: `[HUB] Resolving external catalog: https://github.com/ericmaster/agent-hub
[HUB] Fetching repository manifest hub.json...
[HUB] Checking dependency graph for 'simplification-planner':
  ✔ Dependency satisfied: 'planner' (Fabric core)
  ✔ No name collisions found
[OK] Rendering 'simplification-planner' -> ~/.opencode/agents/simplification-planner.md
[OK] Rendering 'simplification-planner' -> ~/.kilo/agents/simplification-planner.md
[OK] Appending to manifest: ~/.config/agent-fabric/.agent-fabric-manifest.json
[SUCCESS] Hub agent installed and verified across 2 target harnesses.`
  }
};

function initCliExplorer() {
  const buttons = document.querySelectorAll('.cli-btn');
  const label = document.getElementById('cli-label');
  const content = document.getElementById('cli-output-content');
  const status = document.querySelector('.cli-status-badge');

  buttons.forEach(btn => {
    btn.addEventListener('click', () => {
      buttons.forEach(b => b.classList.remove('active'));
      btn.classList.add('active');

      const cmdKey = btn.getAttribute('data-cmd');
      const data = cliOutputs[cmdKey];

      if (data && label && content) {
        label.textContent = data.label;
        content.textContent = data.content;
        if (status) status.textContent = data.status;
      }
    });
  });
}

/* ==========================================================================
   4. FAQ Accordion
   ========================================================================== */
function initFaqAccordion() {
  const items = document.querySelectorAll('.faq-item');

  items.forEach(item => {
    const question = item.querySelector('.faq-question');
    if (!question) return;

    question.addEventListener('click', () => {
      const isActive = item.classList.contains('active');
      // Close other items
      items.forEach(i => i.classList.remove('active'));
      if (!isActive) {
        item.classList.add('active');
      }
    });
  });
}
