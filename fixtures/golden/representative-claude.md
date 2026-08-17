---
description: "Representative adapter fixture"
mode: subagent
model: "provider/test"
variant: "high"
sandbox: "read-only"
visibility: "hidden"
isolation: "sandbox"
permission:
  bash: "allow"
  edit: "deny"
hooks: ["load-task"]
---
## Agent Fabric Policy
profile: reviewer
effort: high
visibility: hidden
isolation: sandbox
sandbox: read-only
permissions:
- bash: allow
- edit: deny
hooks: load-task

Fixture body.
