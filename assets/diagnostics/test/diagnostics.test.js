const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const source=n=>fs.readFileSync(path.join(__dirname,'..',n+'.js'),'utf8');
function run(n,api,vars){return vm.runInNewContext('(function(){'+source(n)+'})()',Object.assign({api},vars));}
function metadata(){let changed=0;const api={getActiveComp:()=> 'comp',getCompLayers:()=>['js','dup','nested','sim'],get:(id,a)=>({startFrame:0,endFrame:20,fps:30,'generator.count':20}[a]),getFrame:()=>7,getLayerType:id=>({js:'javaScript',dup:'duplicator',nested:'duplicator',sim:'forgeDynamicsShape'}[id]),getNiceName:id=>id,getParent:()=> 'comp',getOutConnectedAttributes:id=>id==='js'?['output']:id==='dup'?['id']:[],getOutConnections:(id)=>id==='js'?['dup.shapePosition.x']:['nested.shapes'],getCurrentGeneratorType:()=> 'linearDistribution',getInConnectedAttributes:()=> [],getAnimatedAttributes:()=> [],getAttributeExpression:()=> '',getSceneFilePath:()=> 'scratch.cv',setFrame:()=>{changed++;throw Error('must not change frame')},getBoundingBox:()=>{throw Error('must not inspect bounds')},renderPNGFrame:()=>{throw Error('must not render')}};return {api,get changed(){return changed}};}
test('quick metadata reads structure/counts without evaluation or render',()=>{const f=metadata(),r=run('structure',f.api,{limits:{layers:10,edges:100,ms:1000}});assert.equal(f.changed,0);assert.equal(r.layers[1].copies,20);assert.equal(r.edges.length,2);assert.equal(r.failures.length,0)});
test('failed and truncated metadata are explicit; driven counts are not evaluated',()=>{const f=metadata();f.api.getInConnectedAttributes=()=>['generator.count'];f.api.getNiceName=()=>{throw Error('metadata unavailable')};f.api.get=(id,a)=>{if(a==='generator.count')throw Error('count must not be evaluated');return {startFrame:0,endFrame:20,fps:30}[a]};const r=run('structure',f.api,{limits:{layers:2,edges:1,ms:1000}});assert.equal(r.layers[1].copies,null);assert.ok(r.failures.length);assert.ok(r.skipped.length)});
test('failed dependency inspections never evaluate duplicator counts',()=>{
  for(const query of ['getInConnectedAttributes','getAnimatedAttributes']){
    const f=metadata(),get=f.api.get;let countReads=0;
    f.api[query]=()=>{throw Error('dependency unavailable')};
    f.api.get=(id,attr)=>{if(attr.startsWith('generator.count'))countReads++;return get(id,attr)};
    const r=run('structure',f.api,{limits:{layers:10,edges:100,ms:1000}});
    assert.equal(countReads,0);
    assert.ok(r.layers.filter(l=>l.type==='duplicator').every(l=>l.copies===null));
    assert.ok(r.failures.length);
    assert.ok(r.skipped.some(s=>s.inspection==='copy-count'&&s.reason.includes('dependency inspection failed')));
  }
});
test('profiling is chronological, publishes samples, separates PNG time and restores once',()=>{let current=9;const frames=[],writes=[];const r=run('profile',{getActiveComp:()=> 'comp',getSceneFilePath:()=> 'scratch.cv',getFrame:()=>current,setFrame:f=>{current=f;frames.push(f)},renderPNGFrame:()=>{},filePathExists:()=>true,writeToFile:(p,v)=>writes.push(JSON.parse(v))},{sample:{comp:'comp',scenePath:'scratch.cv',frames:[0,1,2],images:'/scratch',progress:'/scratch/progress',ms:1000}});assert.deepEqual(frames,[0,1,2,9]);assert.equal(current,9);assert.equal(r.restored,true);assert.equal(r.profile.length,3);assert.ok(r.profile[0].category.includes('cache-state-unknown'));assert.equal(typeof r.profile[1].renderPNGMs,'number');assert.equal(writes[0].profile.length,1);assert.equal(writes.at(-1).restored,true)});
test('failed native sample restores the playhead and reports partial results',()=>{let current=7;const r=run('profile',{getActiveComp:()=> 'comp',getSceneFilePath:()=> 'scratch.cv',getFrame:()=>current,setFrame:f=>{current=f},renderPNGFrame:()=>{throw Error('render failed')},writeToFile:()=>{}},{sample:{comp:'comp',scenePath:'scratch.cv',frames:[0,1,2],images:'/scratch',progress:'/scratch/progress',ms:1000}});assert.equal(current,7);assert.equal(r.profile.length,1);assert.equal(r.profile[0].failures.length,1);assert.equal(r.restored,true)});
test('between-sample budget returns skipped work without changing the playhead',()=>{let changes=0;const r=run('profile',{getActiveComp:()=> 'comp',getSceneFilePath:()=> 'scratch.cv',getFrame:()=>8,setFrame:()=>{changes++},writeToFile:()=>{}},{sample:{comp:'comp',scenePath:'scratch.cv',frames:[0,1,2],progress:'/scratch/progress',ms:-1}});assert.equal(r.profile.length,0);assert.ok(r.skipped.length);assert.equal(changes,1)});

const visual=fs.readFileSync(path.join(__dirname,'../../../cmd/cav/cmd_check.go'),'utf8').match(/const checkJS = `([\s\S]*?)`/)[1];
function visualAPI(){let frame=7;const moves=[];return {moves,get frame(){return frame},api:{getFrame:()=>frame,setFrame:f=>{frame=f;moves.push(f)},getActiveComp:()=> 'comp',getSceneFilePath:()=> 'scratch.cv',getCompLayers:()=>['shape'],getLayerType:()=> 'basicShape',getNiceName:()=> 'shape',getParent:()=> 'comp',getChildren:()=>[],getInFrame:()=>0,getOutFrame:()=>20,getAnimatedAttributes:()=>[],hasFill:()=>true,getBoundingBox:()=>{throw Error('bounds unavailable')},get:(id,a)=>({startFrame:0,endFrame:20,resolution:{x:1920,y:1080},fps:30,hidden:false,opacity:100,scale:{x:1,y:1},'material.materialColor':{a:255}}[a])}};}
test('bounded visual checks report failed bounds and restore the actual original playhead',()=>{const f=visualAPI(),r=vm.runInNewContext('(function(){'+visual+'})()',{api:f.api,expected:{comp:'comp',scenePath:'scratch.cv'},limits:{layers:200,samples:3,frames:6,ms:1000}});assert.equal(f.frame,7);assert.ok(r.failures.length);assert.ok(f.moves.length<=7);assert.equal(f.moves.at(-1),7)});
test('unexpected visual errors still execute playhead restoration',()=>{const f=visualAPI();f.api.getCompLayers=()=>{throw Error('inspection failed')};assert.throws(()=>vm.runInNewContext('(function(){'+visual+'})()',{api:f.api,expected:{comp:'comp',scenePath:'scratch.cv'},limits:{layers:200,samples:3,frames:6,ms:1000}}),/inspection failed/);assert.equal(f.moves.at(-1),7)});

test('resumed visual checks reject a changed comp or scene before reading or changing the playhead',()=>{
  for (const change of ['comp','scene']) {
    const f=visualAPI();
    if(change==='comp')f.api.getActiveComp=()=> 'new-comp';
    else f.api.getSceneFilePath=()=> 'new-scene.cv';
    f.api.getFrame=()=>{throw Error('must guard before reading playhead')};
    f.api.getCompLayers=()=>{throw Error('must not inspect changed scene')};
    assert.throws(()=>vm.runInNewContext('(function(){'+visual+'})()',{
      api:f.api,expected:{comp:'comp',scenePath:'scratch.cv'},limits:{layers:200,samples:3,frames:6,ms:1000}
    }),/active scene\/comp changed/);
    assert.deepEqual(f.moves,[]);
  }
});
test('motion beyond the frame budget remains explicitly uninspected',()=>{
  const f=visualAPI(),read=f.api.get;
  f.api.getAnimatedAttributes=()=> ['position.x'];
  f.api.getKeyframeTimes=()=> [0,2,4,6,8,10,12,14,16,18,20];
  f.api.get=(id,attr)=>attr==='position.x'?f.frame:read(id,attr);
  f.api.getBoundingBox=()=> ({left:0,right:100,bottom:0,top:100,width:100,height:100});
  const r=vm.runInNewContext('(function(){'+visual+'})()',{
    api:f.api,expected:{comp:'comp',scenePath:'scratch.cv'},limits:{layers:200,samples:3,frames:6,ms:1000}
  });
  assert.equal(r.evaluations,6);
  assert.ok(r.skipped.some(s=>s.inspection==='visual-frame'));
  assert.ok(r.segs.every(s=>s[1]<20));
  assert.equal(f.frame,7);
});

function nestedMetadata(){
 const f=metadata(),types={rootRef:'compositionReference',sharedRef:'compositionReference',childRef:'compositionReference',childJS:'javaScript',childDup:'duplicator',physics:'particleShape'};
 const owners={rootRef:'comp',sharedRef:'comp',childRef:'child',childJS:'child',childDup:'child',physics:'grandchild'};
 const originalType=f.api.getLayerType;
 Object.assign(f.api,{getCompLayers:()=>['rootRef','sharedRef'],getAllSceneLayers:()=>Object.keys(owners),getParentComp:id=>owners[id],getCompFromReference:id=>id==='childRef'?'grandchild':'child',getLayerType:id=>types[id]||originalType(id),getOutConnectedAttributes:id=>id==='childJS'?['output']:[],getOutConnections:()=>['childDup.shapePosition.x']});
 return f;
}
test('metadata follows shared and nested pre-comps once without scene evaluation',()=>{
 const f=nestedMetadata(),r=run('structure',f.api,{limits:{layers:20,edges:100,scan:100,comps:10,ms:1000}});
 assert.equal(r.coverageComplete,true);assert.equal(r.compositions,3);assert.equal(r.totalLayers,6);
 assert.deepEqual(Array.from(r.layers.find(l=>l.id==='physics').compPath),['comp','child','grandchild']);
 assert.equal(r.layers.filter(l=>l.id==='childJS').length,1);
 assert.ok(r.edges.some(e=>e.from==='childJS'&&e.to==='childDup'));
 assert.equal(f.changed,0);
});
test('pre-comp scan limits, missing membership and cycles cannot claim complete coverage',()=>{
 for(const mode of ['scan','membership','cycle','layers','comps']){
  const f=nestedMetadata(),limits={layers:20,edges:100,scan:100,comps:10,ms:1000};
  if(mode==='scan')limits.scan=1;
  if(mode==='layers')limits.layers=2;
  if(mode==='comps')limits.comps=1;
  if(mode==='membership')f.api.getParentComp=()=>{throw Error('membership unavailable')};
  if(mode==='cycle')f.api.getCompFromReference=id=>id==='childRef'?'comp':'child';
  const r=run('structure',f.api,{limits});assert.equal(r.coverageComplete,false,mode);assert.ok(r.skipped.length||r.failures.length,mode);assert.equal(f.changed,0);
 }
});
function chronologicalAPI(){let current=9;const frames=[],rendered=[],writes=[];return {frames,rendered,writes,get current(){return current},api:{getActiveComp:()=> 'comp',getSceneFilePath:()=> 'scratch.cv',getFrame:()=>current,setFrame:f=>{current=f;frames.push(f)},renderPNGFrame:p=>rendered.push({frame:current,path:p}),filePathExists:()=>true,writeToFile:(p,v)=>writes.push(JSON.parse(v))}};}
const chronoSample={comp:'comp',scenePath:'scratch.cv',start:0,frames:[2,5],chronological:true,warmupCount:4,warmupImage:'/scratch/warmup',images:'/scratch',progress:'/scratch/progress',ms:1000};
test('late simulation targets render every intervening frame and separate warm-up costs',()=>{
 const f=chronologicalAPI(),r=run('profile',f.api,{sample:chronoSample});
 assert.deepEqual(f.rendered.map(x=>x.frame),[0,1,2,3,4,5]);assert.equal(f.current,9);
 assert.equal(r.warmup.frames,4);assert.equal(r.warmup.planned,4);assert.equal(r.warmup.evaluatedThrough,4);
 assert.deepEqual(Array.from(r.profile,p=>p.frame),[2,5]);assert.ok(r.profile[0].category.includes('after-warmup'));
 assert.equal(f.rendered.filter(x=>x.path==='/scratch/warmup').length,4);assert.equal(typeof r.warmup.ms,'number');
});
test('failed warm-up never measures an unwarmed target and restores once',()=>{
 const f=chronologicalAPI();f.api.renderPNGFrame=()=>{throw Error('warm-up failed')};
 const r=run('profile',f.api,{sample:chronoSample});
 assert.equal(r.profile.length,0);assert.equal(r.failures[0].inspection,'profile-warmup');assert.equal(r.warmup.frames,0);assert.deepEqual(f.frames,[0,9]);assert.equal(r.restored,true);
});
test('warm-up budget expires between evaluations and retains partial progress',()=>{
 const f=chronologicalAPI();let now=0;
 const r=vm.runInNewContext('(function(){'+source('profile')+'})()',{api:f.api,sample:{...chronoSample,ms:2},Date:{now:()=>now++}});
 assert.equal(r.profile.length,0);assert.ok(r.skipped.some(s=>s.inspection==='profile-warmup'));assert.equal(f.current,9);assert.equal(f.frames.at(-1),9);
});
test('profile rejects changed scene identity before touching the playhead',()=>{
 const f=chronologicalAPI();f.api.getSceneFilePath=()=> 'other.cv';f.api.getFrame=()=>{throw Error('must guard first')};
 assert.throws(()=>run('profile',f.api,{sample:chronoSample}),/profile scene changed/);assert.deepEqual(f.frames,[]);
});
const frameRender=fs.readFileSync(path.join(__dirname,'../../../cmd/cav/cmd_render.go'),'utf8').match(/const frameRenderJS = `([\s\S]*?)`/)[1];
test('frame exports restore after a native failure and reject changed scenes before evaluation',()=>{
 const f=chronologicalAPI();f.api.renderPNGFrame=()=>{throw Error('native render failed')};
 const sample={comp:'comp',scenePath:'scratch.cv',frames:[2,5],dir:'/scratch',scale:10};
 assert.throws(()=>vm.runInNewContext('(function(){'+frameRender+'})()',{api:f.api,sample}),/native render failed/);assert.deepEqual(f.frames,[2,9]);
 f.frames.length=0;f.api.getActiveComp=()=> 'other';
 assert.throws(()=>vm.runInNewContext('(function(){'+frameRender+'})()',{api:f.api,sample}),/active scene\/comp changed/);assert.deepEqual(f.frames,[]);
});

const openedSceneGuard=fs.readFileSync(path.join(__dirname,'../../../cmd/cav/cmd_scene.go'),'utf8').match(/const openedSceneGuardJS = `([\s\S]*?)`/)[1];
test('scene-open guard normalizes Windows separators and rejects another scene',()=>{
 const api={getSceneFilePath:()=> 'C:\\scenes\\large.cv'};
 const execute=expectedScenePath=>vm.runInNewContext('(function(){'+openedSceneGuard+';return true})()',{api,expectedScenePath});
 assert.equal(execute('C:/scenes/large.cv'),true);
 assert.throws(()=>execute('C:/scenes/other.cv'),/opened scene changed/);
});

// Cavalry defaults overwriteExisting to false and reports refusal with a boolean.
test('profile replaces existing progress through warm-up, samples and restoration',()=>{
 const f=chronologicalAPI(),files=new Map(),writes=[];
 f.api.writeToFile=(path,content,overwrite)=>{
  if(files.has(path)&&!overwrite)return false;
  files.set(path,content);writes.push(JSON.parse(content));return true;
 };
 const r=run('profile',f.api,{sample:{...chronoSample,frames:[12,15],warmupCount:14}});
 const progress=JSON.parse(files.get(chronoSample.progress));
 assert.equal(r.failures.length,0);assert.ok(writes.length>=4);
 assert.equal(writes[0].warmup.frames,10);assert.equal(progress.warmup.frames,14);
 assert.deepEqual(progress.profile.map(p=>p.frame),[12,15]);
 assert.equal(progress.restored,true);assert.equal(progress.phase,'restoring');
});
test('a refused progress write is reported without losing playhead restoration',()=>{
 const f=chronologicalAPI();f.api.writeToFile=()=>false;
 const r=run('profile',f.api,{sample:chronoSample});
 assert.equal(r.failures.filter(f=>f.inspection==='progress-write').length,1);
 assert.ok(r.failures[0].error.includes('refused'));
 assert.equal(r.restored,true);assert.equal(f.current,9);
});

test('progress can recover after a refused write without repeating the failure',()=>{
 const f=chronologicalAPI();let attempts=0,progress;
 f.api.writeToFile=(path,content)=>{
  if(++attempts<=2)return false;
  progress=JSON.parse(content);return true;
 };
 const r=run('profile',f.api,{sample:chronoSample});
 assert.ok(attempts>2);assert.equal(r.failures.filter(f=>f.inspection==='progress-write').length,1);
 assert.equal(progress.restored,true);assert.equal(progress.profile.length,2);
 assert.equal(progress.failures.length,1);
});
