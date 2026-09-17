'use strict';
// Local contract tests. Docker, sudo and GitHub are fakes; HTTP uses loopback.
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const crypto = require('node:crypto');
const {spawnSync, spawn} = require('node:child_process');
const http = require('node:http');
const authorize = require('./authorize.cjs');
const root = process.env.CI_TEST_REPO || path.resolve(__dirname, '../..');
const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'schumacher-ci-test-'));
let checks = 0;
const ok = name => { checks++; console.log('PASS ' + name); };
const sha = 'a'.repeat(40), before = 'b'.repeat(40), branchHead = 'c'.repeat(40);
const oldImage = 'ghcr.io/joaovitormessias/sistema-schumacher-api@sha256:' + '1'.repeat(64);
const newImage = 'ghcr.io/joaovitormessias/sistema-schumacher-api@sha256:' + '2'.repeat(64);
const attempt = '101-1-' + sha, successor = '102-1-' + sha;
function fixture() {
  return {
    context: {eventName:'push', ref:'refs/heads/main', sha, runId:101,
      repo:{owner:'o', repo:'r'},
      payload:{before, after:sha, forced:false, created:false, deleted:false}},
    commit:{sha, parents:[{sha:before}, {sha:branchHead}]},
    pull:{number:1, merged:true, merged_at:'2026-09-15T10:00:00Z', merge_commit_sha:sha,
      base:{ref:'main', repo:{full_name:'o/r'}}, head:{sha:branchHead}},
    run:{id:101, workflow_id:1, head_sha:sha, event:'push', head_branch:'main',
      run_attempt:1, created_at:'2026-09-15T10:00:01Z'},
    history:{total_count:1, workflow_runs:[{id:101}]}
  };
}
async function auth(f) {
  let allowed;
  const github = {rest:{
    repos:{getCommit:async()=>({data:f.commit}), listPullRequestsAssociatedWithCommit(){}},
    pulls:{get:async()=>({data:f.pull})},
    actions:{getWorkflowRun:async()=>({data:f.run}), listWorkflowRuns:async args=>{
      assert.equal(args.head_sha, sha); assert.equal(args.event,'push');
      return {data:f.history};
    }}}, paginate:async()=>[f.pull]};
  await authorize({github,context:f.context,core:{setOutput(k,v){allowed=v;},notice(){}}});
  return allowed;
}
function run(file, args=[], options={}) {
  return spawnSync(file, args, {encoding:'utf8', timeout:15000, ...options});
}
function pass(result) {
  assert.ifError(result.error);
  assert.equal(result.status, 0, result.stderr + result.stdout);
}
function fail(result) {
  assert.notEqual(result.status, 0, result.stdout);
  assert(!result.error, String(result.error));
}
function yaml(file) {
  const result=run('ruby',['-rpsych','-rjson','-e',
    'puts JSON.generate(Psych.safe_load_file(ARGV[0], aliases: false))',file]);
  pass(result); return JSON.parse(result.stdout);
}
const workflow=yaml(path.join(root,'.github/workflows/publish-api-ghcr.yml'));
const deployJob=workflow.jobs['deploy-production'];
const steps=Object.fromEntries(deployJob.steps.map(s=>[s.id,s]));
const bin=path.join(temp,'bin');
fs.mkdirSync(bin);
fs.writeFileSync(path.join(bin,'sleep'),'#!/bin/sh\nexit 0\n',{mode:0o755});
const env={...process.env,PATH:bin+':'+process.env.PATH,GITHUB_ACTOR:'fixture-user'};
const stateFile=path.join(temp,'docker-state.json');
const base=path.join(temp,'transactions');
const state=()=>JSON.parse(fs.readFileSync(stateFile));
const save=x=>fs.writeFileSync(stateFile,JSON.stringify(x));
const fresh=()=>save({image:oldImage, taskImage:oldImage, updates:[], replicas:1, update:'completed',main:sha});
fresh();
// Whitelist replacement isolates paths/UID only; all transaction/verify logic is real.
for (const name of ['deploy','verify','rollback']) {
  const source=fs.readFileSync(path.join(__dirname,'schumacher-api-'+name),'utf8');
  pass(run('bash',['-n'],{input:source}));
  const sandboxed=source
    .replace('export PATH=/usr/sbin:/usr/bin:/sbin:/bin','export PATH='+bin+':/usr/sbin:/usr/bin:/sbin:/bin')
    .replaceAll('$EUID == 0','$EUID == '+process.getuid())
    .replaceAll('== 0:700','== '+process.getuid()+':700')
    .replaceAll('== 0:600','== '+process.getuid()+':600')
    .replaceAll('== 0:0:700','== '+process.getuid()+':'+process.getgid()+':700')
    .replaceAll('== 0:0:600','== '+process.getuid()+':'+process.getgid()+':600')
    .replaceAll('/var/lib/schumacher-api-deploy',base)
    .replaceAll('/usr/local/sbin/schumacher-api-',bin+'/schumacher-api-')
    .replaceAll('/usr/bin/docker',bin+'/docker')
    .replaceAll('/usr/bin/curl',bin+'/api-curl');
  fs.writeFileSync(path.join(bin,'schumacher-api-'+name),sandboxed,{mode:0o755});
}
fs.writeFileSync(path.join(bin,'docker'), '#!'+process.execPath+'\n'+
  'const fs=require("node:fs"); const file='+JSON.stringify(stateFile)+';\n'+String.raw`
const s=JSON.parse(fs.readFileSync(file)); let a=process.argv.slice(2), config;
if(a[0]==='--config'){ config=a[1]; a=a.slice(2); }
const write=()=>fs.writeFileSync(file,JSON.stringify(s));
s.calls=(s.calls||0)+1; write();
if(a[0]==='login'){
  fs.readFileSync(0,'utf8');
  if(s.loginFail)process.exit(1);
  fs.writeFileSync(config+'/config.json','{"auths":{}}');
  if(s.unsafeLogin){
    fs.unlinkSync(config+'/config.json');
    fs.symlinkSync(s.unsafeLogin,config+'/config.json');
  }
}else if(a[0]==='pull'){
  if(s.pullFail)process.exit(1);
  if(s.unsafePull){
    fs.renameSync(config,config+'-saved');
    fs.symlinkSync(s.unsafePull,config);
  }
}else if(a[0]==='service' && a[1]==='inspect'){
  const fmt=a[a.indexOf('--format')+1];
  if(s.advanceOnInspect && !fmt.includes('|')) { s.main='b'.repeat(40); write(); }
  console.log(fmt.includes('|') ? s.image+'|'+s.replicas+'|'+s.update : s.image);
}else if(a[0]==='service' && a[1]==='update'){
  s.image=a[a.indexOf('--image')+1]; s.taskImage=s.image; s.update='completed';
  s.updates.push(s.image); write();
  if(s.advanceOnUpdate) { s.main='b'.repeat(40); write(); }
  if(s.updateFail)process.exit(1); // ERROR AFTER MUTATION
}else if(a[0]==='service' && a[1]==='ps'){
  const tasks=s.tasks || [{id:'task1',status:s.taskStatus||'running',desired:'running',image:s.taskImage},
    ...(s.extraTask?[{id:'task2',status:'running',desired:'running',image:s.taskImage}]:[])];
  const filter=a.includes('--filter') ? a[a.indexOf('--filter')+1] : null;
  if(filter && filter!=='desired-state=running')process.exit(90);
  if(!s.noTasks)process.stdout.write(tasks.filter(t=>!filter||t.desired==='running').map(t=>t.id).join('\n'));
}else if(a[0]==='inspect'){
  const task=s.tasks?.find(t=>t.id===a.at(-1));
  console.log(task ? task.status+'|'+task.desired+'|'+task.image :
    (s.taskStatus || 'running')+'|running|'+s.taskImage);
}else process.exit(90);
`, {mode:0o755});
function wrapper(name,...args) {
  if(name!=='verify')args.push('fixture-user');
  return run('bash',[path.join(bin,'schumacher-api-'+name),...args],{input:'synthetic-token\n',env});
}
fs.writeFileSync(path.join(bin,'api-curl'), '#!'+process.execPath+'\n'+
  'const fs=require("node:fs"); const file='+JSON.stringify(stateFile)+', journal='+JSON.stringify(base)+';\n'+String.raw`
const args=process.argv.slice(2), input=fs.readFileSync(0,'utf8');
if(!args.includes('https://api.github.com/repos/joaovitormessias/sistema-schumacher/git/ref/heads/main') ||
   !args.includes('--disable') || !args.includes('=https') || args.includes('-L') ||
   !args.includes('--config') || !args.includes('--max-time') ||
   args.some(x=>x.includes('synthetic-token')) || !input.includes('Bearer synthetic-token')) process.exit(90);
const lock=require('node:child_process').spawnSync('flock',['-n',journal+'/lock','true']);
if(lock.status!==1)process.exit(91);
const active=fs.readFileSync(journal+'/active','utf8').trim();
if(fs.readFileSync(journal+'/attempts/'+active+'/phase','utf8').trim()!=='started')process.exit(92);
const s=JSON.parse(fs.readFileSync(file));
s.apiCalls=(s.apiCalls||0)+1;
fs.writeFileSync(file,JSON.stringify(s));
if(s.apiError)process.exit(s.apiError);
console.log(s.apiBody===undefined ?
  JSON.stringify({ref:'refs/heads/main',object:{type:'commit',sha:s.main}}) : s.apiBody);
process.stdout.write(String(s.apiStatus||200));
`,{mode:0o755});
fs.writeFileSync(path.join(bin,'sync'),'#!'+process.execPath+'\n'+
  'const fs=require("node:fs"); const file='+JSON.stringify(stateFile)+';\n'+String.raw`
const s=JSON.parse(fs.readFileSync(file));
if(s.advanceOnSync){s.main='b'.repeat(40);fs.writeFileSync(file,JSON.stringify(s));}
const r=require('node:child_process').spawnSync('/usr/bin/sync',process.argv.slice(2));
process.exit(r.status===0?0:1);
`,{mode:0o755});

const redMode=process.argv.includes('--review-red');
function rejected(result, name) {
  assert.ifError(result.error);
  if(redMode) {
    pass(result);
    console.log('RED reproduced: '+name);
  } else { fail(result); ok(name); }
}
function resetTransaction() {
  // Owned fixture directory only; never the real journal.
  fs.rmSync(base,{recursive:true,force:true}); fresh();
}
async function reviewCases() {
  const f=fixture(); f.commit.parents[1]={}; delete f.pull.head;
  if(redMode) {
    assert.equal(await auth(f),'true');
    console.log('RED reproduced: missing parent and PR head authorize production');
  } else { await assert.rejects(auth(f)); ok('missing parent/PR head fail closed'); }

  // Evaluate the current job expression, not a duplicate of its policy.
  const expression=deployJob.if.replace(/^\s*\$\{\{|\}\}\s*$/g,'')
    .replaceAll('needs.authorize-production.outputs.deploy-allowed','needs.allowed');
  const eligible=Function('github','needs','return ('+expression+');');
  for(const run_attempt of [2,3,'2']) {
    const permitted=eligible({event_name:'push',ref:'refs/heads/main',run_attempt},{allowed:'true'});
    assert.equal(permitted,redMode);
  }
  if(redMode)console.log('RED reproduced: production job eligibility with reused outputs and run_attempt > 1');
  else ok('production job eligibility with reused outputs and run_attempt > 1');
  if(!redMode)assert(eligible({event_name:'push',ref:'refs/heads/main',run_attempt:1},{allowed:'true'}));

  resetTransaction();pass(wrapper('deploy','prepare',attempt,newImage));
  save({...state(),advanceOnInspect:true});
  rejected(wrapper('deploy','apply',attempt,newImage),'main advancing inside apply current_image');
  assert.equal(state().updates.length,redMode?1:0);

  resetTransaction();pass(wrapper('deploy','prepare',attempt,newImage));
  save({...state(),updateFail:true});
  fail(wrapper('deploy','apply',attempt,newImage));
  fs.rmSync(path.join(base,'attempts',attempt),{recursive:true});
  save({...state(),updateFail:false});
  rejected(wrapper('rollback',attempt),'active points to missing attempt');
  assert.equal(state().updates.length,1);

  resetTransaction();pass(wrapper('deploy','prepare',attempt,newImage));
  save({...state(),updateFail:true});fail(wrapper('deploy','apply',attempt,newImage));
  fs.unlinkSync(path.join(base,'active'));save({...state(),updateFail:false});
  rejected(wrapper('deploy','prepare',successor,newImage),'started attempt without active blocks successor');
  assert.equal(state().updates.length,1);

  resetTransaction();pass(wrapper('deploy','prepare',attempt,newImage));
  fs.unlinkSync(path.join(base,'attempts',attempt,'previous'));
  rejected(wrapper('rollback',attempt),'partially corrupted prepared record blocks rollback');
  assert.equal(state().updates.length,0);

  resetTransaction();save({...state(),replicas:2,extraTask:true});
  rejected(wrapper('verify',oldImage),'2/2 topology rejected by verify');

  resetTransaction();pass(wrapper('deploy','prepare',attempt,newImage));
  pass(wrapper('deploy','apply',attempt,newImage));
  save({...state(),replicas:2,extraTask:true});
  rejected(wrapper('rollback',attempt),'2/2 topology rejected by rollback convergence');
  if(!redMode)assert.notEqual(fs.readFileSync(path.join(base,'attempts',attempt,'phase'),'utf8').trim(),'rolled_back');
  resetTransaction();
}
async function boundaryCases() {
  resetTransaction();pass(wrapper('deploy','prepare',attempt,newImage));
  save({...state(),advanceOnSync:true});
  fail(wrapper('deploy','apply',attempt,newImage));
  assert.equal(state().updates.length,0);
  assert.equal(state().apiCalls,1);
  ok('main advance during durable phase write refuses update');

  resetTransaction();pass(wrapper('deploy','prepare',attempt,newImage));
  const locker=spawn('bash',['-c',
    'exec 9>"$1"; flock -x 9; printf "locked\\n"; IFS= read -r release','--',base+'/lock'],{env});
  await new Promise((resolve,reject)=>{
    locker.stdout.once('data',resolve);
    locker.once('error',reject);
    locker.once('exit',()=>reject(new Error('lock holder exited before ready')));
  });
  let exited=false;
  const applying=spawn('bash',[bin+'/schumacher-api-deploy','apply',attempt,newImage,'fixture-user'],{env});
  applying.stdin.end('synthetic-token\n');
  let stdout='',stderr='';
  applying.stdout.on('data',x=>stdout+=x);applying.stderr.on('data',x=>stderr+=x);
  const done=new Promise(resolve=>applying.once('close',status=>{exited=true;resolve({status,stdout,stderr});}));
  try {
    await new Promise(resolve=>setTimeout(resolve,100));
    assert(!exited,'apply must wait for the held flock');
    save({...state(),main:before});
  } finally { locker.stdin.end('release\n'); }
  fail(await done);
  assert.equal(state().updates.length,0);
  assert.equal(state().apiCalls,1);
  ok('real flock wait precedes final main query; stale apply never mutates');

  for(const delta of [
    {apiError:28},{apiError:22},{apiStatus:301},{apiStatus:302},{apiStatus:403},{apiStatus:500},
    {apiBody:'invalid JSON'},{apiBody:'{}'},
    {apiBody:JSON.stringify({ref:'refs/heads/main',object:{type:'commit'}})},
    {apiBody:JSON.stringify({ref:'refs/heads/other',object:{type:'commit',sha}})},
    {apiBody:JSON.stringify({ref:'refs/heads/main',object:{type:'commit',sha:'short'}})}
  ]) {
    resetTransaction();pass(wrapper('deploy','prepare',attempt,newImage));save({...state(),...delta});
    const result=wrapper('deploy','apply',attempt,newImage);fail(result);
    assert(!result.stdout.includes('synthetic-token') && !result.stderr.includes('synthetic-token'));
    assert.equal(state().updates.length,0);
    assert.equal(fs.readFileSync(path.join(base,'attempts',attempt,'phase'),'utf8').trim(),'aborted');
    pass(wrapper('rollback',attempt));assert.equal(state().updates.length,0);
    for(const field of ['phase','id','parent','previous','candidate'])
      assert(!fs.readFileSync(path.join(base,'attempts',attempt,field),'utf8').includes('synthetic-token'));
    assert(!fs.existsSync(path.join(base,'attempts',attempt,'docker/config.json')));
  }
  ok('final main gate: timeout, HTTP, JSON, absent/malformed SHA fail closed without token output/storage');

  resetTransaction();pass(wrapper('deploy','prepare',attempt,newImage));
  save({...state(),advanceOnUpdate:true});
  fail(wrapper('deploy','apply',attempt,newImage));
  assert.equal(state().updates.length,1);
  assert.equal(fs.readFileSync(path.join(base,'attempts',attempt,'phase'),'utf8').trim(),'started');
  pass(wrapper('rollback',attempt));assert.equal(state().image,oldImage);
  ok('main advancing DURING update leaves started and rolls back same attempt');

  // Both mutation entrypoints use the exact same journal validator.
  for(const corrupt of [
    ()=>fs.writeFileSync(path.join(base,'attempts',attempt,'id'),successor+'\n'),
    ()=>fs.writeFileSync(path.join(base,'attempts',attempt,'parent'),attempt+'\n'),
    ()=>fs.writeFileSync(path.join(base,'attempts',attempt,'candidate'),'broken\n'),
    ()=>fs.writeFileSync(path.join(base,'attempts',attempt,'phase'),'unknown\n'),
    ()=>fs.writeFileSync(path.join(base,'attempts',attempt,'phase.tmp'),'started\n'),
    ()=>fs.writeFileSync(path.join(base,'active.tmp'),attempt+'\n'),
    ()=>fs.writeFileSync(path.join(base,'active'),successor+'\n'),
    ()=>fs.unlinkSync(path.join(base,'active')),
    ()=>fs.unlinkSync(path.join(base,'attempts',attempt,'previous'))
  ]) {
    resetTransaction();pass(wrapper('deploy','prepare',attempt,newImage));corrupt();
    fail(wrapper('rollback',attempt));fail(wrapper('deploy','prepare',successor,newImage));
    fail(wrapper('deploy','apply',attempt,newImage));assert.equal(state().updates.length,0);
  }
  resetTransaction();pass(wrapper('deploy','prepare',attempt,newImage));
  pass(wrapper('deploy','apply',attempt,newImage));
  pass(wrapper('deploy','prepare',successor,newImage));
  // Rewinding active cannot hide a newer record or authorize an older apply.
  fs.writeFileSync(path.join(base,'active'),attempt+'\n');
  fail(wrapper('deploy','apply',successor,newImage));fail(wrapper('rollback',attempt));
  assert.equal(state().updates.length,1);
  ok('journal: identity, parent cycle, orphan, corruption and ownership rewind refused by both wrappers');
  resetTransaction();
}
function shell(source, extras={}) {
  return run('bash',['-c',source],{env:{...env,...extras}});
}
function asyncShell(source, extras={}) {
  return new Promise(resolve=>{
    const child=spawn('bash',['-c',source],{env:{...env,...extras}});
    let stdout='',stderr='';
    child.stdout.on('data',x=>stdout+=x);child.stderr.on('data',x=>stderr+=x);
    child.on('close',status=>resolve({status,stdout,stderr}));
  });
}
async function thirdReviewCases() {
  const red=process.argv.includes('--third-review-red');
  const check=(result,name)=>{
    if(red){pass(result);console.log('RED reproduced: '+name);}
    else {fail(result);ok(name);}
  };
  const tasks=(status='running',desired='shutdown')=>[
    {id:'task1',status:'running',desired:'running',image:oldImage},
    {id:'task2',status,desired,image:newImage}
  ];
  resetTransaction();save({...state(),tasks:tasks()});
  check(wrapper('verify',oldImage),'old desired-shutdown/current-running task is not convergence');

  const sentinel=path.join(temp,'sentinel');
  const bytes=Buffer.from('sentinel must survive\n');
  for(const operation of ['apply','rollback']) {
    resetTransaction();pass(wrapper('deploy','prepare',attempt,newImage));
    fs.writeFileSync(sentinel,bytes);
    fs.unlinkSync(base+'/lock');fs.symlinkSync(sentinel,base+'/lock');
    check(operation==='apply'?wrapper('deploy','apply',attempt,newImage):wrapper('rollback',attempt),
      'lock symlink refused by '+operation);
    assert.deepEqual(fs.readFileSync(sentinel),red?Buffer.alloc(0):bytes);
    assert.equal(state().updates.length,red&&operation==='apply'?1:0);
  }

  const outside=path.join(temp,'outside');
  fs.mkdirSync(outside,{mode:0o700});
  const corruptions={
    'docker symlink':d=>{fs.rmSync(d,{recursive:true});fs.symlinkSync(outside,d);},
    'docker mode 0777':d=>fs.chmodSync(d,0o777),
    'config symlink':d=>{fs.unlinkSync(d+'/config.json');fs.symlinkSync(sentinel,d+'/config.json');},
    'config mode 0666':d=>fs.chmodSync(d+'/config.json',0o666)
  };
  for(const [name,corrupt] of Object.entries(corruptions)) {
    for(const operation of ['apply','rollback','successor']) {
      resetTransaction();pass(wrapper('deploy','prepare',attempt,newImage));
      // A terminal predecessor isolates credential corruption from phase gating.
      if(operation==='successor') {
        pass(wrapper('rollback',attempt));
        fs.writeFileSync(base+'/attempts/'+attempt+'/docker/config.json','{}',{mode:0o600});
      }
      fs.writeFileSync(sentinel,bytes);
      fs.writeFileSync(outside+'/config.json',bytes,{mode:0o600});
      const d=base+'/attempts/'+attempt+'/docker';corrupt(d);
      const calls=state().calls, updates=state().updates.length;
      const result=operation==='apply'?wrapper('deploy','apply',attempt,newImage):
        operation==='rollback'?wrapper('rollback',attempt):wrapper('deploy','prepare',successor,newImage);
      check(result,name+' blocks '+operation);
      if(!red) {
        assert.equal(state().calls,calls,'no Docker command before rejecting credentials');
        assert.equal(state().updates.length,updates);
        assert.deepEqual(fs.readFileSync(sentinel),bytes);
        assert.deepEqual(fs.readFileSync(outside+'/config.json'),bytes);
      }
      if(red && name==='docker symlink' && operation!=='successor')
        assert(!fs.existsSync(outside+'/config.json'),'baseline cleanup traverses directory symlink');
    }
  }

  // Store only observations/argv; stdin is checked in memory, never persisted.
  const queryLog=path.join(temp,'curl-observations');
  fs.writeFileSync(path.join(bin,'curl'),'#!'+process.execPath+'\n'+
    'const fs=require("node:fs"); const log='+JSON.stringify(queryLog)+';\n'+String.raw`
const args=process.argv.slice(2);
const stdin=args.includes('--config')?fs.readFileSync(0,'utf8'):'';
fs.appendFileSync(log,JSON.stringify({args,stdinOK:stdin==='header = "Authorization: Bearer synthetic-token"\n',
  inherited:!!(process.env.GH_TOKEN||process.env.GITHUB_TOKEN||process.env.registry_token)})+'\n');
const body=process.env.GATE_BODY || JSON.stringify({ref:'refs/heads/main',object:{type:'commit',sha:process.env.GITHUB_SHA}});
console.log(body);
if(args.includes('--write-out'))process.stdout.write(process.env.GATE_STATUS || '200');
`,{mode:0o755});
  fs.writeFileSync(path.join(bin,'sudo'),'#!'+process.execPath+'\n'+String.raw`
const args=process.argv.slice(2), input=require('node:fs').readFileSync(0,'utf8');
if(input!=='synthetic-token\n' || args.some(x=>x.includes('synthetic-token')) ||
   (!process.env.ALLOW_INHERITED_FOR_RED &&
    (process.env.GH_TOKEN || process.env.GITHUB_TOKEN || process.env.registry_token)))process.exit(90);
if(args.includes('apply') && process.env.FAIL_APPLY)process.exit(1);
`,{mode:0o755});
  const gateEnv={IMAGE_DIGEST:'sha256:'+'2'.repeat(64),ATTEMPT:attempt,GH_TOKEN:'synthetic-token',
    GITHUB_SHA:sha,GITHUB_REPOSITORY:'joaovitormessias/sistema-schumacher',
    ...(red?{ALLOW_INHERITED_FOR_RED:'1'}:{GITHUB_TOKEN:'synthetic-token',registry_token:'exported-poison'})};
  const result=shell(steps.deploy.run,gateEnv);pass(result);
  const queries=fs.readFileSync(queryLog,'utf8').trim().split('\n').map(JSON.parse);
  assert.equal(queries.length,2);
  for(const q of queries) {
    assert.equal(q.args.some(x=>x.includes('synthetic-token')),red);
    if(!red) {
      assert(q.stdinOK);assert(!q.inherited);
      assert(!q.args.some(x=>x.includes('Authorization')));
      assert(q.args.includes('=https') && q.args.includes('--max-time') && q.args.includes('--connect-timeout'));
      assert(q.args.includes('https://api.github.com/repos/joaovitormessias/sistema-schumacher/git/ref/heads/main'));
    }
  }
  assert(!result.stdout.includes('synthetic-token')&&!result.stderr.includes('synthetic-token'));
  if(red)console.log('RED reproduced: both external current_main queries expose token in curl argv');
  else {
    const traced=shell('set -x\n'+steps.deploy.run,gateEnv);pass(traced);
    assert(!traced.stdout.includes('synthetic-token')&&!traced.stderr.includes('synthetic-token'));
    fail(shell(steps.deploy.run,{...gateEnv,FAIL_APPLY:'1'}));
    for(const status of ['201','301','302','307','403','500'])
      fail(shell(steps.deploy.run,{...gateEnv,GATE_STATUS:status}));
    for(const body of ['invalid','{}',JSON.stringify({ref:'refs/heads/other',object:{type:'commit',sha}}),
      JSON.stringify({ref:'refs/heads/main',object:{type:'commit',sha:'short'}})])
      fail(shell(steps.deploy.run,{...gateEnv,GATE_BODY:body}));
    // Inspect data artifacts, excluding fixture program sources that name the synthetic token.
    for(const file of [queryLog,stateFile,sentinel,outside+'/config.json'])
      assert(!fs.readFileSync(file,'utf8').includes('synthetic-token'));
    ok('two main queries: stdin-only token, no inherited secret/argv/log/file; strict HTTP/ref/JSON/SHA');
  }
  resetTransaction();
}
async function thirdBoundaryCases() {
  const tasks=(status,desired='shutdown')=>[
    {id:'task1',status:'running',desired:'running',image:oldImage},
    {id:'task2',status,desired,image:newImage}
  ];
  for(const status of ['shutdown','complete','failed','rejected']) {
    resetTransaction();save({...state(),tasks:tasks(status)});
    pass(wrapper('verify',oldImage));
    pass(wrapper('deploy','prepare',attempt,newImage));
    pass(wrapper('rollback',attempt));
  }
  for(const status of ['new','pending','assigned','accepted','ready','preparing','starting',
    'running','orphaned','remove','unknown']) {
    resetTransaction();save({...state(),tasks:tasks(status)});
    fail(wrapper('verify',oldImage));
  }
  // Verify is also called after both mutation paths, not only before prepare.
  for(const operation of ['apply','rollback']) {
    resetTransaction();pass(wrapper('deploy','prepare',attempt,newImage));
    if(operation==='rollback')pass(wrapper('deploy','apply',attempt,newImage));
    const target=operation==='apply'?newImage:oldImage;
    save({...state(),tasks:tasks('running').map((t,i)=>i===0?{...t,image:target}:t)});
    fail(operation==='apply'?wrapper('deploy','apply',attempt,newImage):wrapper('rollback',attempt));
    assert.equal(fs.readFileSync(base+'/attempts/'+attempt+'/phase','utf8').trim(),
      operation==='apply'?'started':'rolling_back');
  }
  ok('shared convergence: terminal history allowed; additional active/uncertain tasks denied in verify/apply/rollback');

  const sentinel=path.join(temp,'boundary-sentinel');
  const bytes=Buffer.from('unchanged sentinel\n');
  const lockMutators=[
    p=>fs.chmodSync(p,0o666),
    p=>{fs.unlinkSync(p);fs.mkdirSync(p,{mode:0o600});},
    p=>{fs.unlinkSync(p);pass(run('mkfifo',[p]));},
    p=>{fs.unlinkSync(p);fs.symlinkSync(sentinel+'-missing',p);}
  ];
  for(const operation of ['prepare','apply','rollback']) for(const mutate of lockMutators) {
    resetTransaction();pass(wrapper('deploy','prepare',attempt,newImage));mutate(base+'/lock');
    const calls=state().calls;
    fail(operation==='rollback'?wrapper('rollback',attempt):
      wrapper('deploy',operation,operation==='prepare'?successor:attempt,newImage));
    assert.equal(state().calls,calls);assert(!fs.existsSync(sentinel+'-missing'));
  }
  resetTransaction();pass(wrapper('deploy','prepare',attempt,newImage));
  fs.writeFileSync(base+'/lock',bytes);
  const ino=fs.statSync(base+'/lock').ino;
  pass(wrapper('deploy','apply',attempt,newImage));pass(wrapper('rollback',attempt));
  assert.deepEqual(fs.readFileSync(base+'/lock'),bytes);
  assert.equal(fs.statSync(base+'/lock').ino,ino);
  fs.unlinkSync(base+'/lock');pass(wrapper('rollback',attempt));
  assert.equal(fs.statSync(base+'/lock').mode&0o777,0o600);
  for(const mode of [0o777,0o770,0o755]) {
    resetTransaction();pass(wrapper('deploy','prepare',attempt,newImage));
    fs.chmodSync(base,mode);
    const calls=state().calls;
    fail(wrapper('deploy','apply',attempt,newImage));fail(wrapper('rollback',attempt));
    assert.equal(state().calls,calls);
  }
  resetTransaction();
  const outer=path.join(temp,'base-target');fs.mkdirSync(outer,{mode:0o700});
  fs.symlinkSync(outer,base);
  fail(wrapper('deploy','prepare',attempt,newImage));fail(wrapper('rollback',attempt));
  assert.deepEqual(fs.readdirSync(outer),[]);
  ok('lock/base: symlink, FIFO, directory and unsafe modes refused; stable inode/bytes and exclusive creation');

  const corruptions=[
    d=>fs.rmSync(d,{recursive:true}),
    d=>{fs.rmSync(d,{recursive:true});fs.writeFileSync(d,'file',{mode:0o700});},
    d=>{fs.unlinkSync(d+'/config.json');fs.symlinkSync(sentinel+'-missing',d+'/config.json');},
    d=>{fs.unlinkSync(d+'/config.json');fs.mkdirSync(d+'/config.json',{mode:0o600});},
    d=>{fs.unlinkSync(d+'/config.json');pass(run('mkfifo',[d+'/config.json']));},
    d=>fs.writeFileSync(d+'/config.json.tmp','partial',{mode:0o600})
  ];
  for(const corrupt of corruptions) {
    resetTransaction();pass(wrapper('deploy','prepare',attempt,newImage));
    corrupt(base+'/attempts/'+attempt+'/docker');
    const calls=state().calls;
    fail(wrapper('deploy','apply',attempt,newImage));fail(wrapper('rollback',attempt));
    fail(wrapper('deploy','prepare',successor,newImage));
    assert.equal(state().calls,calls);
  }
  for(const operation of ['prepare','rollback']) {
    resetTransaction();
    if(operation==='rollback'){
      pass(wrapper('deploy','prepare',attempt,newImage));pass(wrapper('deploy','apply',attempt,newImage));
    }
    fs.writeFileSync(sentinel,bytes);
    save({...state(),unsafeLogin:sentinel});
    const updates=state().updates.length;
    fail(operation==='prepare'?wrapper('deploy','prepare',attempt,newImage):wrapper('rollback',attempt));
    assert.equal(state().updates.length,updates);
    assert.deepEqual(fs.readFileSync(sentinel),bytes);
    assert(fs.lstatSync(base+'/attempts/'+attempt+'/docker/config.json').isSymbolicLink(),
      'cleanup must refuse unsafe config produced during login');
    const calls=state().calls;
    fail(wrapper('deploy','prepare',successor,newImage));assert.equal(state().calls,calls);
  }
  const pullOutside=path.join(temp,'pull-outside');fs.mkdirSync(pullOutside,{mode:0o700});
  for(const operation of ['prepare','rollback']) {
    resetTransaction();
    if(operation==='rollback'){
      pass(wrapper('deploy','prepare',attempt,newImage));pass(wrapper('deploy','apply',attempt,newImage));
    }
    fs.writeFileSync(pullOutside+'/config.json',bytes,{mode:0o600});
    save({...state(),unsafePull:pullOutside});const updates=state().updates.length;
    fail(operation==='prepare'?wrapper('deploy','prepare',attempt,newImage):wrapper('rollback',attempt));
    assert.equal(state().updates.length,updates);
    assert.deepEqual(fs.readFileSync(pullOutside+'/config.json'),bytes);
  }
  ok('credentials: partial/type/temporary corruption blocks all entrypoints; EXIT revalidates post-login corruption');

  // UID/GID are simulated because tests never acquire root; mode/type/symlink
  // cases above use real filesystem metadata. All other stat calls stay real.
  const badStat=path.join(temp,'bad-stat');
  fs.writeFileSync(bin+'/stat','#!'+process.execPath+'\n'+
    'const fs=require("node:fs"); const config='+JSON.stringify(badStat)+';\n'+String.raw`
const args=process.argv.slice(2), bad=JSON.parse(fs.readFileSync(config));
if(args.at(-1)===bad.path)console.log(bad.value);
else {
  const r=require('node:child_process').spawnSync('/usr/bin/stat',args,{encoding:'utf8'});
  process.stdout.write(r.stdout);process.stderr.write(r.stderr);process.exit(r.status);
}
`,{mode:0o755});
  try {
    fs.writeFileSync(badStat,'{}');
    for(const suffix of ['', '/lock','/attempts/'+attempt+'/docker','/attempts/'+attempt+'/docker/config.json'])
      for(const field of ['uid','gid']) {
        resetTransaction();pass(wrapper('deploy','prepare',attempt,newImage));
        const mode=suffix.endsWith('config.json')||suffix==='/lock'?'600':'700';
        const uid=process.getuid(),gid=process.getgid();
        fs.writeFileSync(badStat,JSON.stringify({path:base+suffix,
          value:(field==='uid'?uid+1:uid)+':'+(field==='gid'?gid+1:gid)+':'+mode}));
        const calls=state().calls;
        fail(wrapper('deploy','apply',attempt,newImage));fail(wrapper('rollback',attempt));
        fail(wrapper('deploy','prepare',successor,newImage));assert.equal(state().calls,calls);
        fs.writeFileSync(badStat,'{}');
      }
  } finally { fs.unlinkSync(bin+'/stat'); }
  ok('root-only invariant: wrong owner/group blocks lock/base/credential consumers (stat metadata fake)');
  resetTransaction();
}
async function main() {
  assert.equal(await auth(fixture()),'true');
  for(const mutate of [
    f=>f.context.payload.forced=true,
    f=>delete f.context.payload.forced,
    f=>f.context.payload.after=before,
    f=>f.context.payload.before=branchHead,
    f=>f.context.payload.before='0'.repeat(40),
    f=>f.context.payload.created=true,
    f=>f.context.payload.deleted=true,
    f=>f.commit.parents=[{sha:before}],
    f=>{f.commit.parents[1]={};delete f.pull.head;},
    f=>{f.commit.parents[1]=null;delete f.pull.head;},
    f=>{f.commit.parents[1].sha='d'.repeat(39);f.pull.head.sha='d'.repeat(39);},
    f=>{f.commit.parents[1].sha='D'.repeat(40);f.pull.head.sha='D'.repeat(40);},
    f=>f.pull.merged=false,
    f=>f.pull.base.ref='other',
    f=>f.pull.merge_commit_sha=before,
    f=>f.pull.head.sha=before,
    f=>f.run.run_attempt=2,
    f=>f.run.head_sha=before,
    f=>f.run.created_at='invalid',
    f=>f.run.created_at='2026-09-15T10:05:00Z',
    f=>f.history={total_count:2,workflow_runs:[{id:101},{id:100}]},
    f=>f.history={total_count:1,workflow_runs:[{id:100}]},
    f=>f.history={total_count:0,workflow_runs:[]}
  ]) { const f=fixture();mutate(f);await assert.rejects(auth(f)); }
  const replay=fixture();replay.history={total_count:2,workflow_runs:[{id:100},{id:101}]};
  await assert.rejects(auth(replay));
  for(const event of ['workflow_dispatch','pull_request','pull_request_target']) {
    const f=fixture();f.context.eventName=event;assert.equal(await auth(f),'false');
  }
  ok('authorization: valid merge, invalid transitions, reset+replay, reruns and manual/PR denied');

  // Execute the actual gate/deploy step with fakes, asserting no wrapper call.
  const callLog=path.join(temp,'sudo-calls');
  fs.writeFileSync(path.join(bin,'sudo'),'#!/bin/sh\nprintf "%s\\n" "$*" >> "'+callLog+'"\nexit 0\n',{mode:0o755});
  fs.writeFileSync(path.join(bin,'curl'),'#!/bin/sh\ncat >/dev/null\nprintf \'{"ref":"refs/heads/main","object":{"type":"commit","sha":"%s"}}\\n200\' "$HEAD_SHA"\n',{mode:0o755});
  const gateEnv={IMAGE_DIGEST:'sha256:'+'2'.repeat(64),ATTEMPT:attempt,GH_TOKEN:'synthetic-token',
    GITHUB_SHA:sha,GITHUB_REPOSITORY:'o/r',HEAD_SHA:before};
  fail(shell(steps.deploy.run,gateEnv));
  assert(!fs.existsSync(callLog));
  pass(shell(steps.deploy.run,{...gateEnv,HEAD_SHA:sha}));
  assert.match(fs.readFileSync(callLog,'utf8'),/prepare.*@sha256:/);
  assert.match(fs.readFileSync(callLog,'utf8'),/apply.*@sha256:/);
  fs.unlinkSync(callLog);
  const queryCounter=path.join(temp,'queries');
  fs.writeFileSync(path.join(bin,'curl'),'#!/bin/sh\ncat >/dev/null\nif [ -f "'+queryCounter+'" ]; then sha="'+before+'"; else touch "'+queryCounter+'"; sha="'+sha+'"; fi\nprintf \'{"ref":"refs/heads/main","object":{"type":"commit","sha":"%s"}}\\n200\' "$sha"\n',{mode:0o755});
  fail(shell(steps.deploy.run,gateEnv));
  assert(!fs.readFileSync(callLog,'utf8').includes(' apply '));
  // Transport/JSON/schema failures cannot fall through to either wrapper call.
  for(const response of ['exit 22', "printf 'invalid-json'", "printf '{}'"]) {
    fs.unlinkSync(callLog);
    fs.writeFileSync(path.join(bin,'curl'),'#!/bin/sh\n'+response+'\n',{mode:0o755});
    fail(shell(steps.deploy.run,gateEnv));assert(!fs.existsSync(callLog));
    fs.writeFileSync(callLog,'');
  }
  ok('ordering: old run and main advancing during prepare both refuse service mutation');

  for(const image of ['sha-aaaaaaa',oldImage.replace('@sha256:',':sha-'),oldImage.slice(0,-1)]) {
    fail(wrapper('deploy','prepare',attempt,image));assert.equal(state().updates.length,0);
  }
  assert.equal(workflow.jobs['publish-api'].outputs.digest,'${{ steps.build.outputs.digest }}');
  assert.equal(deployJob.env.IMAGE_DIGEST,'${{ needs.publish-api.outputs.digest }}');
  ok('artifact: exact build digest propagation; mutable/invalid references denied');

  const hash=name=>crypto.createHash('sha256').update(fs.readFileSync(path.join(__dirname,'schumacher-api-'+name))).digest('hex');
  const preflight=steps.preflight.run
    .replace('for directory in / /usr /usr/local /usr/local/sbin; do','for directory in "'+bin+'"; do')
    .replaceAll('== 0:755','== '+process.getuid()+':755')
    .replaceAll('/usr/local/sbin/schumacher-api-',bin+'/canonical-');
  fs.chmodSync(bin,0o755);
  for(const name of ['deploy','verify','rollback']) {
    fs.copyFileSync(path.join(__dirname,'schumacher-api-'+name),path.join(bin,'canonical-'+name));
    fs.chmodSync(path.join(bin,'canonical-'+name),0o755);
  }
  const hashes={IMAGE_DIGEST:'sha256:'+'2'.repeat(64),ATTEMPT:attempt,
    DEPLOY_SHA:hash('deploy'),VERIFY_SHA:hash('verify'),ROLLBACK_SHA:hash('rollback')};
  pass(shell(preflight,hashes));
  for(const name of ['deploy','verify','rollback']) {
    const file=path.join(bin,'canonical-'+name),original=fs.readFileSync(file);
    fs.appendFileSync(file,'\n# drift\n');fail(shell(preflight,hashes));fs.writeFileSync(file,original);
  }
  fail(shell(preflight,{...hashes,IMAGE_DIGEST:'sha-aaaaaaa'}));
  fs.chmodSync(path.join(bin,'canonical-deploy'),0o777);
  fail(shell(preflight,hashes));
  fs.chmodSync(path.join(bin,'canonical-deploy'),0o755);
  ok('wrapper drift: exact file hashes required; each divergent wrapper denied');

  fail(wrapper('rollback',attempt));
  const cancelledPrepare='100-1-'+sha;
  pass(wrapper('deploy','prepare',cancelledPrepare,newImage));
  pass(wrapper('rollback',cancelledPrepare));
  pass(wrapper('rollback',cancelledPrepare));
  assert.equal(fs.readFileSync(path.join(base,'active'),'utf8').trim(),cancelledPrepare);
  assert.equal(fs.readFileSync(path.join(base,'attempts',cancelledPrepare,'phase'),'utf8').trim(),'aborted');
  pass(wrapper('deploy','prepare',attempt,newImage));
  assert.equal(fs.readFileSync(path.join(base,'attempts',attempt,'phase'),'utf8').trim(),'prepared');
  assert.deepEqual(state().updates,[]);
  assert.equal(fs.readFileSync(path.join(base,'active'),'utf8').trim(),attempt);
  fail(wrapper('deploy','prepare',attempt,newImage));
  save({...state(),updateFail:true});
  const failed=wrapper('deploy','apply',attempt,newImage);
  fail(failed);assert(!failed.stdout.includes('synthetic-token'));
  assert.equal(state().image,newImage);
  assert.equal(fs.readFileSync(path.join(base,'attempts',attempt,'phase'),'utf8').trim(),'started');
  fail(wrapper('deploy','prepare',successor,newImage));
  save({...state(),updateFail:false});
  pass(wrapper('rollback',attempt));assert.equal(state().image,oldImage);
  assert.equal(fs.readFileSync(path.join(base,'attempts',attempt,'phase'),'utf8').trim(),'rolled_back');
  assert(!fs.existsSync(path.join(base,'attempts',attempt,'docker/config.json')));
  pass(wrapper('rollback',attempt));
  const updates=state().updates.length;
  fail(wrapper('rollback','999-1-'+sha));assert.equal(state().updates.length,updates);
  pass(wrapper('deploy','prepare',successor,newImage));
  pass(wrapper('deploy','apply',successor,newImage));
  assert.equal(fs.readFileSync(path.join(base,'attempts',successor,'phase'),'utf8').trim(),'completed');
  fail(wrapper('rollback',attempt));assert.equal(state().image,newImage);
  save({...state(),updateFail:true});fail(wrapper('rollback',successor));
  assert.notEqual(fs.readFileSync(path.join(base,'attempts',successor,'phase'),'utf8').trim(),'rolled_back');
  save({...state(),updateFail:false});pass(wrapper('rollback',successor));
  // A verification failure during rollback must not record rolled_back.
  const third='103-1-'+sha;
  pass(wrapper('deploy','prepare',third,newImage));
  pass(wrapper('deploy','apply',third,newImage));
  save({...state(),taskStatus:'starting'});
  fail(wrapper('rollback',third));
  assert.equal(fs.readFileSync(path.join(base,'attempts',third,'phase'),'utf8').trim(),'rolling_back');
  fail(wrapper('deploy','prepare','105-1-'+sha,newImage));
  save({...state(),taskStatus:'running'});pass(wrapper('rollback',third));
  const fourth='104-1-'+sha;
  save({...state(),pullFail:true});
  fail(wrapper('deploy','prepare',fourth,newImage));
  const beforeRollback=state().updates.length;
  pass(wrapper('rollback',fourth));
  assert.equal(state().updates.length,beforeRollback);
  assert(!fs.existsSync(path.join(base,'attempts',fourth,'docker/config.json')));
  save({...state(),pullFail:false});
  ok('transaction: partial failure restores same attempt; stale rollback denied; failed rollback retryable');

  for(const delta of [{noTasks:true},{extraTask:true},{replicas:0},{taskStatus:'starting'},
    {taskImage:newImage},{image:newImage},{update:'paused'},{update:'updating'},
    {update:''},{update:'rollback_completed'},{replicas:3}]) {
    fresh();save({...state(),...delta});fail(wrapper('verify',oldImage));
  }
  fresh();pass(wrapper('verify',oldImage));
  save({...state(),image:oldImage.replace('@',':main@')});pass(wrapper('verify',oldImage));
  ok('convergence: service/tasks digest and replica count; pending/paused/mismatched states denied');

  assert.deepEqual(deployJob.permissions,{contents:'read',packages:'read'});
  assert(!deployJob.steps.some(s=>s.uses?.startsWith('actions/checkout')));
  assert.equal(deployJob.concurrency['cancel-in-progress'],false);
  assert.match(deployJob.if,/github.event_name == 'push'/);
  assert.match(deployJob.if,/github.run_attempt == 1/);
  assert.match(steps.rollback.if,/steps.deploy.outcome == 'failure'/);
  assert.match(steps.rollback.if,/steps.deploy.outcome == 'cancelled'/);
  assert.deepEqual(workflow.jobs['publish-api'].needs,['test-api','authorize-production']);
  const pr=yaml(path.join(root,'.github/workflows/test-api-pr.yml'));
  assert.equal(pr.jobs['test-api']['runs-on'],'ubuntu-latest');
  assert.equal(workflow.jobs['test-api']['runs-on'],'ubuntu-latest');
  assert.deepEqual(Object.keys(pr.on || pr.true),['pull_request']);
  assert(!(workflow.on || workflow.true).pull_request);
  assert.match(workflow.jobs['publish-api'].steps.find(s=>s.id==='meta').with.tags,
    /type=raw,value=main,enable={{is_default_branch}}/);
  // Validate every embedded shell block, not only the wrappers.
  for(const wf of [workflow,pr]) for(const job of Object.values(wf.jobs))
    for(const step of job.steps) if(step.run)pass(run('bash',['-n'],{input:step.run}));
  ok('workflow controls: permissions, test dependency, push-only deploy and rollback eligibility');

  fs.unlinkSync(path.join(bin,'curl'));
  const responses={health:200,ready:200};
  const server=http.createServer((req,res)=>{
    res.statusCode=responses[req.url.slice(1)] || 500;
    if(res.statusCode>=300 && res.statusCode<400)res.setHeader('Location','/health');
    res.end('fixture');
  });
  await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
  try {
    const origin='http://127.0.0.1:'+server.address().port;
    for(const id of ['health','rollback']) {
      const source=steps[id].run.replaceAll('https://api.schumachertursc.com.br',origin);
      responses.health=200;responses.ready=200;pass(await asyncShell(source,{ATTEMPT:attempt,GH_TOKEN:'synthetic-token'}));
      for(const endpoint of ['health','ready']) for(const code of [301,302,307,400,500]) {
        responses.health=200;responses.ready=200;responses[endpoint]=code;
        fail(await asyncShell(source,{ATTEMPT:attempt,GH_TOKEN:'synthetic-token'}));
      }
    }
  } finally { await new Promise(resolve=>server.close(resolve)); }
  ok('HTTP: real curl accepts only 200/200 in both gates; 301/302/307/400/500 rejected on either endpoint');
  console.log(checks+' contract groups passed; Docker/GitHub are deterministic fakes.');
}
const suite=process.argv.includes('--third-review-red') ? thirdReviewCases :
  async()=>{await reviewCases();if(!redMode){await boundaryCases();await thirdReviewCases();await thirdBoundaryCases();await main();}};
suite().catch(error=>{console.error(error);process.exitCode=1;}).finally(()=>{
  fs.rmSync(temp,{recursive:true,force:true});
});
