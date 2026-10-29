---
title: "Spec: standalone secret guard"
type: spec
status: active
covers: internal/guard/, cmd/agent-fabric/guard.go
last_checked: 2026-10-29
---

# Standalone secret guard

`agf guard check` inspects syntax and path metadata; it never executes submitted
commands or opens submitted file contents. This voluntarily invoked check is **not
installed enforcement**, a sandbox, general secret detection, or output screening.
Callers must honor both deny and error before executing/reading anything themselves.
No adapter, installer, model, endpoint, SDK, daemon or server is configured by it.

## Protocol

`agent-fabric guard check [--log-file ABSOLUTE_PATH]` accepts exactly one JSON
object on stdin, at most 65536 bytes, and emits one JSON response on stdout.
Both operations require `version: 1` and absolute `cwd`. For `op: "read"`, supply
a nonempty string array `paths`; for `op: "shell"`, supply a present string
`command` (empty is valid) and `shell: "bash"` or `"posix"`.
Empty paths and NULs in command/cwd/paths are invalid. Optional `source` is empty or
`cli`, `sdk`, `pi`, `opencode`. Unknown fields, null fields, wrong types and extra
JSON values are invalid. Read requests must not contain nonempty `command` or
`shell`; shell requests must not contain nonempty `paths`.
Sensitive input belongs on stdin, not argv. No environment expansion, command
lookup or shell startup file loading occurs.

Responses contain only `version: 1`, `decision`, `reason`, `policy_version: 1`:

| Decision | Reason | Exit |
|---|---|---|
| allow | no_match | 0 |
| deny | environment_dump, proc_environ_read | 1 |
| error | invalid_request, limit_exceeded, unsupported_shell, parse_error, unsupported_dynamic_shell | 2 |

Errors never echo request text, parser excerpts, paths, values or exceptions.

### Parser support boundary

Valid shell grammar can exceed the pinned parser's support. The two mixed-quoted
heredocs below return `error/parse_error`, exit2: an **incomplete, unchecked
operation**, not `allow`, a policy denial, proof of invalid Bash or a completed
protective inspection. Callers must withhold execution on error.

These are JSON-escaped command strings (`\n` decodes to LF), not commands to run:

| Unchecked mixed-quoted input | Fully quoted control: `allow/no_match`, exit0 |
|---|---|
| `"sh -s <<E'O'F\nprintf '%s' '$( '\nEOF"` | `"sh -s <<'EOF'\nprintf '%s' '$( '\nEOF"` |
| `"sh -s <<e'n'v\n# comment \\\nenv"` | `"sh -s <<'env'\n# comment \\\nenv"` |

An author may choose a fully quoted equivalent only after preserving the intended
semantics. The checker never rewrites input, recovers a partial AST, executes Bash
or falls back to another parser. This narrow limitation does not exclude static
heredocs generally; supported programs still receive their normal classification.

## Shell matching

The pinned Go1.22-compatible `mvdan.cc/sh/v3` v3.8.0 parser traverses Bash/POSIX
AST statements, groups, functions, pipelines and substitutions. It checks common
`command`/`builtin`/`exec`/`env` wrappers, literal `eval` and static `sh`/`bash -c`
programs. Dynamic `eval "$code"` and `sh -c "$code"` return
`unsupported_dynamic_shell`, not an invented safe result. Limits:8 nested
shell-string levels and4096 total visited AST nodes across nested parses.

- Deny `env` without a child after supported options/assignments, `printenv`
  value output and bare `set`. Preserve assignments with safe children,
  `set -euo pipefail`, `set -- arg`, `set -o`, help/version, variable-presence
  checks, comments and inert strings.
- `env --` ends option parsing, not assignment processing; `eval --` removes
  its option terminator before parsing the literal program.
- Shell short flags from `c,e,l,s,u,x,v` are supported at the nested-program
  boundary. Static heredocs are parsed when stdin is selected (explicit `-s`,
  or no script operand after options). With `-s`, remaining operands are
  positional arguments; otherwise an external script is outside inspection.
  Unsupported options are not interpreted as supported shell programs.
  Stdin redirections apply left to right by numeric destination descriptor;
  duplication snapshots its source and moves/closes update that source. Only
  the effective fd0 heredoc/here-string is parsed, not superseded or non-stdin
  literal bodies. Closed fd0 has no program; an explicitly selected file,
  inherited/unknown descriptor or dynamic program returns
  `unsupported_dynamic_shell` without opening contents. Named-descriptor
  allocation/dataflow is outside this numeric-descriptor seam.
  Each redirect retains its own parse's original source; heredoc body spans
  exclude the closing delimiter line before `<<-` tab removal and context-specific
  decoding. Quoted literal programs do not use normalized parser body values.
- Only uncaptured, unpiped `printenv` with exactly one stdout `>` redirection to
  literal `/dev/null` is exempt. Pipelines or extra descriptor manipulation
  conservatively deny, even when stdout is discarded.
- Proc matching also applies to input redirections and operands of known
  readers: `cat`, `head`, `tail`, `od`, `xxd`, `hexdump`, `strings`, `grep`, `sed`,
  `awk`, `less`, `more`, `dd`. Dynamic word parts supply wildcard placeholders
  for lexical matching only, never actual expansion.
- `stat`/`test -e` are not content-read requests. Unknown ordinary executables,
  `python app.py`, `environment.md` and `printf '%s' "$TOKEN"` remain allowed.

## Read matching

Deny `/proc/<component>/environ` and
`/proc/<component>/task/<component>/environ`, including literal wildcard
components, relative paths and symlink targets. Lexical matching is deny-only,
including nonexistent proc targets. Metadata traversal preserves raw component
order, resolving symlinks before following `..`; it uses only Lstat/Readlink,
bounded to 40 links and 8192 steps, without glob expansion or content opens.
Unmatched, missing, cyclic and over-bound paths allow; this is a narrow checker,
not proof that a later read is safe. Metadata checks and later reads are not
atomic and cannot prevent replacement races.

## Denial audit

Linux writes denial metadata to
`${XDG_STATE_HOME}/agent-fabric/guard/denials.jsonl` when that root is absolute,
otherwise `$HOME/.local/state/agent-fabric/guard/denials.jsonl`.
`--log-file` is a trusted caller-selected absolute destination, not request data.
Allow/error decisions create no audit state.

Records contain only UTC timestamp, random event_id, source (default `cli`),
rule, policy_version and layer (`deterministic`); never request text, paths,
cwd, values or hashes. Created directories are0700, files0600. Existing final
parent must be owned0700; files must be owned regular0600 with one link.
Descriptor-relative no-follow traversal rejects symlinks. A shared lock bounds
contention to approximately200ms. Rotation at1MiB retains current and one
previous file plus lock. Audit failure preserves the decision/exit and emits
only `guard_audit_unavailable` on stderr. Non-Linux audit fails explicitly
rather than approximating the Linux guarantees.

## Safe caller example

Use an absolute trusted binary path and an ordinary subprocess API, no SDK.
This example checks a synthetic command without executing it:

```python
import json, os, subprocess

request = {"version": 1, "op": "shell", "command": "env",
           "cwd": os.getcwd(), "shell": "bash", "source": "sdk"}
checked = subprocess.run(["/trusted/path/agf", "guard", "check"],
                         input=json.dumps(request), text=True,
                         capture_output=True, timeout=2)
response = json.loads(checked.stdout)
assert response["version"] == response["policy_version"] == 1
assert checked.returncode == {"allow": 0, "deny": 1, "error": 2}[response["decision"]]
if response["decision"] != "allow":
    raise SystemExit("guard withheld operation")
```

## Pi runtime

`extensions/pi/secret-guard.ts` is source support for a loaded Pi `tool_call`
entry. This section does not install it, change host configuration, or make the
CLI an installed enforcement layer.

Stock `bash` checks `shellCommandPrefix + "\n" + command` when a prefix is set,
using the same settings read as the stock tool, then `guard check`. An unreadable
prefix, a non-bash/sh `shellPath`, deny, error, a missing helper, or a timeout
blocks with fixed reason `guard_withheld` and does not echo the request.

Stock `read` resolves the argument with the pinned package `resolveReadPathAsync`
before `guard check`. That resolver strips a leading `@`, expands `~`, decodes
`file://`, normalizes Unicode spaces, and resolves against cwd. The helper sees
that resolved target, not the raw argument. A missing resolver or a resolution
error fixed-blocks without a helper call and without opening target contents.
The resolver existence probe is metadata only.

A loaded entry still runs when Fabric role mode is `off` and when Pi dispatches
without approval. A separate process can load the same entry; parent handlers
are not inherited.

These routes are unsupported, not protected: custom `spawnHook`, replaced or
renamed tools, a tool-closure prefix that no longer matches settings, `pi.exec`,
and user `!`/`!!` (`user_bash`). Shell alias expansion remains outside the
checker even when the prefix text is included. A failed or absent entry is not
enforcement.

## Pi installation

`agent-fabric install --assets secret-guard --tools pi` is explicit opt-in and
does nothing unless `--tools` includes `pi`. Global entry:
`${PI_CODING_AGENT_DIR:-~/.pi/agent}/extensions/agf-secret-guard/index.ts`.
Project entry: `<project>/.pi/extensions/agf-secret-guard/index.ts`. Those are
the pinned Pi 1.4.0 loader destinations. The entry embeds the absolute installer
path and does not search `PATH`. It is not added to the role-mode package.
Install does not rewrite settings, models, permissions, or other extensions.

A prior manifest entry for this resource is owned only when its path is that
exact destination string. A matching hash is not authority, and a lexical alias
such as a symlink plus `..` is not that destination. install and sync fail before
migration or writes when a prior guard path is outside it. Eligible managed-file
removals on that install commit with the writes and are restored if a later
write fails. Release archives and bootstrap stage `extensions/pi/secret-guard.ts`
and do not enable the entry. Loaded behavior is [Pi runtime](#pi-runtime).

## OpenCode runtime

`extensions/opencode/secret-guard.ts` is source support for a plugin loaded by
the pinned OpenCode 2.2.0 V2 loader. It is not host enforcement. The loaded
entry calls the same `guard check` protocol with `source: "opencode"`.

Stock `read`, and a session `context`/`compaction`/`generate` tools key whose
input schema is exactly the stock read schema (`path`, `offset`, `limit`), is
checked at `tool.execute.before`. The path sent to the helper is the pinned
`FileAccess.resolvePath` expansion: leading `~` / `~/`, Windows shell-path
normalization, then resolve against `location.directory`. A missing directory,
resolution failure, helper error, missing helper, or timeout blocks with fixed
reason `guard_withheld` and does not echo the request. Denial fails the call
before the tool executor. Allow-all permission does not skip that hook.

A rename that happens after this entry's session hook is not visible here.
That route is unsupported, not protected. A later `execute.before` can still
change an allowed path; that route is unsupported. Filename recovery inside
the read executor, grep, MCP, and CodeMode are unsupported.

Shell is explicit non-support. `shell.create.before` carries environment data,
has no failure channel, and is not the final spawn: a later hook or
`ShellTool.prepare` can change command, cwd, and shell afterward. This entry
does not subscribe to that hook, rewrite commands, or throw inside a shell
executor. Direct session shell, other subprocesses, and native transcript
persistence are not covered. A failed or absent entry is not enforcement.

## OpenCode installation

`agent-fabric install --assets secret-guard --tools opencode` is explicit
opt-in and does nothing for this entry unless `--tools` includes `opencode`.
`--tools pi` remains the Pi entry only. Global destination:
`${OPENCODE_CONFIG_DIR:-${XDG_CONFIG_HOME:-~/.config}/opencode}/plugins/agf-secret-guard/index.ts`.
Project destination: `<project>/.opencode/plugins/agf-secret-guard/index.ts`.
Those are pinned 2.2.0 loader destinations. The entry embeds the absolute
installer path and does not search `PATH`. Install does not rewrite
`opencode.json`, models, permissions, or other plugins.

Ownership matches the Pi rule: a prior manifest path is this resource only
when it is that exact destination string. Release archives and bootstrap stage
`extensions/opencode/secret-guard.ts` and do not enable the entry. Loaded
behavior is [OpenCode runtime](#opencode-runtime). Operator command:
[opencode-secret-guard](../runbooks/opencode-secret-guard.md).

## Limits and verification

This is not general taint/dataflow analysis. Aliases/session state, external
scripts/interpreter code, copied executables, dynamic command names/path dataflow,
ANSI-C quoted names, exotic wrappers/options (`sudo`, `doas`, `nohup`, `timeout`,
BusyBox, `env -S`) and explicit variable-value output remain outside v1 coverage.
A successful check says only that supported rules did not match. It says nothing
about all subprocesses, native dispatch, raw streams, persistence or installed
harness enforcement. Later reads remain susceptible to replacement races.

The frozen [`70-case corpus`](../../fixtures/guard/corpus.json) activates all
D01–D39, A01–A22 and R01–R09 in policy and compiled CLI tests. Synthetic
placeholders are materialized, but commands remain data. Supplemental public CLI
tests cover SG-R1/R2/R4 wrapper state and SG-R3 read/shell consumers. Tests also
cover bounded parsing, private errors, nonexecution/content-free FIFO checks and
shell/read denial-audit permissions, rotation, concurrency and failure handling.

```sh
go test -json ./internal/guard ./cmd/agent-fabric -run '^TestGuardParserBoundary$' -count=1
go test -json ./internal/guard ./cmd/agent-fabric -run 'TestGuardCorpus|TestGuardCLI' -count=1
go test -json ./internal/guard ./cmd/agent-fabric -run 'TestGuardLimits|TestGuardPrivateErrors|TestGuardNoExecution' -count=1
go test -json ./internal/guard ./cmd/agent-fabric -run TestGuardDenialLog -count=1
go test ./...
```

Reported leaks/false denials use the existing [harness-review contract](task-retrospective.md):
private evidence, sanitized regression first, smallest independently reviewed
correction. No new collector/service or automatic policy relaxation.
