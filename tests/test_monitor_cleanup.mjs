// Spec: docs/specs/retrospective-monitor.md — cleanup public maintenance regression.
import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';
import { Readable } from 'node:stream';
import { maintenance, publish, authorize, projectQueue, boundedFetch, firstPartyTransport, readInput } from '../scripts/retrospective-monitor-cleanup.mjs';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const sha = value => createHash('sha256').update(value).digest('hex');
test('production stdin reader retains a fixed bound and rejects oversized or malformed input', async () => {
  await assert.rejects(readInput(Readable.from(['x'.repeat(1024 * 1024), 'x'])));
  await assert.rejects(readInput(Readable.from(['{invalid json'])));
  assert.deepEqual(await readInput(Readable.from(['{"plan":', 'true}'])), { plan: true });
});
// Spec: docs/specs/retrospective-monitor.md — synthetic verified run/post-list receipt.
function receipt(sessionID) {
  const source = { directory: '/home/ericmaster/nimbler-ops', enabled: false, kind: 'schedule.run+list',
    last_status: 'success', origin: 'http://127.0.0.1:4098', run_session_id: sessionID,
    session_id: sessionID, task_id: 'd2785bca-a95c-4252-ab72-7e89a1b5e941' };
  return { schema: 1, producer_origin: 'http://127.0.0.1:41629', source, source_sha256: sha(JSON.stringify(['cleanup_receipt', source])) };
}

// Spec: docs/specs/retrospective-monitor.md — native wire envelopes, nullable endpoints.
function wire(request) {
  return firstPartyTransport(async (_, e, options) => {
    const r = await request(e, options.method);
    const wrapped = /^\/api\/session\/ses_[A-Za-z0-9]+(?:\/inbox)?$/.test(e) || e === '/api/session/active';
    return { response: { status: r.status }, body: r.status === 200 && wrapped ? { data: r.body } : r.body };
  });
}

// Spec: docs/specs/retrospective-monitor.md — synthetic trusted current definition, no live prompt.
function legacyDefinition(id = 'ses_monitor123') {
  return { id: 'd2785bca-a95c-4252-ab72-7e89a1b5e941', name: 'AGF-1 OpenChamber session monitor', enabled: false,
    schedule: { timezone: 'UTC' }, execution: { prompt: 'PRIVATE_USER' }, state: { lastStatus: 'success', lastSessionId: id } };
}

// Spec: docs/specs/retrospective-monitor.md via public legacy regression fixtures.
function legacyOptions(f, definitions = [legacyDefinition(f.id)]) {
  f.session.title = 'AGF-1 OpenChamber session monitor 1970-01-01 00:00';
  return { ...f.options, listTasks: async () => ({ tasks: definitions }), request: wire(async (e, m) =>
    e.startsWith('/api/session?') && !new URL(e, 'http://fixture').searchParams.has('parentID')
      ? { status: 200, body: { data: [f.session], cursor: { previous: null, next: null } } } : f.request(e, m)) };
}
// Spec: docs/specs/retrospective-monitor.md via maintenance — fixtures only/no live APIs.
async function fixture(run) {
  const root = await fs.mkdtemp(path.join(HERE, '.cleanup-fixture-'));
  const id = 'ses_monitor123';
  const session = { id, projectID: 'prj_test', location: { directory: '/home/ericmaster/nimbler-ops' },
    metadata: { openchamber: { kind: 'scheduled' } }, outcome: 'succeeded', time: { created: 1000, updated: 3000, idle: 3000 } };
  const messages = [{ id: 'msg_user', type: 'user', time: { created: 1000 }, text: 'PRIVATE_USER' },
    { id: 'msg_final', type: 'assistant', time: { created: 2000, completed: 2900 }, finish: 'stop',
      content: [{ type: 'text', text: 'Visible summary.' }, { type: 'reasoning', text: 'PRIVATE_REASONING' }, { type: 'tool', output: 'PRIVATE_TOOL' }] },
    { id: 'msg_idle', type: 'idle', outcome: 'succeeded', time: { created: 3000 } }];
  const queue = { revision: 1, sessions: [{ sessionId: 'ses_other', directory: '/unrelated', items: [{ text: 'PRIVATE_QUEUE', content: 'PRIVATE_QUEUE' }], sendingId: null }] };
  const calls = []; let removed = false;
  const request = async (endpoint, method = 'GET') => {
    calls.push([endpoint, method]);
    if (endpoint === '/health') return { status: 200, body: { openCodePort: 41629, openCodeRunning: true, isOpenCodeReady: true } };
    if (endpoint === '/api/message-queue') return { status: 200, body: queue };
    if (endpoint === `/api/session/${id}`) {
      if (method === 'DELETE') {
        const file = path.join(root, '1970-01-01', `${id}.md`);
        const output = await fs.readFile(file, 'utf8');
        assert.match(output, /Visible summary/); assert.doesNotMatch(output, /PRIVATE/);
        assert.equal((await fs.stat(file)).mode & 0o777, 0o600);
        assert.equal((await fs.stat(path.dirname(file))).mode & 0o777, 0o700);
        assert.ok(await fs.readFile(path.join(root, `${id}.attempt.json`)));
        assert.equal(calls.at(-2)[0], '/api/message-queue');
        removed = true; return { status: 204, body: null };
      }
      return { status: removed ? 404 : 200, body: removed ? null : session };
    }
    if (endpoint === '/api/session/active') return { status: 200, body: {} };
    if (endpoint.includes('/inbox')) return { status: 200, body: [] };
    if (endpoint.includes('/message')) return { status: 200, body: { data: structuredClone(messages), cursor: {} } };
    if (endpoint.startsWith('/api/session?')) return { status: 200, body: { data: [], cursor: {} } };
    throw new Error('unexpected fixture request');
  };
  const input = { known: [id], receipts: { [id]: receipt(id) }, after: null, plan: false };
  const options = { root, request, jobReady: async () => true };
  try { await run({ root, id, session, messages, queue, calls, request, input, options }); }
  finally { await fs.rm(root, { recursive: true, force: true }); }
}

test('public path archives/fsyncs/verifies visible final BEFORE one204 delete and survives restart', () => fixture(async f => {
  const result = await maintenance(f.input, f.options);
  assert.equal(result.deleted, 1); assert.doesNotMatch(JSON.stringify(result), /PRIVATE|Visible summary/);
  const again = await maintenance(f.input, f.options);
  assert.equal(again.deleted, 0); assert.equal(again.reconciled, 1);
  assert.equal(f.calls.filter(x => x[1] === 'DELETE').length, 1);
}));

for (const kind of ['active', 'unknown', 'job-pending', 'resumed', 'child', 'no-final', 'failed', 'linked', 'foreign', 'origin', 'hidden', 'control', 'queue-pending', 'queue-sending', 'queue-malformed', 'queue-partial']) {
  test(`retains ${kind} even when native looks idle`, () => fixture(async f => {
    let request = f.request;
    if (kind === 'active') request = async (e, m) => e === '/api/session/active' ? { status: 200, body: { [f.id]: {} } } : f.request(e, m);
    if (kind === 'unknown') f.input.known = [];
    if (kind === 'job-pending') f.options.jobReady = async () => false;
    if (kind === 'resumed') f.messages.push({ id: 'msg_next', type: 'user', time: { created: 4000 } });
    if (kind === 'child') request = async (e, m) => e.startsWith('/api/session?') ? { status: 200, body: { data: [{ id: 'ses_child' }], cursor: {} } } : f.request(e, m);
    if (kind === 'no-final') f.messages[1].content = [];
    if (kind === 'failed') f.session.outcome = 'failed';
    if (kind === 'linked') f.session.metadata.openchamber.linked_issues = [{}];
    if (kind === 'foreign') f.session.location.directory = '/other';
    if (kind === 'hidden') f.messages[1].content[0].hidden = true;
    if (kind === 'control') f.input.known = ['ses_eedbd0409ffeOPv2DOxqVRc1iZ'];
    if (kind === 'origin') request = async (e, m) => e === '/health' ? { status: 200, body: { openCodePort: 4097 } } : f.request(e, m);
    if (kind === 'queue-pending' || kind === 'queue-sending') f.queue.sessions.push({ sessionId: f.id, directory: '/home/ericmaster/nimbler-ops', items: kind === 'queue-pending' ? [{ text: 'PRIVATE_QUEUE' }] : [], sendingId: kind === 'queue-sending' ? 'queued_test' : null });
    if (kind === 'queue-malformed') f.queue.sessions = null;
    if (kind === 'queue-partial') f.queue.cursor = 'more';
    const result = await maintenance(f.input, { ...f.options, request });
    assert.equal(f.calls.filter(x => x[1] === 'DELETE').length, 0);
    assert.doesNotMatch(JSON.stringify(result), /PRIVATE/);
  }));
}

test('read-only plan produces metadata only and writes nothing', () => fixture(async f => {
  const result = await maintenance({ ...f.input, plan: true }, f.options);
  assert.equal(result.eligible, 1); assert.deepEqual(await fs.readdir(f.root), []);
  assert.doesNotMatch(JSON.stringify(result), /PRIVATE|Visible summary/);
}));

test('native pagination carries order in opaque cursor, not a repeated order parameter', () => fixture(async f => {
  const request = async (e, m) => {
    if (e.includes('/message?')) {
      const query = new URL(e, 'http://fixture').searchParams;
      if (query.has('cursor')) {
        assert.equal(query.has('order'), false);
        return { status: 200, body: { data: f.messages.slice(1), cursor: {} } };
      }
      assert.equal(query.get('order'), 'asc');
      return { status: 200, body: { data: f.messages.slice(0, 1), cursor: { next: 'opaque-page2' } } };
    }
    return f.request(e, m);
  };
  assert.equal((await maintenance(f.input, { ...f.options, request })).deleted, 1);
}));

for (const fault of ['conflict', 'symlink', 'disk', 'fsync', 'second-read', 'job-changed', 'pagination', 'late-queue', 'last-queue']) {
  test(`archive/fresh-read fault retains session: ${fault}`, () => fixture(async f => {
    let request = f.request, write = publish, reads = 0, jobReady = async () => true;
    if (fault === 'conflict' || fault === 'symlink') {
      const day = path.join(f.root, '1970-01-01'); await fs.mkdir(day, { mode: 0o700 });
      if (fault === 'conflict') await fs.writeFile(path.join(day, `${f.id}.md`), 'unrelated', { mode: 0o600 });
      else await fs.symlink(path.join(f.root, 'elsewhere'), path.join(day, `${f.id}.md`));
    }
    if (fault === 'disk') write = async () => { throw new Error('ENOSPC'); };
    if (fault === 'job-changed') jobReady = async () => ++reads === 1;
    if (fault === 'second-read') request = async (e, m) => {
      if (e === `/api/session/${f.id}` && ++reads === 2) f.session.time.updated++;
      return f.request(e, m);
    };
    if (fault === 'pagination') request = async (e, m) => e.includes('/message') ? { status: 200, body: { data: f.messages, cursor: { next: 'repeat' } } } : f.request(e, m);
    if (fault === 'late-queue' || fault === 'last-queue') request = async (e, m) => {
      if (e === '/api/message-queue' && ++reads === (fault === 'late-queue' ? 2 : 3)) f.queue.sessions.push({ sessionId: f.id, directory: '', items: [{ text: 'PRIVATE_LATE' }], sendingId: null });
      return f.request(e, m);
    };
    const open = fs.open;
    if (fault === 'fsync') fs.open = async (...args) => {
      const h = await open(...args); if (String(args[0]).endsWith('/pending')) h.sync = async () => { throw new Error('fsync'); }; return h;
    };
    try { await maintenance(f.input, { ...f.options, request, write, jobReady }); }
    finally { fs.open = open; }
    assert.equal(f.calls.filter(x => x[1] === 'DELETE').length, 0);
  }));
}

test('ambiguous removal has one attempt and read-only reconciliation', () => fixture(async f => {
  let attempts = 0;
  const request = async (e, m) => { if (m === 'DELETE') { attempts++; throw new Error('timeout'); } return f.request(e, m); };
  assert.equal((await maintenance(f.input, { ...f.options, request })).ambiguous, 1);
  assert.equal((await maintenance(f.input, { ...f.options, request })).ambiguous, 1);
  assert.equal(attempts, 1);
}));

test('missing/conflicting archive cannot erase an ambiguous attempt or authorize replay', () => fixture(async f => {
  let attempts = 0;
  const request = async (e, m) => { if (m === 'DELETE') { attempts++; return { status: 500, body: null }; } return f.request(e, m); };
  await maintenance(f.input, { ...f.options, request });
  await fs.unlink(path.join(f.root, '1970-01-01', `${f.id}.md`));
  await maintenance(f.input, { ...f.options, request }); assert.equal(attempts, 1);
  await fs.writeFile(path.join(f.root, '1970-01-01', `${f.id}.md`), 'unrelated', { mode: 0o600 });
  await maintenance(f.input, { ...f.options, request }); assert.equal(attempts, 1);
}));

test('crash after delete before tombstone reconciles404 without second delete', () => fixture(async f => {
  const write = async (file, bytes) => { if (file.endsWith('.deleted.json')) throw new Error('crash'); await publish(file, bytes); };
  await maintenance(f.input, { ...f.options, write }); await maintenance(f.input, f.options);
  assert.equal(f.calls.filter(x => x[1] === 'DELETE').length, 1);
  assert.ok(await fs.readFile(path.join(f.root, `${f.id}.deleted.json`)));
}));

test('archive-only crash reuses immutable matching output', () => fixture(async f => {
  const write = async (file, bytes) => { if (file.endsWith('.attempt.json')) throw new Error('crash'); await publish(file, bytes); };
  await maintenance(f.input, { ...f.options, write });
  assert.equal(f.calls.filter(x => x[1] === 'DELETE').length, 0);
  assert.equal((await maintenance(f.input, f.options)).deleted, 1);
}));

test('receipt entry guard rejects wrong task, unknown ID and unsafe cursor', () => fixture(async f => {
  const file = path.join(f.root, 'gate-v1.json');
  const cursor = { schema: 'agf.retrospective-gate.v1', format: 2, binding: sha(JSON.stringify(['gate_binding', '/home/ericmaster/nimbler-ops', 4098, 'd2785bca-a95c-4252-ab72-7e89a1b5e941'])), inflight: false, claim: null, known: [f.id], cleanup_receipts: f.input.receipts };
  await fs.writeFile(file, JSON.stringify(cursor), { mode: 0o600 }); await authorize(f.input, f.root);
  await assert.rejects(authorize({ ...f.input, known: ['ses_other'] }, f.root));
  await fs.writeFile(file, JSON.stringify({ ...cursor, binding: 'wrong' })); await assert.rejects(authorize(f.input, f.root));
  await fs.unlink(file); await fs.symlink(path.join(f.root, 'other'), file); await assert.rejects(authorize(f.input, f.root));
}));

test('queue projection drops text and rejects duplicate/unbounded records', () => {
  assert.deepEqual(projectQueue({ revision: 1, sessions: [{ sessionId: 'ses_x', directory: '', items: [{ text: 'PRIVATE' }], sendingId: null }] }), { ses_x: { pending: 1, sending: false } });
  assert.throws(() => projectQueue({ revision: 1, sessions: Array(51).fill({}) }));
  const entry = { sessionId: 'ses_x', directory: '', items: [], sendingId: null };
  assert.throws(() => projectQueue({ revision: 1, sessions: [entry, entry] }));
});

test('bounded first-party fetch rejects oversize, foreign origin and redirects without secret logging', async () => {
  const fetch = boundedFetch(async () => new Response('x'.repeat(1024 * 1024 + 1)));
  await assert.rejects(fetch('http://127.0.0.1:4098/api/message-queue', {}));
  await assert.rejects(fetch('http://127.0.0.1:4097/api/message-queue', {}));
  let options;
  const safe = boundedFetch(async (_, init) => { options = init; return new Response('{}'); });
  await safe('http://127.0.0.1:4098/api/message-queue', {}); assert.equal(options.redirect, 'error');
});

test('fake first-party authentication transport owns fixed target/method/204 without secret acquisition', async () => {
  const calls = [];
  const transport = firstPartyTransport(async (port, endpoint, options) => {
    calls.push({ port, endpoint, options }); return { response: { status: 204 }, body: null };
  });
  assert.deepEqual(await transport('/api/session/ses_fixture', 'DELETE'), { status: 204, body: null });
  assert.equal(calls[0].port, 4098); assert.equal(calls[0].options.timeoutMs, 4000);
  assert.equal(calls[0].options.method, 'DELETE');
  assert.deepEqual(calls[0].options.headers, { 'x-opencode-directory': encodeURIComponent('/home/ericmaster/nimbler-ops') });
});

test('bounded rotation advances past retained IDs without any model request', () => fixture(async f => {
  const known = Array.from({ length: 20 }, (_, i) => `ses_${String(i).padStart(2, '0')}`);
  const options = { ...f.options, jobReady: async () => false };
  const one = await maintenance({ known, after: null, plan: true }, options);
  const two = await maintenance({ known, after: one.after, plan: true }, options);
  assert.equal(one.after, 'ses_07'); assert.equal(two.after, 'ses_15'); assert.equal(f.calls.length, 0);
}));

test('review provenance: manual classification ID never authorizes archive or deletion', () => fixture(async f => {
  const result = await maintenance({ ...f.input, receipts: {} }, f.options);
  assert.equal(result.deleted, 0);
  assert.deepEqual(await fs.readdir(f.root), []);
  assert.equal(f.calls.length, 0);
}));

test('review wire: real native data envelopes through first-party transport complete cleanup', () => fixture(async f => {
  f.session.metadata = { openchamber: { assist: { suggestion: 'PRIVATE_ASSIST', recap: '', forMessageID: 'msg_final', generatedAt: 3100 }, knowledge_context_delivered: 'l:on|m:on:c|r:on|w:auto', kind: 'scheduled' } };
  const transport = firstPartyTransport(async (_, endpoint, options) => {
    const response = await f.request(endpoint, options.method);
    if (response.body?.cursor) response.body.cursor = { previous: null, next: null };
    const wrapped = endpoint === `/api/session/${f.id}` || endpoint === '/api/session/active' || endpoint.endsWith('/inbox');
    return { response: { status: response.status }, body: wrapped && response.status === 200 ? { data: response.body } : response.body };
  });
  assert.equal((await maintenance(f.input, { ...f.options, request: transport })).deleted, 1);
}));

test('legacy composite discovery binds native user.text, not assistant text or classification IDs', () => fixture(async f => {
  f.input.receipts = {};
  f.session.title = 'AGF-1 OpenChamber session monitor 1970-01-01 00:00';
  const listTasks = async () => ({ tasks: [{ id: 'd2785bca-a95c-4252-ab72-7e89a1b5e941', name: 'AGF-1 OpenChamber session monitor', enabled: false,
    execution: { prompt: 'PRIVATE_USER' }, state: { lastStatus: 'success', lastSessionId: f.id } }] });
  const request = async (e, m) => e.startsWith('/api/session?') && !new URL(e, 'http://fixture').searchParams.has('parentID')
    ? { status: 200, body: { data: [f.session], cursor: { previous: null, next: null } } } : f.request(e, m);
  const result = await maintenance(f.input, { ...f.options, request, listTasks });
  assert.equal(result.deleted, 1);
  const archive = await fs.readFile(path.join(f.root, '1970-01-01', `${f.id}.md`), 'utf8');
  assert.match(archive, /agf.monitor-legacy.v1/); assert.doesNotMatch(archive, /PRIVATE_USER/);
}));

test('review durability: every newly created ancestor entry is parent-fsynced before DELETE', () => fixture(async f => {
  const root = path.join(f.root, '.artifacts-manager', 'monitor-output');
  const synced = new Set(), open = fs.open, mkdir = fs.mkdir, created = [];
  fs.mkdir = async (target, options) => { created.push(target); return mkdir(target, options); };
  fs.open = async (...args) => {
    const handle = await open(...args), sync = handle.sync.bind(handle);
    handle.sync = async () => { synced.add(String(args[0])); return sync(); };
    return handle;
  };
  try {
    const request = async (endpoint, method) => {
      if (method === 'DELETE') {
        for (const dir of [f.root, path.join(f.root, '.artifacts-manager'), root]) assert.ok(synced.has(dir), `missing parent fsync ${dir}`);
        return { status: 204, body: null };
      }
      return f.request(endpoint, method);
    };
    await maintenance(f.input, { ...f.options, root, request });
    for (const dir of [f.root, path.join(f.root, '.artifacts-manager'), root]) assert.ok(synced.has(dir), `missing parent fsync ${dir}`);
    assert.ok(created.length > 0 && created.every(dir => dir.startsWith(path.resolve(HERE, '..') + '/')));
  } finally { fs.open = open; fs.mkdir = mkdir; }
}));

for (const pending of ['active', 'inbox']) {
  test(`wire ${pending}: nested native pending work blocks archive/delete`, () => fixture(async f => {
    const transport = firstPartyTransport(async (_, endpoint, options) => {
      const r = await f.request(endpoint, options.method);
      let body = r.body;
      if (endpoint === `/api/session/${f.id}`) body = { data: body };
      if (endpoint === '/api/session/active') body = { data: pending === 'active' ? { [f.id]: { type: 'running' } } : {} };
      if (endpoint.endsWith('/inbox')) body = { data: pending === 'inbox' ? [{ id: 'inbox_fixture' }] : [] };
      return { response: { status: r.status }, body };
    });
    await maintenance(f.input, { ...f.options, request: transport });
    assert.equal(f.calls.filter(x => x[1] === 'DELETE').length, 0);
    assert.deepEqual(await fs.readdir(f.root), []);
  }));
}

test('wire invalid envelopes/error/partial pages fail closed per operation', async () => {
  for (const [endpoint, body] of [
    ['/api/session/ses_fixture', { id: 'ses_fixture' }],
    ['/api/session/ses_fixture', { data: {}, error: 'PRIVATE_ERROR' }],
    ['/api/session/active', { data: [] }],
    ['/api/session/active', { data: { ses_fixture: { type: 'idle' } } }],
    ['/api/session/ses_fixture/inbox', []],
    ['/api/session/ses_fixture/inbox', { data: [], partial: true }],
    ['/api/session/ses_fixture/message?limit=100', { data: [], cursor: {}, partial: true }],
    ['/api/session?parentID=ses_fixture', { data: [], cursor: { more: 'unknown' } }],
  ]) {
    const transport = firstPartyTransport(async () => ({ response: { status: 200 }, body }));
    await assert.rejects(transport(endpoint));
  }
});

for (const corruption of ['task', 'project', 'origin', 'hash']) {
  test(`invalid deletion receipt ${corruption} never authorizes archive`, () => fixture(async f => {
    const proof = f.input.receipts[f.id];
    if (corruption === 'task') proof.source.task_id = 'other-task';
    if (corruption === 'project') proof.source.directory = '/other';
    if (corruption === 'origin') proof.producer_origin = 'http://127.0.0.1:4097';
    if (corruption === 'hash') proof.source_sha256 = '0'.repeat(64);
    await maintenance(f.input, f.options);
    assert.equal(f.calls.length, 0); assert.deepEqual(await fs.readdir(f.root), []);
  }));
}

test('ancestor parent fsync failure prevents any DELETE', () => fixture(async f => {
  const open = fs.open;
  fs.open = async (...args) => {
    const handle = await open(...args);
    if (String(args[0]) === f.root) handle.sync = async () => { throw new Error('parent fsync'); };
    return handle;
  };
  try { await maintenance(f.input, { ...f.options, root: path.join(f.root, '.artifacts-manager', 'monitor-output') }); }
  finally { fs.open = open; }
  assert.equal(f.calls.filter(x => x[1] === 'DELETE').length, 0);
}));

for (const invalid of ['root', 'null', 'array', 'unknown', 'pin', 'goal', 'knowledge-type', 'knowledge-duplicate', 'knowledge-unsorted', 'knowledge-pin', 'knowledge-conflict', 'assist-null', 'assist-extra', 'assist-long', 'assist-id', 'assist-time', 'assist-future', 'assist-stale', 'assist-empty']) {
  test(`closed scheduled metadata retains ${invalid} through public wire`, () => fixture(async f => {
    const m = { kind: 'scheduled', knowledge_context_delivered: 'm:on:p|w:ask',
      assist: { recap: 'PRIVATE_RECAP', suggestion: 'PRIVATE_SUGGESTION', forMessageID: 'msg_final', generatedAt: 3100 } };
    f.session.metadata.openchamber = m;
    if (invalid === 'root') f.session.metadata.extra = true;
    if (invalid === 'null') f.session.metadata.openchamber = null;
    if (invalid === 'array') f.session.metadata.openchamber = [];
    if (invalid === 'unknown') m.taskID = legacyDefinition().id;
    if (invalid === 'pin') m.project_context_pins = [];
    if (invalid === 'goal') m.goal = {};
    if (invalid === 'knowledge-type') m.knowledge_context_delivered = {};
    if (invalid === 'knowledge-duplicate') m.knowledge_context_delivered = 'l:on|l:on';
    if (invalid === 'knowledge-unsorted') m.knowledge_context_delivered = 'r:on|l:on';
    if (invalid === 'knowledge-pin') m.knowledge_context_delivered = 'n:pin:1';
    if (invalid === 'knowledge-conflict') m.knowledge_context_delivered = 'm:on:c|m:on:p';
    if (invalid === 'assist-null') m.assist = null;
    if (invalid === 'assist-extra') m.assist.goal = true;
    if (invalid === 'assist-long') m.assist.suggestion = 'x'.repeat(501);
    if (invalid === 'assist-id') m.assist.forMessageID = 'msg_other';
    if (invalid === 'assist-time') m.assist.generatedAt = '3100';
    if (invalid === 'assist-future') m.assist.generatedAt = Date.now() + 86400000;
    if (invalid === 'assist-stale') m.assist.generatedAt = 2900 + 86400001;
    if (invalid === 'assist-empty') m.assist.recap = m.assist.suggestion = '';
    const r = await maintenance(f.input, { ...f.options, request: wire(f.request) });
    assert.equal(r.deleted, 0); assert.equal(r.eligible, 0); assert.deepEqual(await fs.readdir(f.root), []);
    assert.doesNotMatch(JSON.stringify(r), /PRIVATE/);
  }));
}

for (const signature of ['', 'm:on:c', 'l:on|m:on:p|r:on|w:ask', 'r:on|w:auto']) {
  test(`installed benign guide signature ${signature} is accepted independent of key ordering`, () => fixture(async f => {
    f.session.metadata.openchamber = { knowledge_context_delivered: signature, kind: 'scheduled' };
    assert.equal((await maintenance({ ...f.input, plan: true }, { ...f.options, request: wire(f.request) })).eligible, 1);
  }));
}

for (const value of [true, 1, '', [], {}, 'x'.repeat(4097)]) {
  test(`nullable cursor rejects invalid ${typeof value}/${String(value).length}`, async () => {
    const request = firstPartyTransport(async () => ({ response: { status: 200 }, body: { data: [], cursor: { next: value } } }));
    await assert.rejects(request('/api/session?limit=100'));
  });
}
for (const key of ['previous', 'next']) {
  test(`nonnull child ${key} cursor retains parent through wire`, () => fixture(async f => {
    const request = wire(async (e, m) => e.startsWith('/api/session?')
      ? { status: 200, body: { data: [], cursor: { previous: null, next: null, [key]: 'opaque' } } } : f.request(e, m));
    assert.equal((await maintenance(f.input, { ...f.options, request })).eligible, 0);
    assert.deepEqual(await fs.readdir(f.root), []);
  }));
}

for (const fault of ['prompt', 'assistant-only', 'later-repeat', 'project', 'name', 'task', 'kind', 'date', 'duplicate-task', 'ambiguous-prompt', 'supervisor', 'reviewer', 'child', 'parent', 'fork', 'late-instruction', 'queue', 'goal']) {
  test(`legacy composite never authorizes ${fault}`, () => fixture(async f => {
    f.input.receipts = {};
    const tasks = [legacyDefinition(f.id)], options = legacyOptions(f, tasks);
    if (fault === 'prompt') f.messages[0].text = 'UNRECOGNIZED_HISTORICAL';
    if (fault === 'assistant-only') { f.messages[0].text = 'OTHER'; f.messages[1].content[0].text = 'PRIVATE_USER'; }
    if (fault === 'later-repeat') f.messages.splice(1, 0, { ...f.messages[0], id: 'msg_repeat' });
    if (fault === 'project') f.session.location.directory = '/other';
    if (fault === 'name') tasks[0].name += ' other';
    if (fault === 'task') tasks[0].id = 'other-task';
    if (fault === 'kind') f.session.metadata.openchamber.kind = 'work';
    if (fault === 'date') f.session.title = 'AGF-1 OpenChamber session monitor 1970-01-01 00:01';
    if (fault === 'duplicate-task') tasks.push(structuredClone(tasks[0]));
    if (fault === 'ambiguous-prompt') tasks.push({ ...structuredClone(tasks[0]), id: '55d5f2b9-3ea8-4223-91fe-7cffdc71bcef', name: 'Monitor operativo de agentes — progreso y loops' });
    if (fault === 'supervisor' || fault === 'reviewer') f.session.agent = `ops-${fault}`;
    if (fault === 'parent') f.session.parentID = 'ses_parent';
    if (fault === 'fork') f.session.fork = { sessionID: 'ses_parent' };
    if (fault === 'goal') tasks[0].execution.goalEnabled = true;
    if (fault === 'queue') f.queue.sessions.push({ sessionId: f.id, directory: '', items: [{ text: 'PRIVATE_QUEUE' }], sendingId: null });
    const send = options.request; let reads = 0;
    options.request = async (e, m) => {
      if (fault === 'child' && e.includes('parentID=')) return { status: 200, body: { data: [{ id: 'ses_child' }], cursor: { previous: null, next: null } } };
      if (fault === 'late-instruction' && e === `/api/session/${f.id}` && ++reads === 2) f.messages[0].text = 'CHANGED';
      return send(e, m);
    };
    try { assert.equal((await maintenance(f.input, options)).deleted, 0); }
    catch (e) { assert.equal(e.message, 'TASK_DEFINITIONS_UNVERIFIED'); }
    assert.equal(f.calls.filter(c => c[1] === 'DELETE').length, 0);
    if (fault !== 'late-instruction') assert.deepEqual(await fs.readdir(f.root), []);
  }));
}

test('native synthetic knowledge is nonhuman but later authored instructions still retain', () => fixture(async f => {
  f.input.receipts = {};
  const options = legacyOptions(f);
  f.session.metadata.openchamber.knowledge_context_delivered = 'l:on';
  f.messages.unshift({ type: 'synthetic', id: 'msg_knowledge', text: 'PRIVATE_CONTEXT', time: { created: 1000 } });
  assert.equal((await maintenance({ ...f.input, plan: true }, options)).eligible, 1);
  f.messages.splice(2, 0, { ...f.messages[1], id: 'msg_human' });
  assert.equal((await maintenance(f.input, options)).eligible, 0);
  assert.deepEqual(await fs.readdir(f.root), []);
}));

test('complete 195-candidate paginated wire discovery rotates bounded batches across all three tasks without LLM wake', () => fixture(async f => {
  const names = ['AGF-1 OpenChamber session monitor', 'Monitor operativo de agentes — progreso y loops', 'Monitor operativo de agentes — persistente'];
  const ids = [legacyDefinition().id, '55d5f2b9-3ea8-4223-91fe-7cffdc71bcef', 'ce0092b5-5552-4d14-a695-592ff7b48d96'];
  const tasks = ids.map((id, i) => ({ ...legacyDefinition(null), id, name: names[i], execution: { prompt: `SYNTHETIC_TASK_${i}` } }));
  const sessions = Array.from({ length: 195 }, (_, i) => ({ ...structuredClone(f.session), id: `ses_${String(i).padStart(3, '0')}`, title: `${names[i % 3]} 1970-01-01 00:00` }));
  const unrelated = Array.from({ length: 78 }, (_, i) => ({ ...structuredClone(f.session), id: `ses_other${i}`, metadata: {}, title: 'Normal work' }));
  const deleted = new Set(), inspected = new Set(); let pages = 0, deletes = 0;
  const request = wire(async (e, method) => {
    if (e.startsWith('/api/session?') && !e.includes('parentID=')) {
      const q = new URL(e, 'http://fixture').searchParams, offset = Number(q.get('cursor') || 0);
      assert.equal(q.get('directory'), '/home/ericmaster/nimbler-ops', 'every discovery page must explicitly scope the native list');
      const records = [...sessions, ...unrelated].filter(s => !deleted.has(s.id)); pages++;
      return { status: 200, body: { data: records.slice(offset, offset + 100), cursor: { previous: null, next: offset + 100 < records.length ? String(offset + 100) : null } } };
    }
    const match = e.match(/^\/api\/session\/(ses_\d{3})(\/message\?.*|\/inbox)?$/);
    if (!match) return f.request(e, method);
    const sessionID = match[1], index = Number(sessionID.slice(4)), session = sessions[index];
    inspected.add(sessionID);
    if (method === 'DELETE') {
      const bytes = await fs.readFile(path.join(f.root, '1970-01-01', `${sessionID}.md`), 'utf8');
      assert.match(bytes, /agf.monitor-legacy.v1/); assert.doesNotMatch(bytes, /SYNTHETIC_TASK|PRIVATE/);
      assert.equal(JSON.parse(bytes.match(/<!-- (.*) -->/)[1]).taskID, ids[index % 3]);
      deleted.add(sessionID); deletes++; return { status: 204, body: null };
    }
    if (match[2]?.startsWith('/message')) {
      const rows = structuredClone(f.messages); rows[0].text = index % 5 === 0 ? 'UNRECOGNIZED_HISTORICAL' : tasks[index % 3].execution.prompt;
      return { status: 200, body: { data: rows, cursor: { previous: null, next: null } } };
    }
    if (match[2] === '/inbox') return { status: 200, body: [] };
    return { status: deleted.has(sessionID) ? 404 : 200, body: session };
  });
  let input = { known: ['ses_eedbd0409ffeOPv2DOxqVRc1iZ'], receipts: {}, after: null, plan: false };
  for (let tick = 0; tick < 25; tick++) {
    // Simulate the Python-owned durable classification barrier before the destructive phase.
    const classification = await maintenance({ ...input, plan: true }, { ...f.options, request, listTasks: async () => ({ tasks }) });
    input.known = [...new Set([...input.known, ...classification.proven])];
    const result = await maintenance(input, { ...f.options, request, listTasks: async () => ({ tasks }) });
    assert.ok(result.candidates.length + result.retained <= 8);
    assert.doesNotMatch(JSON.stringify(result), /SYNTHETIC_TASK|PRIVATE/);
    input = { ...input, after: result.after, known: [...new Set([...input.known, ...result.proven])] };
  }
  assert.equal(inspected.size, 195); assert.equal(deletes, 156); assert.equal(deleted.size, 156);
  assert.ok(pages >= 100 && pages <= 150); // Both phases complete inventory, no offset skips.
  assert.equal(input.known.length, 157);
}));

for (const fault of ['repeat', 'cap', 'duplicate']) {
  test(`incomplete native inventory ${fault} aborts before any archive/delete`, () => fixture(async f => {
    const options = legacyOptions(f), send = options.request;
    let page = 0;
    options.request = async (e, m) => {
      if (e.startsWith('/api/session?') && !e.includes('parentID=')) {
        page++;
        return { status: 200, body: { data: fault === 'duplicate' ? [f.session, f.session] : [], cursor: { previous: null, next: fault === 'repeat' ? 'same' : String(page) } } };
      }
      return send(e, m);
    };
    await assert.rejects(maintenance({ ...f.input, receipts: {} }, options), /INVENTORY_PARTIAL/);
    assert.ok(page <= 8); assert.deepEqual(await fs.readdir(f.root), []);
  }));
}

test('operational task lastSessionIds are probed but missing scheduled kind never gains composite authority', () => fixture(async f => {
  const tasks = [legacyDefinition(null),
    { ...legacyDefinition('ses_oldone'), id: '55d5f2b9-3ea8-4223-91fe-7cffdc71bcef', name: 'Monitor operativo de agentes — progreso y loops', execution: { prompt: 'SYNTHETIC_OLD_ONE' } },
    { ...legacyDefinition('ses_oldtwo'), id: 'ce0092b5-5552-4d14-a695-592ff7b48d96', name: 'Monitor operativo de agentes — persistente', execution: { prompt: 'SYNTHETIC_OLD_TWO' } }];
  const read = new Set();
  const request = wire(async (e, m) => {
    if (e.startsWith('/api/session?') && !e.includes('parentID=')) return { status: 200, body: { data: [], cursor: { previous: null, next: null } } };
    if (/^\/api\/session\/ses_old(one|two)$/.test(e)) {
      const id = e.split('/').at(-1); read.add(id);
      return { status: 200, body: { ...f.session, id, metadata: {}, title: 'Old task' } };
    }
    return f.request(e.replace(/ses_old(one|two)/, f.id), m);
  });
  const r = await maintenance({ known: [], receipts: {}, after: null, plan: false }, { ...f.options, request, listTasks: async () => ({ tasks }) });
  assert.deepEqual([...read].sort(), ['ses_oldone', 'ses_oldtwo']);
  assert.equal(r.retained, 2); assert.equal(r.eligible, 0); assert.deepEqual(r.proven, []);
  assert.deepEqual(await fs.readdir(f.root), []);
}));

test('fresh legacy metadata changes cannot reuse an earlier attestation', () => fixture(async f => {
  const options = legacyOptions(f), send = options.request; let reads = 0;
  f.session.metadata.openchamber.assist = { recap: 'PRIVATE_RECAP', suggestion: '', forMessageID: 'msg_final', generatedAt: 3100 };
  options.request = async (e, m) => {
    if (e === `/api/session/${f.id}` && ++reads === 2) f.session.metadata.openchamber.assist.recap = 'PRIVATE_CHANGED';
    return send(e, m);
  };
  const result = await maintenance({ ...f.input, receipts: {} }, options);
  assert.equal(result.deleted, 0); assert.equal(result.retainedSessions[0].code, 'SNAPSHOT_CHANGED');
  assert.doesNotMatch(await fs.readFile(path.join(f.root, '1970-01-01', `${f.id}.md`), 'utf8'), /PRIVATE/);
}));

test('unique message cursors hitting page cap never silently skip history', () => fixture(async f => {
  let pages = 0;
  const request = wire(async (e, m) => e.includes('/message?')
    ? { status: 200, body: { data: [], cursor: { previous: null, next: `opaque-${++pages}` } } } : f.request(e, m));
  assert.equal((await maintenance(f.input, { ...f.options, request })).eligible, 0);
  assert.equal(pages, 8); assert.deepEqual(await fs.readdir(f.root), []);
}));

test('newly discovered composite proof is classification-only until present in the authorized cursor snapshot', () => fixture(async f => {
  const options = legacyOptions(f);
  const input = { known: [], receipts: {}, after: null, plan: false };
  const first = await maintenance(input, options);
  assert.deepEqual(first.proven, [f.id]); assert.equal(first.eligible, 1); assert.equal(first.deleted, 0);
  assert.deepEqual(await fs.readdir(f.root), []);
  assert.equal(f.calls.filter(c => c[1] === 'DELETE').length, 0);
  // In production authorize verifies the Python-fsynced cursor, not a mere candidate ID.
  const second = await maintenance({ ...input, known: first.proven }, options);
  assert.equal(second.deleted, 1);
  const again = await maintenance({ ...input, known: first.proven }, options);
  assert.equal(again.reconciled, 1); assert.deepEqual(again.proven, [f.id]);
  assert.equal(f.calls.filter(c => c[1] === 'DELETE').length, 1);
}));
