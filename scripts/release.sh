#!/usr/bin/env bash
# Spec: docs/specs/agent-fabric.md
set -euo pipefail

version="${1:?usage: release.sh VERSION OUTPUT_DIR}"
out="${2:?usage: release.sh VERSION OUTPUT_DIR}"
root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
mkdir -p "$out"
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
  os="${target%/*}"; arch="${target#*/}"
  suffix=""; [[ "$os" == windows ]] && suffix=".exe"
  name="agent-fabric_${version}_${os}_${arch}"
  work="$(mktemp -d)"
  GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$work/agent-fabric${suffix}" "$root/cmd/agent-fabric"
  cp -R "$root/agents" "$root/adapters" "$root/hooks" "$work/"
  tar -C "$work" -czf "$out/${name}.tar.gz" "agent-fabric${suffix}" agents adapters hooks
  rm -rf "$work"
done
if command -v sha256sum >/dev/null; then
  (cd "$out" && sha256sum agent-fabric_*.tar.gz > sha256sums.txt)
else
  (cd "$out" && shasum -a 256 agent-fabric_*.tar.gz > sha256sums.txt)
fi
python3 - "$out" "$version" <<'PY'
import hashlib
import json
import pathlib
import sys

out = pathlib.Path(sys.argv[1])
version = sys.argv[2]
artifacts = []
for path in sorted(out.glob("agent-fabric_*.tar.gz")):
    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    artifacts.append({"name": path.name, "sha256": digest, "bytes": path.stat().st_size})
(out / "release-manifest.json").write_text(
    json.dumps({"schema": 1, "version": version, "artifacts": artifacts}, indent=2) + "\n"
)
PY
printf 'released %s to %s\n' "$version" "$out"
