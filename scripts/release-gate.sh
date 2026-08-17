#!/usr/bin/env bash
# Spec: docs/specs/agent-fabric.md
set -euo pipefail

# This is an internal-only disposable Proxmox validation gate, not a consumer
# install step. It is deliberately fail-closed and never discovers credentials or
# destroys a container unless every preflight check and the exact VMID claim pass.
: "${PROXMOX_NODE:?set PROXMOX_NODE for the release-gate target}"
: "${AGF_DEBIAN_TEMPLATE:?set AGF_DEBIAN_TEMPLATE to the disposable Debian template}"
: "${AGF_BRIDGE:?set AGF_BRIDGE to the validation bridge}"
: "${AGF_STORAGE:?set AGF_STORAGE to the disposable storage}"
: "${AGF_BOOTSTRAP_URL:=https://ericmaster.ninja/agent-fabric/install}"
node="$PROXMOX_NODE"
template="$AGF_DEBIAN_TEMPLATE"
bridge="$AGF_BRIDGE"
storage="$AGF_STORAGE"
name="agent-fabric-gate-$(date -u +%Y%m%d%H%M%S)-$$"
: "${AGF_GATE_APPROVED:?set AGF_GATE_APPROVED=1 only for an authorized disposable release gate}"
[[ "$AGF_GATE_APPROVED" == 1 ]] || { printf 'release gate not approved\n' >&2; exit 2; }
[[ "$AGF_BOOTSTRAP_URL" == https://* ]] || { printf 'bootstrap URL must use HTTPS\n' >&2; exit 2; }
[[ "$AGF_BOOTSTRAP_URL" != *[\'\"\\[:space:]]* ]] || { printf 'bootstrap URL contains unsafe shell characters\n' >&2; exit 2; }
for command in pvesh pct pvesm ip timeout python3 grep; do command -v "$command" >/dev/null || { printf 'CAPABILITY_UNAVAILABLE: %s\n' "$command" >&2; exit 127; }; done
pvesh get /nodes/"$node"/status >/dev/null
pvesh get /storage >/dev/null
pvesh get /cluster/nextid >/dev/null
ip link show "$bridge" >/dev/null
pvesh get /nodes/"$node"/storage/"$storage"/content >/dev/null
pvesh get /nodes/"$node"/storage/"$storage"/content | grep -F -- "$template" >/dev/null
vmid="$(pvesh get /cluster/nextid --output-format json | python3 -c 'import json,sys; value=json.load(sys.stdin); print(value["nextid"] if isinstance(value, dict) else value)')"
[[ -n "$vmid" ]] || { printf 'could not claim VMID\n' >&2; exit 1; }
if pct status "$vmid" >/dev/null 2>&1; then printf 'VMID collision: %s\n' "$vmid" >&2; exit 1; fi
if pct list | grep -F -- "$name" >/dev/null; then printf 'name collision: %s\n' "$name" >&2; exit 1; fi
[[ "$vmid" =~ ^[0-9]+$ ]] || { printf 'invalid VMID\n' >&2; exit 2; }
claimed=1
log_dir="${AGF_GATE_LOG_DIR:-/tmp/agent-fabric-release-gate-${vmid}}"
mkdir -p "$log_dir"
cleanup() {
  status=$?
  trap - EXIT
  if [[ "$claimed" == 1 ]]; then
    if ! pct config "$vmid" | grep -Fx "hostname: $name" >/dev/null; then
      if pct status "$vmid" >/dev/null 2>&1; then
        printf 'refusing cleanup: VMID %s is not the claimed container\n' "$vmid" >&2
        status=1
      fi
    else
      timeout 60 pct stop "$vmid" >/dev/null 2>&1 || true
      if ! timeout 60 pct destroy "$vmid" --purge 1 >/dev/null 2>&1; then
        printf 'cleanup destroy failed for VMID %s\n' "$vmid" >&2
        status=1
      fi
      if pct status "$vmid" >/dev/null 2>&1 || pct list | grep -F -- "$name" >/dev/null || pvesm list "$storage" | grep -F "subvol-${vmid}-disk" >/dev/null; then
        printf 'cleanup verification failed for VMID %s\n' "$vmid" >&2
        status=1
      fi
    fi
  fi
  exit "$status"
}
trap cleanup EXIT
pct create "$vmid" "$template" --hostname "$name" --unprivileged 1 --onboot 0 --cores 1 --memory 1024 --swap 512 --net0 "name=eth0,bridge=${bridge},ip=dhcp" --storage "$storage" --features nesting=0 >/dev/null
pct start "$vmid" >/dev/null
printf 'created disposable validation CT %s (%s)\n' "$vmid" "$name"
version="${AGF_VERSION:-latest}"
[[ "$version" =~ ^[[:alnum:]_.-]+$ ]] || { printf 'invalid AGF_VERSION\n' >&2; exit 2; }
pct exec "$vmid" -- sh -lc "set -eu
  command -v apt-get >/dev/null
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq
  apt-get install -y -qq --no-install-recommends ca-certificates curl tar coreutils
  command -v curl >/dev/null
  command -v tar >/dev/null
  command -v sha256sum >/dev/null
  command -v cat >/dev/null
  curl --fail --ipv4 --retry 3 --retry-all-errors --retry-delay 2 --proto '=https' --tlsv1.2 '$AGF_BOOTSTRAP_URL' -o /tmp/install.sh
  chmod 0755 /tmp/install.sh
  mkdir -p "\$HOME/.config/kilo/agent"
  ln -s "\$HOME/.config/kilo/agents/planner.md" "\$HOME/.config/kilo/agent/simplification-plan.md" || true
  AGF_VERSION='$version' bash /tmp/install.sh --all --tools opencode,kilo,agy,codex,claude --yes
  export PATH=\"\$HOME/.local/bin:\$PATH\"
  agf validate --all
  agf --version
  agf list
  agf doctor
  test ! -L "\$HOME/.config/kilo/agent/simplification-plan.md"
  test -f \"\$HOME/.config/opencode/agents/planner.md\"
  test -f \"\$HOME/.config/kilo/agents/planner.md\"
  test -f \"\$HOME/.agents/agents/planner.md\"
  test -f \"\$HOME/.codex/agents/planner.toml\"
  test -f \"\$HOME/.claude/agents/planner.md\"
  grep -q developer_instructions \"\$HOME/.codex/agents/planner.toml\"
  grep -q sandbox_mode \"\$HOME/.codex/agents/planner.toml\"
  grep -q 'edit: allow' \"\$HOME/.config/opencode/agents/planner.md\"
  ! grep -q x-agent-fabric \"\$HOME/.config/opencode/agents/planner.md\"
  grep -q 'sha256' \"\$HOME/.config/agent-fabric/.agent-fabric-manifest.json\"
  before=\"\$(sha256sum \"\$HOME/.config/opencode/agents/planner.md\")\"
  agf sync --all --tools opencode,kilo,agy,codex,claude --yes
  test \"\$(sha256sum \"\$HOME/.config/opencode/agents/planner.md\")\" = \"\$before\"
  printf '\\nuser edit\\n' >> \"\$HOME/.config/opencode/agents/planner.md\"
  if agf sync --all --tools opencode,kilo,agy,codex,claude --yes; then exit 1; fi
  agf sync --all --tools opencode,kilo,agy,codex,claude --yes --force
  mkdir -p /tmp/release-gate-project
  agf install --project /tmp/release-gate-project --agents planner --tools kilo --yes
  test -f /tmp/release-gate-project/.kilo/agents/planner.md
  agf uninstall --project /tmp/release-gate-project --force
  test ! -e /tmp/release-gate-project/.kilo/agents/planner.md
  agf hub install https://github.com/ericmaster/agent-hub --tools opencode,kilo,agy,codex,claude --yes
  mkdir -p /tmp/hub-collision/agents /tmp/release-gate-collision
  printf '%s\\n' '{\"schema\":1,\"agents\":{\"planner\":{\"requires\":[]}}}' > /tmp/hub-collision/hub.json
  cat > /tmp/hub-collision/agents/planner.md <<'EOF'
---
description: Collision fixture
mode: subagent
x-agent-fabric:
  schema: 1
  profile: planner
  effort: high
  visibility: public
  isolation: sandbox
---
collision fixture
EOF
  if agf hub install /tmp/hub-collision --project /tmp/release-gate-collision --tools opencode --yes; then exit 1; fi
  test ! -e /tmp/release-gate-collision/.opencode/agents/planner.md
  mkdir -p /tmp/hub-dependency/agents /tmp/release-gate-dependency
  printf '%s\\n' '{\"schema\":1,\"agents\":{\"fixture\":{\"requires\":[\"planner\"]}}}' > /tmp/hub-dependency/hub.json
  cat > /tmp/hub-dependency/agents/fixture.md <<'EOF'
---
description: Dependency fixture
mode: subagent
x-agent-fabric:
  schema: 1
  profile: planner
  effort: high
  visibility: public
  isolation: sandbox
  requires: [planner]
---
dependency fixture
EOF
  if agf hub install /tmp/hub-dependency --project /tmp/release-gate-dependency --tools opencode --yes; then exit 1; fi
  test ! -e /tmp/release-gate-dependency/.opencode/agents/fixture.md
  agf uninstall --force
  test ! -e \"\$HOME/.config/opencode/agents/planner.md\"
  test ! -e \"\$HOME/.codex/agents/mr-meeseeks.toml\"" | tee "$log_dir/container-validation.log"
printf 'release-gate evidence: %s\n' "$log_dir/container-validation.log"
