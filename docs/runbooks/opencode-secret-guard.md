# OpenCode secret-guard install

Opt-in only. This does not edit host OpenCode config, models, or permissions.
Behavior and unsupported routes: [secret-guard spec](../specs/secret-guard.md#opencode-runtime).

```sh
agf install --source "$PWD" --tools opencode --assets secret-guard --yes
```

Global destination:
`${OPENCODE_CONFIG_DIR:-${XDG_CONFIG_HOME:-~/.config}/opencode}/plugins/agf-secret-guard/index.ts`.
Project destination, with `--project`:
`<project>/.opencode/plugins/agf-secret-guard/index.ts`.

The entry embeds the absolute installer path. It checks stock and observed
read calls. Shell is non-support. Release archives stage the source and do
not enable the entry.
