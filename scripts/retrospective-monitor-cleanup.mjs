// Spec: docs/specs/retrospective-monitor.md — archive-first completed monitor cleanup.
import fs from 'node:fs/promises';
import { constants as C } from 'node:fs';
import path from 'node:path';
import { createHash } from 'node:crypto';
import { pathToFileURL } from 'node:url';

const PROJECT = '/home/ericmaster/nimbler-ops';
const TASK = 'd2785bca-a95c-4252-ab72-7e89a1b5e941';
const CONTROL = 'ses_eedbd0409ffeOPv2DOxqVRc1iZ';
const P = '/home/ericmaster/.npm-global/lib/node_modules/@openchamber/web';
const ROOT = '/home/ericmaster/tools/agent-fabric-worktrees/agf1/.artifacts-manager/monitor-output';
const ANCHOR = path.resolve(ROOT, '../..');
const STATE = '/home/ericmaster/.local/state/agent-fabric/openchamber-retrospective';
const GATE = '/home/ericmaster/.local/state/agent-fabric/monitoring/agf1-openchamber41629/gate-v2';
const LIMIT = 1024 * 1024;
const INPUT_BYTES = 1024 * 1024; // Spec: docs/specs/retrospective-monitor.md — max valid payload697465 bytes.
// Spec: docs/specs/retrospective-monitor.md via maintenance.
const hash = value => createHash('sha256').update(value).digest('hex');
// Spec: docs/specs/retrospective-monitor.md via maintenance.
function check(value, code = 'UNVERIFIED') { if (!value) throw new Error(code); }
// Spec: docs/specs/retrospective-monitor.md via receipt identity validation.
function id(value) { check(typeof value === 'string' && /^ses_[A-Za-z0-9]{1,64}$/.test(value)); return value; }
// Spec: docs/specs/retrospective-monitor.md via snapshot completion validation.
function stamp(value) { check(Number.isSafeInteger(value) && value >= 0); return value; }

// Spec: docs/specs/retrospective-monitor.md — safe ancestors/private output roots.
async function directory(target, create = false) {
  check(path.isAbsolute(target) && !target.split('/').includes('..'));
  if (create) check(target.startsWith(ANCHOR + '/'));
  let current = '/';
  for (const part of target.split('/').filter(Boolean)) {
    current = path.join(current, part);
    if (create && current.startsWith(ANCHOR + '/')) await fs.mkdir(current, { mode: 0o700 }).catch(e => { if (e.code !== 'EEXIST') throw e; });
    const s = await fs.lstat(current);
    check(s.isDirectory() && !s.isSymbolicLink() && [0, process.getuid()].includes(s.uid) && !(s.mode & 0o022));
    if (create && current === ANCHOR) check(s.uid === process.getuid()); // Existing declared durable anchor, never recreate it.
    if (create && current.startsWith(ANCHOR + '/')) {
      // Sync even an existing entry: a prior mkdir may have survived a failed parent fsync.
      const parent = await fs.open(path.dirname(current), C.O_RDONLY | C.O_DIRECTORY | C.O_NOFOLLOW);
      try { await parent.sync(); } finally { await parent.close(); }
    }
  }
  const s = await fs.lstat(target);
  check(s.uid === process.getuid() && (s.mode & 0o777) === 0o700);
}

// Spec: docs/specs/retrospective-monitor.md via archive/receipt validation.
async function read(file, limit = LIMIT) {
  const handle = await fs.open(file, C.O_RDONLY | C.O_NOFOLLOW | C.O_NONBLOCK);
  try {
    const s = await handle.stat();
    check(s.isFile() && s.uid === process.getuid() && s.nlink === 1 && (s.mode & 0o777) === 0o600 && s.size <= limit);
    const bytes = Buffer.alloc(limit + 1); let offset = 0;
    while (offset < bytes.length) {
      const { bytesRead } = await handle.read(bytes, offset, bytes.length - offset, null);
      if (!bytesRead) break;
      offset += bytesRead;
    }
    check(offset <= limit); return bytes.subarray(0, offset);
  } finally { await handle.close(); }
}

// Spec: docs/specs/retrospective-monitor.md — fsync/rename with no-overwrite publication.
export async function publish(file, data) {
  check(Buffer.byteLength(data) <= LIMIT);
  const parent = path.dirname(file);
  await directory(parent, true);
  const temp = await fs.mkdtemp(path.join(parent, '.cleanup-'));
  const pending = path.join(temp, 'pending'), ready = path.join(temp, 'ready');
  try {
    const handle = await fs.open(pending, C.O_WRONLY | C.O_CREAT | C.O_EXCL | C.O_NOFOLLOW, 0o600);
    try { await handle.writeFile(data); await handle.sync(); } finally { await handle.close(); }
    await fs.rename(pending, ready);
    // ponytail: link atomically refuses an existing target; rename alone would overwrite it.
    try { await fs.link(ready, file); } catch (e) { if (e.code !== 'EEXIST') throw e; }
    await fs.unlink(ready); await fs.rmdir(temp);
    const dir = await fs.open(parent, C.O_RDONLY | C.O_DIRECTORY | C.O_NOFOLLOW);
    try { await dir.sync(); } finally { await dir.close(); }
    check((await read(file)).equals(Buffer.from(data)), 'ARCHIVE_CONFLICT');
  } finally {
    await fs.unlink(pending).catch(() => {}); await fs.unlink(ready).catch(() => {});
    await fs.rmdir(temp).catch(() => {});
  }
}

// Spec: docs/specs/retrospective-monitor.md — drop global queue bodies immediately.
export function projectQueue(body) {
  check(body && Object.keys(body).sort().join(',') === 'revision,sessions' && Number.isSafeInteger(body.revision) && body.revision >= 0, 'QUEUE_UNVERIFIED');
  check(Array.isArray(body.sessions) && body.sessions.length <= 50, 'QUEUE_UNVERIFIED');
  const result = {};
  for (const row of body.sessions) {
    check(row && Object.keys(row).sort().join(',') === 'directory,items,sendingId,sessionId', 'QUEUE_UNVERIFIED');
    const session = id(row?.sessionId);
    check(!(session in result) && typeof row.directory === 'string' && Array.isArray(row.items) && row.items.length <= 20, 'QUEUE_UNVERIFIED');
    check(row.sendingId === null || (typeof row.sendingId === 'string' && row.sendingId.length > 0 && row.sendingId.length <= 256), 'QUEUE_UNVERIFIED');
    result[session] = { pending: row.items.length, sending: row.sendingId !== null };
  }
  return result; // No item text, content, attachment, context, model or directory retained.
}

// Spec: docs/specs/retrospective-monitor.md via snapshot/pre-delete check.
async function queueClear(request, sessionID) {
  let response = await request('/api/message-queue');
  check(response.status === 200, 'QUEUE_UNVERIFIED');
  const projected = projectQueue(response.body);
  response = null;
  check(!projected[sessionID]?.pending && !projected[sessionID]?.sending, 'USER_QUEUE_PENDING');
}

// Spec: docs/specs/retrospective-monitor.md — fixed producer identity, no ensure/start.
async function producer(request) {
  const r = await request('/health');
  check(r.status === 200 && r.body?.openCodePort === 41629 && r.body.openCodeRunning === true && r.body.isOpenCodeReady === true, 'PRODUCER_UNVERIFIED');
}

// Spec: docs/specs/retrospective-monitor.md — bounded complete pagination.
async function messages(request, sessionID) {
  const rows = [], seen = new Set(); let cursor;
  for (let page = 0; page < 8; page++) {
    const query = new URLSearchParams({ limit: '100', ...(cursor ? { cursor } : { order: 'asc' }) });
    const r = await request(`/api/session/${sessionID}/message?${query}`);
    check(r.status === 200 && Array.isArray(r.body?.data) && r.body.data.length <= 100 && r.body.cursor && typeof r.body.cursor === 'object');
    rows.push(...r.body.data);
    const next = r.body.cursor.next;
    if (next == null) return rows;
    check(typeof next === 'string' && next.length > 0 && next.length <= 4096 && !seen.has(next));
    seen.add(next); cursor = next;
  }
  throw new Error('HISTORY_PARTIAL');
}

// Spec: docs/specs/retrospective-monitor.md — common invariant before archive AND removal.
async function snapshot(request, sessionID) {
  await producer(request);
  const result = await request(`/api/session/${sessionID}`);
  check(result.status === 200, 'SESSION_UNAVAILABLE');
  const s = result.body;
  check(s?.id === sessionID && s.location?.directory === PROJECT && typeof s.projectID === 'string' && /^[A-Za-z0-9_-]{1,128}$/.test(s.projectID));
  check(!s.parentID && !s.fork && !s.revert && !s.subpath && !s.time?.archived && s.outcome === 'succeeded');
  check(JSON.stringify(s.metadata) === JSON.stringify({ openchamber: { kind: 'scheduled' } }));
  stamp(s.time.created); stamp(s.time.updated); stamp(s.time.idle);
  const active = await request('/api/session/active');
  check(active.status === 200 && active.body && typeof active.body === 'object' && !Array.isArray(active.body) && !(sessionID in active.body), 'SESSION_ACTIVE');
  const inbox = await request(`/api/session/${sessionID}/inbox`);
  check(inbox.status === 200 && Array.isArray(inbox.body) && inbox.body.length === 0, 'NATIVE_INBOX_PENDING');
  const children = await request(`/api/session?${new URLSearchParams({ parentID: sessionID, limit: '1' })}`);
  check(children.status === 200 && Array.isArray(children.body?.data) && children.body.data.length === 0 && children.body.cursor && children.body.cursor.next == null && children.body.cursor.previous == null, 'SESSION_HAS_CHILDREN_OR_UNKNOWN');
  const rows = await messages(request, sessionID);
  check(rows.length >= 3 && rows.filter(m => m.type === 'user').length === 1, 'CONTINUATION_OR_HISTORY_UNKNOWN');
  const seen = new Set();
  for (const row of rows) {
    check(typeof row.id === 'string' && /^[A-Za-z0-9_-]{1,128}$/.test(row.id) && !seen.has(row.id)); seen.add(row.id); stamp(row.time?.created);
    check(['user', 'assistant', 'idle', 'agent-switched', 'model-switched'].includes(row.type));
  }
  const end = rows.at(-1), final = rows.at(-2);
  check(end.type === 'idle' && end.outcome === 'succeeded' && !end.error && end.time.created === s.time.idle);
  check(final.type === 'assistant' && final.finish === 'stop' && !final.error && !final.retry && !final.metadata);
  check(stamp(final.time.completed) <= end.time.created && final.time.completed >= final.time.created && Array.isArray(final.content));
  check(rows.findIndex(m => m.type === 'user') < rows.length - 2);
  const text = [];
  for (const part of final.content) {
    if (part.type !== 'text') { check(['reasoning', 'tool'].includes(part.type)); continue; }
    check(Object.keys(part).every(k => ['type', 'text', 'state'].includes(k)) && typeof part.text === 'string');
    text.push(part.text);
  }
  const output = text.join('\n\n');
  check(output.trim().length > 0 && Buffer.byteLength(output) <= LIMIT / 2);
  check(!/authorization\s*:|\b(?:api[_ -]?key|password|access[_ -]?token)\s*[:=]|-----BEGIN .*PRIVATE KEY-----/i.test(output));
  const version = hash(JSON.stringify({ session: s, rows }));
  await queueClear(request, sessionID);
  return { sessionID, projectID: s.projectID, finalID: final.id, completed: end.time.created, version, output };
}

// Spec: docs/specs/retrospective-monitor.md — settled capture before history disappears.
async function captured(sessionID, completed) {
  await directory(path.join(STATE, 'jobs'));
  const job = JSON.parse(await read(path.join(STATE, 'jobs', `${sessionID}.json`)));
  return job.schema === 1 && job.session_id === sessionID && job.status === 'done' && job.server_url === 'http://127.0.0.1:41629'
    && (completed === undefined || (typeof job.updated_at === 'string' && Date.parse(job.updated_at) >= completed));
}

// Spec: docs/specs/retrospective-monitor.md — final-only private immutable archive.
async function archive(root, snap, write) {
  const relative = `${new Date(snap.completed).toISOString().slice(0, 10)}/${snap.sessionID}.md`;
  const meta = { schema: 'agf.monitor-output.v1', sessionID: snap.sessionID, taskID: TASK,
    project: PROJECT, projectID: snap.projectID, producer: 'http://127.0.0.1:41629',
    completed: snap.completed, sourceGeneration: snap.completed, finalID: snap.finalID,
    version: snap.version, outputSHA256: hash(snap.output) };
  const bytes = Buffer.from(`<!-- ${JSON.stringify(meta)} -->\n\n${snap.output}\n`);
  await write(path.join(root, relative), bytes);
  return { schema: 'agf.monitor-cleanup.v1', sessionID: snap.sessionID, taskID: TASK,
    relative, bytes: bytes.length, sha256: hash(bytes), version: snap.version };
}

// Spec: docs/specs/retrospective-monitor.md — no deletion authority from a filename alone.
async function prior(root, sessionID) {
  try { await directory(root); } catch (e) { if (e.code === 'ENOENT') return null; throw e; }
  let record;
  try { record = JSON.parse(await read(path.join(root, `${sessionID}.attempt.json`))); }
  catch (e) {
    if (e.code !== 'ENOENT') throw e;
    try { record = JSON.parse(await read(path.join(root, `${sessionID}.deleted.json`))); }
    catch (missing) { if (missing.code === 'ENOENT') return null; throw missing; }
  }
  check(record.schema === 'agf.monitor-cleanup.v1' && record.sessionID === sessionID && record.taskID === TASK);
  check(typeof record.relative === 'string' && record.relative.match(new RegExp(`^\\d{4}-\\d{2}-\\d{2}/${sessionID}\\.md$`)) && /^[0-9a-f]{64}$/.test(record.version));
  try {
    await directory(path.dirname(path.join(root, record.relative)));
    const bytes = await read(path.join(root, record.relative));
    check(bytes.length === record.bytes && hash(bytes) === record.sha256);
  } catch { throw new Error('ARCHIVE_UNVERIFIED'); }
  return record;
}

// Spec: docs/specs/retrospective-monitor.md — one deletion attempt, safe restart/ambiguity.
export async function maintenance(input, { root = ROOT, request, write = publish, jobReady = captured } = {}) {
  check(Array.isArray(input.known) && input.known.length <= 1024 && typeof input.plan === 'boolean');
  const known = [...new Set(input.known.map(id))].sort();
  check(input.after == null || known.includes(id(input.after)));
  const deadline = Date.now() + 30000, send = request;
  request = async (...args) => { check(Date.now() < deadline, 'BUDGET_EXHAUSTED'); return send(...args); };
  const result = { deleted: 0, reconciled: 0, eligible: 0, retained: 0, ambiguous: 0, after: input.after, candidates: [], retainedSessions: [], outputDirectory: root };
  const remaining = known.filter(x => !input.after || x > input.after);
  for (const sessionID of (remaining.length ? remaining : known).slice(0, 8)) {
    if (Date.now() >= deadline) break;
    result.after = sessionID;
    try {
      check(sessionID !== CONTROL, 'CONTROL_EXCLUDED');
      validateReceipt(input.receipts?.[sessionID], sessionID);
      const attempted = await prior(root, sessionID);
      if (attempted) {
        await producer(request);
        const exists = await request(`/api/session/${sessionID}`);
        if (exists.status === 404) {
          if (!input.plan) await write(path.join(root, `${sessionID}.deleted.json`), JSON.stringify(attempted));
          result.reconciled++;
        } else {
          result.ambiguous++; result.retainedSessions.push({ sessionID, code: 'DELETE_RECONCILIATION_REQUIRED' });
        }
        continue; // Includes a crash before the first DELETE: never replay an uncertain attempt.
      }
      check(await jobReady(sessionID), 'CAPTURE_UNSETTLED');
      const first = await snapshot(request, sessionID);
      result.eligible++; result.candidates.push({ sessionID, finalID: first.finalID, completed: first.completed, version: first.version });
      if (input.plan) continue;
      const record = await archive(root, first, write);
      await write(path.join(root, `${sessionID}.attempt.json`), JSON.stringify(record));
      const second = await snapshot(request, sessionID);
      check(second.version === first.version, 'SNAPSHOT_CHANGED');
      check(await jobReady(sessionID, first.completed), 'CAPTURE_UNSETTLED');
      check((await prior(root, sessionID)).sha256 === record.sha256);
      await queueClear(request, sessionID); // Last observation before the irreversible call.
      try {
        const removed = await request(`/api/session/${sessionID}`, 'DELETE');
        check(removed.status === 204);
      } catch { /* Ambiguous: GET only, never another DELETE. */ }
      const absent = await request(`/api/session/${sessionID}`);
      if (absent.status === 404) {
        await write(path.join(root, `${sessionID}.deleted.json`), JSON.stringify(record)); result.deleted++;
      } else { result.ambiguous++; result.retainedSessions.push({ sessionID, code: 'DELETE_RECONCILIATION_REQUIRED' }); }
    } catch (e) {
      result.retained++;
      const safe = new Set(['NO_DELETION_RECEIPT', 'USER_QUEUE_PENDING', 'QUEUE_UNVERIFIED', 'CAPTURE_UNSETTLED', 'SESSION_ACTIVE', 'NATIVE_INBOX_PENDING', 'SESSION_HAS_CHILDREN_OR_UNKNOWN', 'CONTINUATION_OR_HISTORY_UNKNOWN', 'CONTROL_EXCLUDED', 'PRODUCER_UNVERIFIED', 'SNAPSHOT_CHANGED', 'BUDGET_EXHAUSTED', 'ARCHIVE_CONFLICT']);
      result.retainedSessions.push({ sessionID, code: safe.has(e.message) ? e.message : 'CLEANUP_RETAINED_UNVERIFIED' });
    }
  }
  return result;
}

// Spec: docs/specs/retrospective-monitor.md — adapter entry verifies canonical durable authority.
export async function authorize(input, gate = GATE) {
  await directory(gate);
  const cursor = JSON.parse(await read(path.join(gate, 'gate-v1.json'), 8 * LIMIT));
  check(cursor.schema === 'agf.retrospective-gate.v1' && cursor.format === 2 && cursor.inflight === false && cursor.claim === null);
  check(cursor.binding === hash(JSON.stringify(['gate_binding', PROJECT, 4098, TASK])));
  check(Array.isArray(cursor.known) && cursor.known.length <= 1024 && Array.isArray(input.known));
  cursor.known.forEach(id);
  check(input.known.every(s => cursor.known.includes(id(s))) && (input.after ?? null) === (cursor.cleanup_after ?? null));
  check(input.receipts && typeof input.receipts === 'object' && !Array.isArray(input.receipts));
  for (const [sessionID, receipt] of Object.entries(input.receipts)) {
    validateReceipt(receipt, sessionID);
    check(JSON.stringify(receipt) === JSON.stringify(cursor.cleanup_receipts?.[sessionID]));
  }
}

// Spec: docs/specs/retrospective-monitor.md — classification is not deletion provenance.
export function validateReceipt(receipt, sessionID) {
  check(receipt && Object.keys(receipt).sort().join(',') === 'producer_origin,schema,source,source_sha256', 'NO_DELETION_RECEIPT');
  check(receipt.schema === 1 && receipt.producer_origin === 'http://127.0.0.1:41629', 'NO_DELETION_RECEIPT');
  const s = receipt.source;
  check(s && Object.keys(s).sort().join(',') === 'directory,enabled,kind,last_status,origin,run_session_id,session_id,task_id', 'NO_DELETION_RECEIPT');
  check(s.task_id === TASK && s.session_id === id(sessionID) && s.directory === PROJECT && s.origin === 'http://127.0.0.1:4098' && s.enabled === false, 'NO_DELETION_RECEIPT');
  check([null, 'idle', 'running', 'queued', 'success', 'error', 'skipped'].includes(s.last_status), 'NO_DELETION_RECEIPT');
  check((s.kind === 'schedule.run+list' && s.run_session_id === sessionID)
    || (s.kind === 'schedule.list.backlog' && s.run_session_id === null && !['running', 'queued'].includes(s.last_status)), 'NO_DELETION_RECEIPT');
  const canonical = Object.fromEntries(Object.entries(s).sort(([a], [b]) => a < b ? -1 : a > b ? 1 : 0));
  check(receipt.source_sha256 === hash(JSON.stringify(['cleanup_receipt', canonical])), 'NO_DELETION_RECEIPT');
}

// Spec: docs/specs/retrospective-monitor.md — bounded first-party transport, no secret inspection.
export function boundedFetch(fetch) {
  return async (url, options) => {
    check(new URL(url).origin === 'http://127.0.0.1:4098');
    const response = await fetch(url, { ...options, redirect: 'error' });
    const chunks = []; let bytes = 0;
    if (response.body) {
      const reader = response.body.getReader();
      try {
        for (;;) {
          const part = await reader.read(); if (part.done) break;
          bytes += part.value.byteLength; check(bytes <= LIMIT); chunks.push(part.value);
        }
      } finally { await reader.cancel().catch(() => {}); reader.releaseLock(); }
    }
    return new Response(response.status === 204 ? null : Buffer.concat(chunks), { status: response.status, headers: response.headers });
  };
}

// Spec: docs/specs/retrospective-monitor.md — first-party code alone owns authentication.
export function firstPartyTransport(requestJson) {
  return async (endpoint, method = 'GET') => {
    check(endpoint.startsWith('/api/session') || endpoint === '/api/message-queue' || endpoint === '/health');
    const { response, body } = await requestJson(4098, endpoint, { method, timeoutMs: 4000,
      headers: { 'x-opencode-directory': encodeURIComponent(PROJECT) } });
    if (method === 'DELETE') {
      if (response.status === 204) check(body === null, 'WIRE_INVALID');
      return { status: response.status, body };
    }
    if (response.status !== 200) return { status: response.status, body: null };
    const dataOnly = endpoint === '/api/session/active' || /^\/api\/session\/ses_[A-Za-z0-9]+(?:\/inbox)?$/.test(endpoint);
    if (dataOnly) {
      check(body && Object.keys(body).join(',') === 'data', 'WIRE_INVALID');
      const data = body.data;
      if (endpoint.endsWith('/inbox')) check(Array.isArray(data), 'WIRE_INVALID');
      else check(data && typeof data === 'object' && !Array.isArray(data), 'WIRE_INVALID');
      if (endpoint === '/api/session/active') {
        for (const [sessionID, active] of Object.entries(data)) {
          id(sessionID); check(active && Object.keys(active).join(',') === 'type' && active.type === 'running', 'WIRE_INVALID');
        }
      }
      return { status: response.status, body: data };
    }
    if (endpoint.startsWith('/api/session?') || /^\/api\/session\/ses_[A-Za-z0-9]+\/message\?/.test(endpoint)) {
      check(body && Object.keys(body).sort().join(',') === 'cursor,data' && Array.isArray(body.data), 'WIRE_INVALID');
      check(body.cursor && typeof body.cursor === 'object' && !Array.isArray(body.cursor), 'WIRE_INVALID');
      for (const [key, value] of Object.entries(body.cursor)) {
        check(['previous', 'next'].includes(key) && typeof value === 'string' && value.length > 0 && value.length <= 4096, 'WIRE_INVALID');
      }
    }
    return { status: response.status, body };
  };
}

// Spec: docs/specs/retrospective-monitor.md — installed first-party loader, no credential reader.
async function transport() {
  const pkg = JSON.parse(await fs.readFile(path.join(P, 'package.json'), 'utf8'));
  check(pkg.name === '@openchamber/web' && pkg.version === '2.3.3');
  const { requestJson } = await import(pathToFileURL(path.join(P, 'bin/lib/cli-http.js')).href);
  globalThis.fetch = boundedFetch(globalThis.fetch); // Isolated short-lived adapter process only.
  return firstPartyTransport(requestJson);
}

// Spec: docs/specs/retrospective-monitor.md — shared bounded production stdin reader.
export async function readInput(stream) {
  let body = '', bytes = 0;
  for await (const chunk of stream) {
    bytes += Buffer.byteLength(chunk); check(bytes <= INPUT_BYTES); body += chunk;
  }
  return JSON.parse(body);
}

// Spec: docs/specs/retrospective-monitor.md via Python gate invocation; output is metadata only.
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const timer = setTimeout(() => process.exit(1), 45000);
  try {
    const input = await readInput(process.stdin); await authorize(input);
    const result = await maintenance(input, { request: await transport() });
    process.stdout.write(JSON.stringify(result) + '\n');
  } catch { process.stdout.write('{"error":"CLEANUP_UNAVAILABLE"}\n'); process.exitCode = 1; }
  finally { clearTimeout(timer); }
}
