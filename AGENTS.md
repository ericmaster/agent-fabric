# Agent Fabric

Portable, provider-neutral agent definitions and adapters. Canonical agents live in
`agents/<id>.md`; generated files are copied into a selected harness scope and owned
by `.agent-fabric-manifest.json`.

Run `go test ./...` before publishing. Do not add provider credentials or private
Company OS paths to this repository.
