## Persist Ticket

Write the ticket payload JSON to a temporary file under the current execution
root. Invoke executable `{{.Script}}` with `AGENT_TICKET_PATH` pointing at that
file. Expect stdout JSON
`{"status":"ok|error","ticket_id","identifier","url","reason"}`.
On `error` or non-zero exit, fall back to writing the ticket to
`bugfix-tickets/UTC-ts-slug.md` under the current execution root with
self-generated id `bugfix-ts-slug`, and tell the reporter the local path in
plain words.
