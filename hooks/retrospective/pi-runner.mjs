// Explicit host-selected Pi SDK transport. No extension discovery or ambient tools.
import { constants } from 'node:fs';
import { open, realpath, stat, readFile } from 'node:fs/promises';
import path from 'node:path';
import { pathToFileURL } from 'node:url';
import { createHash } from 'node:crypto';

const hash = value => createHash('sha256').update(value).digest('hex');
const inside = (root, file) => file === root || file.startsWith(root + path.sep);
const fail = message => { throw new Error(message); };
// Filename policy, not a content scanner: keep known credential/config material
// out even when a broad source root or an exact evidence grant contains it.
const sensitivePath = file => /(^|[\\/])(?:\.env[^\\/]*|credentials[^\\/]*|id_(?:rsa|dsa|ecdsa|ed25519)[^\\/]*|(?:auth|models)\.json|\.(?:netrc|npmrc|pypirc|git-credentials)|(?:secrets?|tokens?|keys?)(?:\.(?:json|ya?ml|toml|txt|ini|config))?)(?:[\\/]|$)/i.test(file)
  || /(^|[\\/])\.(?:ssh|gnupg|aws|azure|kube|docker)(?:[\\/]|$)/i.test(file)
  || /\.(?:env|pem|p12|pfx|key|jks|keystore)$/i.test(file);
const denySensitivePath = file => { if (sensitivePath(file)) fail('sensitive path denied'); };

// Open the resolved inode without following a final symlink; compare identities
// after open so a renamed ancestor cannot redirect this read outside the roots.
export async function scopedReadOperations(roots, verifiedFiles = []) {
  const allowed = await Promise.all(roots.map(root => realpath(root)));
  if (!allowed.length || allowed.includes('/')) fail('explicit bounded read roots required');
  if (!Array.isArray(verifiedFiles) || verifiedFiles.length > 128) fail('bounded verified evidence required');
  const exact = new Map();
  for (const file of verifiedFiles) {
    denySensitivePath(file.path ?? '');
    if (!path.isAbsolute(file.path ?? '') || path.normalize(file.path) !== file.path || !/^[0-9a-f]{64}$/.test(file.sha256 ?? '') || await realpath(file.path) !== file.path) fail('invalid exact evidence binding');
    if (exact.has(file.path) && exact.get(file.path) !== file.sha256) fail('conflicting evidence digests');
    exact.set(file.path, file.sha256);
  }
  const permitted = file => allowed.some(root => inside(root, file)) || exact.has(file);
  async function checked(file) {
    denySensitivePath(file);
    const resolved = await realpath(file);
    denySensitivePath(resolved);
    if (!permitted(resolved)) fail('read outside permitted roots');
    const before = await stat(resolved);
    if (!before.isFile() || before.size > 4 * 1024 * 1024) fail('read requires bounded regular file');
    const handle = await open(resolved, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
    try {
      const current = await handle.stat();
      const actual = await realpath(`/proc/self/fd/${handle.fd}`);
      denySensitivePath(actual);
      if (!permitted(actual) || actual !== resolved || current.dev !== before.dev || current.ino !== before.ino) fail('read identity changed');
      const buffer = Buffer.alloc(4 * 1024 * 1024 + 1);
      let length = 0;
      while (length < buffer.length) {
        const chunk = await handle.read(buffer, length, buffer.length - length, null);
        if (!chunk.bytesRead) break;
        length += chunk.bytesRead;
      }
      if (length > 4 * 1024 * 1024) fail('read exceeded limit');
      const bytes = buffer.subarray(0, length);
      if (exact.has(actual) && hash(bytes) !== exact.get(actual)) fail('verified evidence digest changed');
      return bytes;
    } finally { await handle.close(); }
  }
  return { readFile: checked, access: async file => { await checked(file); }, detectImageMimeType: async () => null };
}

export async function run(packet, sdk) {
  for (const key of ['sdkPath', 'agentDir', 'rolesDir', 'sessionDir', 'directory', 'rolePath']) {
    if (!path.isAbsolute(packet[key] ?? '')) fail(`absolute ${key} required`);
  }
  if (!/^[a-z][a-z0-9-]*$/.test(packet.role) || !/^[0-9a-f-]{36}$/.test(packet.sessionID)) fail('invalid role/session identity');
  const rolePath = await realpath(packet.rolePath);
  const rolesDir = await realpath(packet.rolesDir);
  if (rolePath !== path.join(rolesDir, `${packet.role}.md`)) fail('role path mismatch');
  const roleFile = await readFile(rolePath, 'utf8');
  if (hash(roleFile) !== packet.roleSHA256) fail('role hash mismatch');
  const parsed = /^---\r?\n([\s\S]*?)\r?\n---\r?\n([\s\S]+)$/.exec(roleFile);
  if (!parsed || !new RegExp(`^name: ["']?${packet.role}["']?\\s*$`, 'm').test(parsed[1])) fail('rendered role name mismatch');
  if (!/^tools:.*\bread\b/m.test(parsed[1])) fail('rendered role does not authorize read');
  if (!packet.learningOrigin?.startsWith('agf-learning-origin:v1:')) fail('learning origin required');
  const parts = packet.model.split('/');
  const provider = parts.shift(), modelID = parts.join('/');
  if (!provider || !modelID) fail('explicit provider/model required');
  sdk ??= await import(pathToFileURL(packet.sdkPath).href);
  if (sdk.VERSION !== '1.0.0') fail('unsupported Pi SDK version');
  // Go owns the exact encoded byte string; Go and JS escape HTML/separators
  // differently. Check its digest AND its parsed agreement with the file list.
  if (typeof packet.evidenceJSON !== 'string' || hash(packet.evidenceJSON) !== packet.evidenceSHA256 || JSON.stringify(JSON.parse(packet.evidenceJSON)) !== JSON.stringify(packet.verifiedFiles)) fail('evidence packet binding mismatch');
  const operations = await scopedReadOperations(packet.readRoots, packet.verifiedFiles);
  const tools = ['read'];
  const binding = { role: packet.role, roleSHA256: packet.roleSHA256, model: packet.model, tools, directory: packet.directory, dispatchID: packet.dispatchID, sessionID: packet.sessionID, promptSHA256: hash(packet.prompt), runtimeSHA256: packet.runtimeSHA256, evidenceSHA256: packet.evidenceSHA256 };
  // Go reconciles lost results from completed native history; never re-post here.
  const manager = sdk.SessionManager.create(packet.directory, packet.sessionDir, { id: packet.sessionID });
  manager.appendCustomEntry('agf-learning-origin', { marker: packet.learningOrigin });
  manager.appendCustomEntry('agf.native-role.v1', binding);
  const services = await sdk.createAgentSessionServices({
    cwd: packet.directory, agentDir: packet.agentDir,
    settingsManager: sdk.SettingsManager.inMemory({ packages: [], extensions: [], skills: [], promptTemplates: [], themes: [], compaction: { enabled: false }, retry: { enabled: false } }),
    resourceLoaderOptions: { systemPrompt: parsed[2] + '\n\nHost tool restriction: only scoped read is available. No shell, writes, delegation or ambient discovery.', noExtensions: true, noSkills: true, noPromptTemplates: true, noThemes: true, noContextFiles: true },
  });
  let session;
  try {
    const model = services.modelRuntime.getModel(provider, modelID);
    if (!model || model.provider !== provider || model.id !== modelID) fail('configured model unavailable or mismatched');
    const result = await sdk.createAgentSessionFromServices({ services, sessionManager: manager, model, tools, customTools: [sdk.createReadToolDefinition(packet.directory, { operations, autoResizeImages: false })] });
    session = result.session;
    if (result.modelFallbackMessage || session.model?.provider !== provider || session.model?.id !== modelID || JSON.stringify(session.getActiveToolNames()) !== JSON.stringify(tools)) fail('effective model/tools mismatch');
    if (!session.systemPrompt?.includes(parsed[2])) fail('effective role body mismatch');
    let started = false, settled = false, final;
    const unsubscribe = session.subscribe(event => {
      if (event.type === 'agent_start') { started = true; settled = false; final = undefined; }
      if (started && event.type === 'message_end' && event.message.role === 'assistant') final = event.message;
      if (started && event.type === 'agent_settled') settled = true;
    });
    try {
      await session.prompt(packet.prompt, { expandPromptTemplates: false, source: 'rpc' });
      await session.waitForIdle();
    } finally { unsubscribe(); }
    if (!started || !settled || !final || final.stopReason !== 'stop' || final.provider !== provider || final.model !== modelID) fail('no completed fresh assistant result');
    const text = final.content.filter(part => part.type === 'text').map(part => part.text).join('\n');
    if (!text.trim()) fail('empty assistant result');
    const output = { ...binding, sessionPath: manager.getSessionFile(), completed: true, text };
    manager.appendCustomEntry('agf.native-completed.v1', { ...binding, textSHA256: hash(text) });
    if (manager.getSessionId() !== packet.sessionID || !output.sessionPath || !inside(await realpath(packet.sessionDir), await realpath(output.sessionPath))) fail('native session identity/path mismatch');
    return output;
  } finally { session?.dispose(); }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  process.umask(0o077);
  try {
    let input = '';
    for await (const chunk of process.stdin) { input += chunk; if (input.length > 8 * 1024 * 1024) fail('packet too large'); }
    const output = await run(JSON.parse(input));
    process.stdout.write(JSON.stringify(output));
  } catch { process.stderr.write('Pi native execution incomplete; inspect protected native history.\n'); process.exitCode = 1; }
}
