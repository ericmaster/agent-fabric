#!/usr/bin/env bash
# Spec: docs/specs/agent-fabric.md
set -euo pipefail

repo="${AGF_REPOSITORY:-ericmaster/agent-fabric}"
version="${AGF_VERSION:-latest}"
install_dir="${AGF_INSTALL_DIR:-${HOME}/.local/bin}"
case "$(uname -s)" in Linux) os=linux;; Darwin) os=darwin;; MINGW*|MSYS*|CYGWIN*) os=windows;; *) printf 'unsupported OS\n' >&2; exit 1;; esac
case "$(uname -m)" in x86_64|amd64) arch=amd64;; aarch64|arm64) arch=arm64;; *) printf 'unsupported architecture\n' >&2; exit 1;; esac
if [[ "$version" == latest ]]; then
  tag="$(curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 "https://api.github.com/repos/${repo}/releases/latest" | sed -n 's/.*"tag_name": "\([^"]*\)".*/\1/p')"
  [[ -n "$tag" ]] || { printf 'could not resolve latest release\n' >&2; exit 1; }
  clean="${tag#v}"
  prefix="https://github.com/${repo}/releases/download/${tag}"
  asset="agent-fabric_${clean}_${os}_${arch}.tar.gz"
else
  clean="${version#v}"
  prefix="https://github.com/${repo}/releases/download/v${clean}"
  asset="agent-fabric_${clean}_${os}_${arch}.tar.gz"
fi
export AGF_VERSION="$clean"
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 "$prefix/$asset" -o "$tmp/$asset"
curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 "$prefix/sha256sums.txt" -o "$tmp/sha256sums.txt"
expected="$(awk -v asset="$asset" '$2 == asset { print $1 }' "$tmp/sha256sums.txt")"
[[ -n "$expected" ]] || { printf 'missing checksum for %s\n' "$asset" >&2; exit 1; }
if command -v sha256sum >/dev/null; then
  actual="$(sha256sum "$tmp/$asset" | awk '{ print $1 }')"
else
  actual="$(shasum -a 256 "$tmp/$asset" | awk '{ print $1 }')"
fi
[[ "$actual" == "$expected" ]] || { printf 'checksum mismatch for %s\n' "$asset" >&2; exit 1; }
tar -tzf "$tmp/$asset" > "$tmp/members.txt"
while IFS= read -r member; do
  case "$member" in
    /*|../*|*/../*|*/..|..)
      printf 'unsafe archive path: %s\n' "$member" >&2
      exit 1
      ;;
  esac
done < "$tmp/members.txt"
tar -tvzf "$tmp/$asset" > "$tmp/list.txt"
while IFS= read -r entry; do
  case "${entry:0:1}" in
    l|h|c|b|p|s)
      printf 'unsupported archive entry type\n' >&2
      exit 1
      ;;
  esac
done < "$tmp/list.txt"
mkdir -p "$install_dir"
tar -xzf "$tmp/$asset" -C "$tmp"
if [[ "$os" == windows ]]; then bin="agent-fabric.exe"; else bin="agent-fabric"; fi
install -m 0755 "$tmp/$bin" "$install_dir/$bin"
rm -rf "$install_dir/agents" "$install_dir/adapters"
cp -R "$tmp/agents" "$tmp/adapters" "$install_dir/"
if [[ "$os" == windows ]]; then
  cp "$install_dir/$bin" "$install_dir/agf.exe"
else
  install -m 0755 "$install_dir/$bin" "$install_dir/agf"
fi
printf 'installed %s\n' "$install_dir/$bin"

if [[ -r /dev/tty ]] && exec 3<>/dev/tty 2>/dev/null; then
  exec "$install_dir/$bin" "$@" <&3
fi
if [[ $# -eq 0 ]]; then
  printf 'noninteractive install requires explicit agent-fabric flags (for example --all --tools opencode --yes)\n' >&2
  exit 2
fi
exec "$install_dir/$bin" "$@"
