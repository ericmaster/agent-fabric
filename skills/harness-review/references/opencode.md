# OpenCode native session and history API

Use the already-running OpenCode service through the installed `opencode api`
CLI. It is the same first-party HTTP surface the UI uses: no SDK, scripted
dispatcher, new server, private runner or hidden child is needed. Do not inspect
credentials or host configuration, and do not invent a port; pass `--server`
only for a documented origin. Keep the source session's agent/model tuple, which
`session.get` reports.

## Discover operations

`opencode api --help` documents the invocation (`--param`, `--data`, `--header`,
`--server`). Operation IDs, methods, paths, parameters and schemas come from the
running server's OpenAPI document:

```bash
opencode api GET /openapi.json
```

An unknown name fails as `Operation not found: <name>` (for example the V1-style
`session.messages`). That is a naming defect, not proof that history is missing.

## Read metadata, then history

```bash
opencode api session.get --param sessionID=<ses id>    # title, agent, model, outcome, time
opencode api session.message.list --param sessionID=<ses id> --param limit=20 --param order=asc
opencode api session.message.get --param sessionID=<ses id> --param messageID=<msg id>
```

`session.message.list` returns `{data, cursor: {previous, next}}`; page items carry
id, type, outcome and time. The first page may set `order=asc|desc`; continuation
passes the opaque `cursor` **alone**. `cursor` combined with `order` is rejected
(`InvalidCursorError: Cursor cannot be combined with order`) and an altered cursor
is rejected too. Follow `cursor.next` until it is null or empty: that, not a
title or the newest assistant answer, is the end of the paged range.

`limit` is a maximum, not a guarantee. A large page can exceed the CLI's output
cap and return truncated, unparsable JSON; reduce `limit` until the body parses, or
read one message with `session.message.get`. A truncated body or a 4xx JSON body
is incomplete coverage, never absent history; report the exact unread range.

## Launch a root and prompt

```json
{"title": "Harness review — <scope>",
 "agent": "<role agent>",
 "model": {"providerID": "<provider>", "id": "<model>", "variant": "<variant>"},
 "location": {"directory": "/absolute/project"}}
```

`opencode api session.create --data '<json>'` returns `{data: Session.Info}`; omit
`parentID` so the result is a new root rather than a fork, and require its `id` to
differ from the source. Then
`opencode api session.prompt --param sessionID=<new id> --data '{"text":"<brief>"}'`.
The prompt body takes a required top-level `text` string, not a V1 `parts` array.
A 2xx returns an inbox record rather than the transcript: verify the root with
`session.get` and read the actual prompt and replies through `session.message.list`.

## Read-only fallback

When the API is unavailable, inspect the native database read-only:
`sqlite3 -readonly <state>/opencode.db`. `session_v2` holds
id/parent_id/title/directory/model/time; `session_message` holds
session_id/type/seq/time_created/data for the current engine. A missing row or
table is not proof of absent history — older rows can live in the `message` table
and other engines keep their own store. Use the table that actually holds rows
before concluding a read is impossible.
