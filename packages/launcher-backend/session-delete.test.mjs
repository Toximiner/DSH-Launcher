import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, writeFile, readFile, lstat, symlink, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { capabilities, deleteSession, descendantTree, sessionId, transcriptDirectory } from './session-delete.mjs';

async function fixture(t) {
 const dir = await mkdtemp(join(tmpdir(),'launcher-delete-unit-'));
 t.after(()=>rm(dir,{recursive:true,force:true}));
 const root=join(dir,'sessions');await mkdir(root);
 const cwd=join(dir,'project');await mkdir(cwd);await writeFile(join(cwd,'keep.txt'),'PROJECT');
 const headers=[{id:'primary',cwd},{id:'child',cwd,origin:'subagent',parentSession:'primary'},
  {id:'grandchild',cwd,origin:'subagent',parentSession:'child'}, {id:'fork',cwd,parentSession:'primary',origin:'fork'}];
 const live=new Map();const agents=new Map();const entries=new Map();const notices=[];const removedCache=[];let pending=[];let activity=[];let flushes=0;let detaches=0;let leases=0;
 const persistence={name:'session-persistence-jsonl',root,coldLogMemo:new Map(),tracker:{writers:new Map()},
  locate:h=>({kind:'jsonl',path:join(root,'--project--',h.id,'session.v4.jsonl.zstd')}),
  async list(){const result=[];for(const h of headers)if(await this.stat(h.id))result.push({header:h});return result;},
  async stat(id){const h=headers.find(h=>h.id===id);if(!h)return undefined;try{await lstat(this.locate(h).path);return {header:h};}catch(e){if(e.code==='ENOENT')return undefined;throw e;}},
  async acquireLease(){leases++;return {release:async()=>{leases--;}};}};
 for(const h of headers){await mkdir(join(root,'--project--',h.id),{recursive:true});await writeFile(persistence.locate(h).path,'TRANSCRIPT');persistence.coldLogMemo.set(h.id,{});}
 const rows=new Map([['workspace',{sessionIds:headers.map(h=>h.id),cwd}]]);let state={workspaceIds:['workspace'],archivedSessionIds:['primary','child'],pinnedSessionIds:['fork']};
 const registry={headers:new Map(headers.map(h=>[h.id,h])),sessionPaths:new Map(),missingSessionPaths:new Set(),invalidSessionPaths:new Map(),entities:new Map(),
  enqueueOperation:async fn=>fn(),requireState:()=>state,setState:async s=>{state=s;},requireTable:()=>({get:id=>rows.get(id),update:async(id,fn)=>{const v=fn(rows.get(id));rows.set(id,v);return v;}})};
 const services={sessionPersistence:persistence,workspaceRegistry:registry,sessionProjectionCache:{requireTable:()=>({delete:async id=>{removedCache.push(id);}})},
  sessions:{get:id=>live.get(id),list:()=>[...live.values()],flush:async()=>{flushes++;},liveEntryFor:s=>s,detachEntered:s=>{live.delete(s.header.id);detaches++;}},
  agents:{get:id=>agents.get(id)},typertGateway:{getPendingRemoteEvents:()=>pending},sessionQuery:{_observations:{cache:entries}}};
 const ctx={get:key=>services[key],waterfall:async()=>activity,emit:(event,id)=>notices.push([event,id])};
 return {ctx,headers,persistence,registry,root,cwd,live,agents,rows,services,notices,removedCache,
  pending:v=>{pending=v;},activity:v=>{activity=v;},counts:()=>({flushes,detaches,leases})};
}

test('deletion rejects traversal IDs and cascades only true subagents',()=>{
 for(const id of ['','../outside','/tmp','..','.','a/b','a\\b','x\0y'])assert.throws(()=>sessionId(id));
 assert.equal(sessionId('session-uuid_1'),'session-uuid_1');
 const h=[{id:'root'},{id:'child',origin:'subagent',parentSession:'root'},{id:'fork',parentSession:'root'},{id:'grandchild',origin:'subagent',parentSession:'child'}];
 assert.deepEqual(descendantTree(h,'root').map(h=>h.id),['grandchild','child','root']);
});
test('unsupported host capabilities refuse before any writes',async t=>{
 const f=await fixture(t);f.services.sessions.detachEntered=undefined;
 assert.equal(capabilities(f.ctx).supportsPermanentDelete,false);
 assert.deepEqual(await deleteSession(f.ctx,'primary'),{deleted:false,reason:'unsupported',sessionIds:[]});
 assert.deepEqual(f.counts(),{flushes:0,detaches:0,leases:0});
 assert.ok(await f.persistence.stat('primary'));
});
test('cold tree deletion removes owned artifacts and metadata, retaining project/fork',async t=>{
 const f=await fixture(t);
 assert.equal(capabilities(f.ctx).supportsPermanentDelete,true);
 assert.deepEqual(await deleteSession(f.ctx,'primary'),{deleted:true,sessionIds:['primary','grandchild','child']});
 for(const id of ['primary','child','grandchild']){assert.equal(await f.persistence.stat(id),undefined);assert.equal(f.registry.headers.has(id),false);assert.equal(f.persistence.coldLogMemo.has(id),false);}
 assert.ok(await f.persistence.stat('fork'));assert.equal(await readFile(join(f.cwd,'keep.txt'),'utf8'),'PROJECT');
 assert.deepEqual(f.rows.get('workspace').sessionIds,['fork']);
 assert.deepEqual(f.registry.requireState().archivedSessionIds,[]);
 assert.deepEqual(f.registry.requireState().pinnedSessionIds,['fork']);
 assert.equal(f.counts().leases,0);assert.equal(f.notices.length,3);assert.equal(f.removedCache.length,3);
 assert.deepEqual(await deleteSession(f.ctx,'primary'),{deleted:false,reason:'missing',sessionIds:[]});
});
test('activity anywhere in tree refuses before flush, detach or deletion',async t=>{
 for(const mode of ['running','queue','pending','provider']){
  const f=await fixture(t);
  if(mode==='provider')f.activity([{kind:'job'}]);
  else if(mode==='pending')f.pending([{agentId:'grandchild'}]);
  else f.agents.set('child',{status:mode==='running'?'running':'idle',inbox:{nextTurn:mode==='queue'?[{}]:[],nextStep:[]},scope:{dispose:async()=>{}}});
  assert.deepEqual(await deleteSession(f.ctx,'primary'),{deleted:false,reason:'active',sessionIds:[]});
  assert.ok(await f.persistence.stat('primary'));assert.ok(await f.persistence.stat('child'));
  assert.deepEqual(f.counts(),{flushes:0,detaches:0,leases:0});
 }
});
test('symlinked project/session/artifact/lock storage is refused before live flush',async t=>{
 for(const target of ['project','session','artifact','lock']){
  const f=await fixture(t);const h=f.headers[0];f.live.set(h.id,{header:h});
  const outside=join(f.cwd,'outside');await mkdir(outside);
  const path=target==='project'?join(f.root,'--project--'):target==='session'?join(f.root,'--project--','primary'):target==='artifact'?f.persistence.locate(h).path:join(f.root,'--project--','primary','session.lock');
  await rm(path,{recursive:true,force:true});
  await symlink(target==='artifact'||target==='lock'?join(f.cwd,'keep.txt'):outside,path);
  await assert.rejects(()=>deleteSession(f.ctx,'primary'));
  assert.equal(f.counts().flushes,0);assert.equal(await readFile(join(f.cwd,'keep.txt'),'utf8'),'PROJECT');
 }
});
test('live writer is flushed, idle lifecycle disposed and native writer closed before lease',async t=>{
 const f=await fixture(t);const order=[];const h=f.headers[0];f.live.set(h.id,{header:h});
 f.persistence.tracker.writers.set(h.id,{close:async()=>{order.push('writer-close');}});
 f.agents.set(h.id,{status:'idle',inbox:{nextTurn:[],nextStep:[]},scope:{dispose:async()=>{order.push('dispose-idle');f.agents.delete(h.id);}}});
 const acquire=f.persistence.acquireLease.bind(f.persistence);
 f.persistence.acquireLease=async id=>{if(id==='primary')assert.ok(order.includes('writer-close'));order.push('lease:'+id);return acquire();};
 const result=await deleteSession(f.ctx,h.id);
 assert.equal(result.deleted,true);assert.equal(f.counts().flushes,1);assert.equal(f.counts().detaches,1);
 assert.ok(order.indexOf('dispose-idle')<order.indexOf('writer-close'));
});
test('unknown artifact layout cannot authorize recursive directory deletion',async t=>{
 const f=await fixture(t);f.persistence.locate=()=>({kind:'jsonl',path:join(f.cwd,'keep.txt')});
 await assert.rejects(()=>transcriptDirectory(f.persistence,f.headers[0]),/outside/);
 assert.equal(await readFile(join(f.cwd,'keep.txt'),'utf8'),'PROJECT');
});

test('all leases and local guards release after mutation error, preserving its identity',async t=>{
 const f=await fixture(t);const original=new Error('native projection I/O failure');const released=[];const deleting=new Set();
 f.services.sessionProjectionCache.requireTable=()=>({delete:async()=>{throw original;}});
 f.persistence.acquireLease=async id=>({release:()=>{released.push(id);if(id==='child')throw new Error('synchronous release failure');}});
 await assert.rejects(()=>deleteSession(f.ctx,'primary',deleting),error=>error===original);
 assert.deepEqual(new Set(released),new Set(['primary','child','grandchild']));
 assert.equal(deleting.size,0);
});

test('lease release failure after otherwise confirmed cleanup rejects rather than returning success',async t=>{
 const f=await fixture(t);const released=[];const deleting=new Set();
 f.persistence.acquireLease=async id=>({release:()=>{released.push(id);if(id==='child')throw new Error('release failure');}});
 await assert.rejects(()=>deleteSession(f.ctx,'primary',deleting),error=>error instanceof AggregateError && error.message==='Native deletion lease release failed');
 assert.equal(released.length,3);assert.equal(deleting.size,0);
});

test('all idle live sessions detach before asynchronous scope disposal can admit a competing prompt',async t=>{
 const f=await fixture(t);const admitted=[];const checked=[];
 for(const header of f.headers.filter(h=>h.id!=='fork')){
  f.live.set(header.id,{header});
  f.agents.set(header.id,{status:'idle',inbox:{nextTurn:[],nextStep:[]},scope:{dispose:()=>Promise.resolve().then(()=>{
   // Native retained Agent prompts append through their SessionStore attachment.
   // During this disposal continuation every member must already be detached.
   for(const id of ['primary','child','grandchild'])if(f.live.has(id))admitted.push(id);
   checked.push(header.id);f.agents.delete(header.id);
  })}});
 }
 const result=await deleteSession(f.ctx,'primary');
 assert.equal(result.deleted,true);assert.equal(checked.length,3);
 assert.deepEqual(admitted,[],'a competing prompt could still reach an attached session during disposal');
});
