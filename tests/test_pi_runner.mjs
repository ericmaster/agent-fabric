import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, writeFile, symlink, rm } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { createHash } from 'node:crypto';
import { pathToFileURL } from 'node:url';
import { run, scopedReadOperations } from '../hooks/retrospective/pi-runner.mjs';

const pinned = '/home/ericmaster/.npm-global/lib/node_modules/@pi-chamber/web/node_modules/@earendil-works/pi-coding-agent/dist/index.js';
const hash = x => createHash('sha256').update(x).digest('hex');
async function fixture(t) {
  const dir = await mkdtemp(path.join(os.tmpdir(), 'agf-pi-runner-'));
  t.after(() => rm(dir, { recursive: true, force: true }));
  for (const name of ['roles','sessions','workspace','agent']) await mkdir(path.join(dir,name));
  const body = '---\nname: "reviewer"\ntools: read\n---\nIndependent review.';
  const rolePath = path.join(dir,'roles/reviewer.md');
  await writeFile(rolePath,body);
  return { sdkPath:pinned,agentDir:path.join(dir,'agent'),rolesDir:path.join(dir,'roles'),sessionDir:path.join(dir,'sessions'),directory:path.join(dir,'workspace'),rolePath,role:'reviewer',roleSHA256:hash(body),model:'mock/model',sessionID:'12345678-1234-4234-8234-123456789abc',dispatchID:'incident:review',learningOrigin:'agf-learning-origin:v1:fixture',prompt:'Review bound candidate',readRoots:[path.join(dir,'workspace')],verifiedFiles:[],evidenceJSON:'[]',evidenceSHA256:hash('[]') };
}
function mockSDK(packet, events, change={}) {
  let listener,systemPrompt;
  const file = path.join(packet.sessionDir,'native.jsonl');
  const sdk = {
    VERSION:'1.0.0',
    SessionManager:{create(cwd,sessionDir,options){assert.equal(options.id,packet.sessionID);return { appendCustomEntry(){},getSessionId:()=>options.id,getSessionFile:()=>file };}},
    SettingsManager:{inMemory:value=>value},
    async createAgentSessionServices(options){
      systemPrompt=options.resourceLoaderOptions.systemPrompt;
      assert.equal(options.resourceLoaderOptions.noExtensions,true);
      assert.equal(options.resourceLoaderOptions.noContextFiles,true);
      assert.deepEqual(options.settingsManager.packages,[]);
      return {modelRuntime:{getModel:()=>({provider:'mock',id:'model'})}};
    },
    createReadToolDefinition:(cwd,options)=>({name:'read',...options}),
    async createAgentSessionFromServices(options){
      assert.deepEqual(options.tools,['read']);
      assert.equal(options.customTools[0].name,'read');
      return {session:{systemPrompt,model:{provider:'mock',id:'model',...change},getActiveToolNames:()=>['read'],subscribe(fn){listener=fn;return ()=>{};},async prompt(text,opts){assert.equal(opts.expandPromptTemplates,false);await writeFile(file,'native fixture');for(const event of events)listener(event);},async waitForIdle(){},dispose(){}}};
    },
  };
  return sdk;
}
const final = (extra={}) => ({type:'message_end',message:{role:'assistant',provider:'mock',model:'model',stopReason:'stop',content:[{type:'text',text:'accepted fixture'}],...extra}});

test('fresh assistant plus authoritative settlement required', async t => {
  const p=await fixture(t);
  const result=await run(p,mockSDK(p,[{type:'agent_start'},final(),{type:'agent_end'},{type:'agent_settled'}]));
  assert.equal(result.text,'accepted fixture');assert.equal(result.completed,true);
  for(const events of [ [final(),{type:'agent_settled'}], [{type:'agent_start'},final(),{type:'agent_end'}], [{type:'agent_start'},final({stopReason:'aborted'}),{type:'agent_settled'}], [{type:'agent_start'},final({model:'other'}),{type:'agent_settled'}] ]) {
    await assert.rejects(run(p,mockSDK(p,events)),/completed fresh/);
  }
});
test('role hash and effective model mismatch fail before prompt',async t=>{
  const p=await fixture(t);
  await assert.rejects(run({...p,roleSHA256:'wrong'},mockSDK(p,[])),/role hash/);
  await assert.rejects(run(p,mockSDK(p,[],{id:'other'})),/effective model/);
});
test('outside-root secret and symlink denied; bounded native read works',async t=>{
  const p=await fixture(t);
  const secret=path.join(p.agentDir,'secret-test-placeholder');await writeFile(secret,'NOT A REAL SECRET');
  await writeFile(path.join(p.directory,'allowed.txt'),'okay');
  await symlink(secret,path.join(p.directory,'escape'));
  const ops=await scopedReadOperations(p.readRoots);
  assert.equal((await ops.readFile(path.join(p.directory,'allowed.txt'))).toString(),'okay');
  await assert.rejects(ops.readFile(secret),/outside/);
  await assert.rejects(ops.readFile(path.join(p.directory,'escape')),/outside/);
});
test('known sensitive source paths and aliases are denied even by exact evidence grants',async t=>{
  const p=await fixture(t),placeholder='PRIVATE_CANARY';
  const ops=await scopedReadOperations(p.readRoots);
  const sensitive=['.env','.env.local','nested/.env.production','config/app.env','auth.json','models.json','credentials-backup.json','tokens.json','keys/canary.txt','secrets/canary.txt','.ssh/id_rsa','nested/server.pem','bundle.p12','bundle.pfx','private.key'];
  for(const name of sensitive){
    const file=path.join(p.directory,name);await mkdir(path.dirname(file),{recursive:true});await writeFile(file,placeholder);
    await assert.rejects(ops.readFile(file),/sensitive path denied/);
    await assert.rejects(ops.access(file),/sensitive path denied/);
  }
  const target=path.join(p.directory,'.env'),alias=path.join(p.directory,'innocent-source-alias');
  await symlink(target,alias);await assert.rejects(ops.readFile(alias),/sensitive path denied/);
  const normal=path.join(p.directory,'normal.txt');await writeFile(normal,'ordinary source');
  const sensitiveAlias=path.join(p.directory,'.env-alias');await symlink(normal,sensitiveAlias);
  await assert.rejects(ops.readFile(sensitiveAlias),/sensitive path denied/);
  await assert.rejects(ops.readFile(p.directory+'/keys/../normal.txt'),/sensitive path denied/);
  await assert.rejects(scopedReadOperations(p.readRoots,[{path:target,sha256:hash(placeholder)}]),/sensitive path denied/);
  for(const name of ['normal.txt','auth.ts','models.ts','keys.go','tokens.ts']){
    const file=path.join(p.directory,name);await writeFile(file,'ordinary source');assert.equal((await ops.readFile(file)).toString(),'ordinary source');
  }
  const report=path.join(path.dirname(p.agentDir),'verified-report.json');await writeFile(report,'{"report":"ordinary evidence"}');
  const evidence=await scopedReadOperations(p.readRoots,[{path:report,sha256:hash('{"report":"ordinary evidence"}')}]);
  assert.equal((await evidence.readFile(report)).toString(),'{"report":"ordinary evidence"}');
});
test('only host-verified cited files are readable inside private Store; fd bytes must match',async t=>{
  const p=await fixture(t),store=path.join(path.dirname(p.agentDir),'state');
  await mkdir(store);
  for(const leaf of ['reports','traces','pi-native','receipts','origins']) await mkdir(path.join(store,leaf));
  const cited=path.join(store,'reports/cited.json'),trace=path.join(store,'traces/cited.json');
  await writeFile(cited,'{"report":"fixture"}');await writeFile(trace,'{"trace":"fixture"}');
  const files=[{path:cited,sha256:hash('{"report":"fixture"}')},{path:trace,sha256:hash('{"trace":"fixture"}')}];
  const ops=await scopedReadOperations(p.readRoots,files);
  assert.equal((await ops.readFile(cited)).toString(),'{"report":"fixture"}');
  assert.equal((await ops.readFile(trace)).toString(),'{"trace":"fixture"}');
  const denied=['reports/unrelated.json','traces/unrelated.json','pi-native/journal.json','receipts/receipt.json','origins/marker','config.json'];
  for(const name of denied){const file=path.join(store,name);await writeFile(file,'NOT REAL SECRET');await assert.rejects(ops.readFile(file),/outside/);}
  const auth=path.join(p.agentDir,'auth.json');await writeFile(auth,'{"placeholder":true}');await assert.rejects(ops.readFile(auth),/sensitive path denied/);
  await symlink(auth,path.join(store,'reports/auth-link.json'));
  await assert.rejects(ops.readFile(path.join(store,'reports/auth-link.json')),/sensitive path denied/);
  await writeFile(cited,'{"report":"mutated"}');
  await assert.rejects(ops.readFile(cited),/digest changed/);
  await assert.rejects(run({...p,verifiedFiles:files,evidenceSHA256:hash('[]')},mockSDK(p,[])),/evidence packet binding/);
});
test('Go-escaped evidence JSON binds valid HTML and Unicode-separator paths before native creation',async t=>{
  const p=await fixture(t);
  const file=path.join(path.dirname(p.agentDir),'bound<>&\u2028\u2029.json');
  const body='{"report":"special path fixture"}';await writeFile(file,body);
  const verifiedFiles=[{path:file,sha256:hash(body)}];
  const evidenceJSON=JSON.stringify(verifiedFiles).replace(/[<>&\u2028\u2029]/g,c=>'\\u'+c.charCodeAt(0).toString(16).padStart(4,'0'));
  assert.notEqual(evidenceJSON,JSON.stringify(verifiedFiles));
  const packet={...p,verifiedFiles,evidenceJSON,evidenceSHA256:hash(evidenceJSON)};
  const result=await run(packet,mockSDK(packet,[{type:'agent_start'},final(),{type:'agent_settled'}]));
  assert.equal(result.completed,true);assert.equal(result.evidenceSHA256,packet.evidenceSHA256);
  const ops=await scopedReadOperations(p.readRoots,verifiedFiles);assert.equal((await ops.readFile(file)).toString(),body);
  const sdk=mockSDK(packet,[]);let created=0;
  sdk.SessionManager.create=()=>{created++;throw Error('must not create');};
  await assert.rejects(run({...packet,evidenceSHA256:hash(JSON.stringify(verifiedFiles))},sdk),/evidence packet binding/);
  await assert.rejects(run({...packet,verifiedFiles:[]},sdk),/evidence packet binding/);
  assert.equal(created,0);
});
test('installed Pi 1.0.0 import and scoped read structural probe, no provider', async t=>{
  const sdk=await import(pathToFileURL(pinned).href);
  assert.equal(sdk.VERSION,'1.0.0');
  for(const name of ['createAgentSessionServices','createAgentSessionFromServices','createReadToolDefinition']) assert.equal(typeof sdk[name],'function');
  const p=await fixture(t),ops=await scopedReadOperations(p.readRoots);
  const tool=sdk.createReadToolDefinition(p.directory,{operations:ops,autoResizeImages:false});
  assert.equal(tool.name,'read');assert.equal(typeof tool.execute,'function');
  await writeFile(path.join(p.directory,'probe.txt'),'native tool probe');
  const result=await tool.execute('probe',{path:'probe.txt'},undefined,undefined,{});
  assert.match(JSON.stringify(result),/native tool probe/);
  await assert.rejects(tool.execute('probe',{path:path.join(p.agentDir,'outside')},undefined,undefined,{}));
  // Real SDK assembly with a no-network model-runtime stub proves that the
  // custom read definition actually replaces the callable built-in read.
  const model={provider:'mock',id:'model',api:'openai-completions',reasoning:false,input:['text'],contextWindow:8192,maxTokens:512};
  const services=await sdk.createAgentSessionServices({cwd:p.directory,agentDir:p.agentDir,modelRuntime:{async refresh(){},getModel:()=>model},settingsManager:sdk.SettingsManager.inMemory({packages:[]}),resourceLoaderOptions:{systemPrompt:'Structural probe',noExtensions:true,noSkills:true,noPromptTemplates:true,noThemes:true,noContextFiles:true}});
  const native=await sdk.createAgentSessionFromServices({services,sessionManager:sdk.SessionManager.inMemory(p.directory),model,tools:['read'],customTools:[tool]});
  try {
    assert.deepEqual(native.session.getActiveToolNames(),['read']);
    assert.deepEqual(native.session.agent.state.tools.map(item=>item.name),['read']);
    const callable=native.session.agent.state.tools[0];
    const secret=path.join(p.agentDir,'secret-placeholder');await writeFile(secret,'NOT REAL');
    await assert.rejects(callable.execute('denial',{path:secret},undefined,undefined),/outside/);
    assert.match(JSON.stringify(await callable.execute('allowed',{path:'probe.txt'},undefined,undefined)),/native tool probe/);
    for(const name of ['.env','auth.json','keys/canary.txt']){
      const file=path.join(p.directory,name);await mkdir(path.dirname(file),{recursive:true});await writeFile(file,'PRIVATE_CANARY');
      await assert.rejects(callable.execute('sensitive-denial',{path:name},undefined,undefined),/sensitive path denied/);
    }
    await symlink(path.join(p.directory,'.env'),path.join(p.directory,'safe-looking-alias'));
    await assert.rejects(callable.execute('alias-denial',{path:'safe-looking-alias'},undefined,undefined),/sensitive path denied/);
  } finally { native.session.dispose(); }
});
