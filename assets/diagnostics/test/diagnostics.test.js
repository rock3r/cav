const test=require('node:test'),assert=require('node:assert/strict'),fs=require('node:fs'),path=require('node:path'),vm=require('node:vm');
const source=n=>fs.readFileSync(path.join(__dirname,'..',n+'.js'),'utf8');
function run(n,api,vars){return vm.runInNewContext('(function(){'+source(n)+'})()',Object.assign({api},vars));}
function metadata(){let changed=0;const api={getActiveComp:()=> 'comp',getCompLayers:()=>['js','dup','nested','sim'],get:(id,a)=>({startFrame:0,endFrame:20,fps:30,'generator.count':20}[a]),getFrame:()=>7,getInFrame:()=>0,getOutFrame:()=>20,getKeyframeTimes:()=>[],getLayerType:id=>({js:'javaScript',dup:'duplicator',nested:'duplicator',sim:'forgeDynamicsShape'}[id]),getNiceName:id=>id,getParent:()=> 'comp',getOutConnectedAttributes:id=>id==='js'?['output']:id==='dup'?['id']:[],getOutConnections:(id)=>id==='js'?['dup.shapePosition.x']:['nested.shapes'],getCurrentGeneratorType:()=> 'linearDistribution',getInConnectedAttributes:()=> [],getAnimatedAttributes:()=> [],getAttributeExpression:()=> '',getSceneFilePath:()=> 'scratch.cv',setFrame:()=>{changed++;throw Error('must not change frame')},getBoundingBox:()=>{throw Error('must not inspect bounds')},renderPNGFrame:()=>{throw Error('must not render')}};return {api,get changed(){return changed}};}
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

// Dead time (#117): with limits.motion the script reads the keys' values and the hidden flag, still
// without evaluating the scene.
function motionAPI(){
 const f=metadata(),keys={'js:position.x':[[0,5,0],[30,5,0],[60,9,2]],'dup:opacity':[[10,0,0],[20,100,0]]};
 const kidsOf=(id,attr)=>(keys[id+':'+attr]||[]).map((_,i)=>id+':'+attr+':'+i);
 const node=kid=>{const [id,attr,i]=kid.split(':');return keys[id+':'+attr][+i]};
 const get=f.api.get;
 Object.assign(f.api,{
  getAnimatedAttributes:id=>id==='js'?['position.x']:id==='dup'?['opacity']:[],
  getKeyframeTimes:(id,attr)=>(keys[id+':'+attr]||[]).map(k=>k[0]),
  getKeyframeIdsForAttribute:kidsOf,
  hasAttributeExpression:(id,attr)=>(id==='nested'&&attr==='scale.x')||(id==='dup'&&attr==='rotation.z'),
  get:(id,attr)=>{
   if(id.includes(':')){const k=node(id);return attr==='timeOffset'?k[0]:attr==='frame'?k[0]:{numValue:k[1],interpolation:k[2]}}
   if(attr==='hidden')return id==='sim';
   if(attr==='opacity')return id==='dup'?0:100;
   return get(id,attr);
  }
 });
 return f;
}
test('motion metadata reads key values, hidden flags and expressions without evaluating the scene',()=>{
 const f=motionAPI(),r=run('structure',f.api,{limits:{layers:10,edges:100,ms:1000,motion:true}});
 const js=r.layers.find(l=>l.id==='js'),dup=r.layers.find(l=>l.id==='dup');
 assert.equal(f.changed,0);
 assert.deepEqual(Array.from(js.motion[0].times),[0,30,60]);
 assert.deepEqual(Array.from(js.motion[0].values),[5,5,9]);
 assert.deepEqual(Array.from(js.motion[0].interp),[0,0,2]);
 assert.deepEqual(Array.from(dup.motion[0].values),[0,100]);
 assert.equal(r.layers.find(l=>l.id==='sim').hidden,true);
 assert.equal(js.hidden,false);
 assert.deepEqual(Array.from(r.layers.find(l=>l.id==='nested').expressions),['scale.x']);
 assert.deepEqual(Array.from(js.expressions),[]);
 assert.deepEqual(Array.from(dup.expressions),['rotation.z'],'rotation.z is the rotation channel');
 assert.equal(js.opacity,100);
 assert.equal(dup.opacity,0,'static opacity is read');
 assert.equal(r.motion.complete,true);
});
test('expressions are queried on every transform channel and opacity',()=>{
 const f=motionAPI(),seen=new Set();
 f.api.hasAttributeExpression=(id,attr)=>{seen.add(attr);return false};
 run('structure',f.api,{limits:{layers:10,edges:100,ms:1000,motion:true}});
 for(const a of ['position.x','position.y','position.z','scale.x','scale.y','scale.z','rotation.x','rotation.y','rotation.z','opacity'])assert.ok(seen.has(a),a);
});
test('motion metadata is absent without limits.motion',()=>{
 const f=motionAPI();
 f.api.getKeyframeIdsForAttribute=()=>{throw Error('must not read keys')};
 f.api.hasAttributeExpression=()=>{throw Error('must not query expressions')};
 const r=run('structure',f.api,{limits:{layers:10,edges:100,ms:1000}});
 assert.equal(r.motion,undefined);
 assert.ok(r.layers.every(l=>l.motion===undefined&&l.hidden===undefined));
 assert.equal(r.failures.length,0);
});
test('an attribute with many keys is reported by its first and last frame only',()=>{
 const f=motionAPI(),many=Array.from({length:200},(_,i)=>i*2);
 const times=f.api.getKeyframeTimes;
 f.api.getKeyframeTimes=(id,attr)=>id==='js'?many.slice().reverse():times(id,attr);
 const ids=f.api.getKeyframeIdsForAttribute;
 f.api.getKeyframeIdsForAttribute=(id,attr)=>{if(id==='js')throw Error('must not read 200 keys');return ids(id,attr)};
 const r=run('structure',f.api,{limits:{layers:10,edges:100,ms:1000,motion:true}});
 assert.deepEqual(Array.from(r.layers.find(l=>l.id==='js').motion[0].span),[0,398]);
 assert.equal(r.motion.complete,true);
});
// Overshoot on data (#116): the keys' eases are read from the same key data, a Custom formula is run only
// when it is a plain formula, and the cavRole user data is read.
function easeAPI(data){
 const f=motionAPI(),get=f.api.get;
 f.api.get=(id,attr)=>{
  if(id==='js:position.x:0'&&attr==='data')return Object.assign({numValue:0},data);
  return get(id,attr);
 };
 return f;
}
const limitsMotion={limits:{layers:10,edges:100,ms:1000,motion:true}};
test('a Custom ease is measured: outBack goes above 1, outCubic stays monotonic',()=>{
 const E={outBack:'1 + 2.70158*pow(x-1,3) + 1.70158*pow(x-1,2)',outCubic:'1 - pow(1-x,3)',spring:'x>=1 ? 1 : 1 - exp(-7*x)*cos(14*x)',inBack:'2.70158*x*x*x - 1.70158*x*x'};
 const read=expr=>{const f=easeAPI({easing:34,exprEasing:expr}),r=run('structure',f.api,limitsMotion);assert.equal(f.changed,0);return r.layers.find(l=>l.id==='js').motion[0].eases[0]};
 let e=read(E.outBack);assert.equal(e.magic,34);assert.ok(e.custom.max>1.05&&e.custom.min>=0);
 e=read(E.outCubic);assert.ok(e.custom.max<=1.0000001&&e.custom.mono);
 e=read(E.spring);assert.ok(e.custom.max>1.05&&!e.custom.mono);
 e=read(E.inBack);assert.ok(e.custom.min<-0.05);
});
test('a formula that is not plain is never evaluated',()=>{
 for(const bad of ['constructor.constructor("return 1")()','(function(){return 1})()','x.toString()','globalThis.api','pow(x,2)+Math.PI','1e3']){
  const f=easeAPI({easing:34,exprEasing:bad}),r=run('structure',f.api,limitsMotion);
  const e=r.layers.find(l=>l.id==='js').motion[0].eases[0];
  assert.ok(e.custom.error,bad);assert.equal(e.custom.max,undefined,bad);
 }
 const f=easeAPI({easing:34}),e=run('structure',f.api,limitsMotion).layers.find(l=>l.id==='js').motion[0].eases[0];
 assert.ok(e.custom.error,'a Custom ease with no formula');
});
test('Magic Easing numbers and Bezier handles are read; plain keys have no ease',()=>{
 let r=run('structure',easeAPI({easing:23}).api,limitsMotion),m=r.layers.find(l=>l.id==='js').motion[0];
 assert.equal(m.eases[0].magic,23);assert.equal(m.eases[1],null);
 r=run('structure',easeAPI({rightBez:{x:5,y:140},leftBez:{x:-5,y:0}}).api,limitsMotion);m=r.layers.find(l=>l.id==='js').motion[0];
 assert.deepEqual(Array.from(m.eases[0].right),[5,140]);assert.deepEqual(Array.from(m.eases[0].left),[-5,0]);
 r=run('structure',motionAPI().api,limitsMotion);
 assert.equal(r.layers.find(l=>l.id==='js').motion[0].eases,undefined);
});
test('cavRole user data is read as the layer role and a failed read is no failure',()=>{
 const f=motionAPI();
 f.api.getUserDataKey=(id,k)=>{assert.equal(k,'cavRole');if(id==='dup')throw Error('no user data');return id==='js'?'data':id==='nested'?'card':''};
 const r=run('structure',f.api,limitsMotion);
 assert.equal(r.layers.find(l=>l.id==='js').role,'data');
 assert.equal(r.layers.find(l=>l.id==='nested').role,undefined,'only data and container');
 assert.equal(r.layers.find(l=>l.id==='dup').role,undefined);
 assert.equal(r.failures.length,0);assert.equal(r.motion.complete,true);
 const q=run('structure',f.api,{limits:{layers:10,edges:100,ms:1000}});
 assert.ok(q.layers.every(l=>l.role===undefined),'no role without limits.motion');
});
test('key reading is capped, and the cap makes the motion metadata incomplete',()=>{
 const f=motionAPI();
 const r=run('structure',f.api,{limits:{layers:10,edges:100,ms:1000,motion:true,motionGets:3}});
 assert.equal(r.motion.complete,false);
 assert.ok(r.skipped.some(s=>s.inspection==='motion'));
});
test('every motion-relevant truncation makes the motion metadata incomplete',()=>{
 const base={layers:10,edges:100,ms:1000,motion:true};
 // keyframe-query cap: not every animated attribute is read
 let r=run('structure',motionAPI().api,{limits:{...base,keys:1}});
 assert.ok(r.skipped.some(s=>s.inspection==='keyframes'));
 assert.equal(r.motion.complete,false,'keyframe query cap');
 // edge cap: a driver's targets are not all known
 r=run('structure',motionAPI().api,{limits:{...base,edges:1}});
 assert.ok(r.skipped.some(s=>s.inspection==='connections'&&s.reason==='edge limit'));
 assert.equal(r.motion.complete,false,'edge cap');
 // time budget while reading connections
 const f=motionAPI();let now=0;
 r=vm.runInNewContext('(function(){'+source('structure')+'})()',{api:f.api,limits:{...base,ms:100},Date:{now:()=>(now+=10)}});
 assert.ok(r.skipped.length);
 assert.equal(r.motion.complete,false,'time budget');
 // control: no truncation
 assert.equal(run('structure',motionAPI().api,{limits:base}).motion.complete,true);
});
test('a failed key read makes the motion metadata incomplete',()=>{
 const f=motionAPI(),get=f.api.get;
 f.api.get=(id,attr)=>{if(id.includes(':')&&attr==='data')throw Error('no key data');return get(id,attr)};
 const r=run('structure',f.api,{limits:{layers:10,edges:100,ms:1000,motion:true}});
 assert.equal(r.motion.complete,false);
 assert.ok(r.failures.length);
 assert.equal(f.changed,0);
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

test('metadata reports key density, opacity boundaries and SkSL inputs without evaluating the scene',()=>{
 const f=metadata();f.api.getCompLayers=()=>['shader','shape'];f.api.getLayerType=id=>id==='shader'?'skslFilter':'basicShape';
 f.api.getAnimatedAttributes=id=>id==='shape'?['opacity','position.x']:[];
 f.api.getKeyframeTimes=()=>Array.from({length:20},(_,i)=>i*2);
 f.api.getAttrChildren=()=>['inputs.0'];f.api.getCustomAttributeName=()=> 'amount';
 const get=f.api.get;f.api.get=(id,a)=>a==='code'?'uniform float amount;':get(id,a);
 const r=run('structure',f.api,{limits:{layers:10,edges:100,keys:10,ms:1000}});
 assert.equal(r.layers[1].keyframes,40);assert.ok(r.layers[1].sampledCurves.includes('position.x'));
 assert.ok(r.layers[1].boundaries.includes(10));assert.equal(r.layers[0].uniforms[0],'amount');assert.equal(f.changed,0);
});
test('key query cap explicitly reports omitted curves',()=>{
 const f=metadata();f.api.getAnimatedAttributes=()=>['opacity','position.x'];
 const r=run('structure',f.api,{limits:{layers:10,edges:100,keys:1,ms:1000}});
 assert.ok(r.skipped.some(s=>s.inspection==='keyframes'));assert.equal(f.changed,0);
});


test('out boundaries use native exclusive getters and truncation stays explicit',()=>{
 const f=metadata();f.api.getOutFrame=()=>10;f.api.getAnimatedAttributes=()=>['opacity'];
 f.api.getKeyframeTimes=()=>Array.from({length:130},(_,i)=>i);
 const r=run('structure',f.api,{limits:{layers:10,edges:100,ms:1000}});
 assert.ok(r.layers.every(l=>l.boundaries.includes(10)));
 assert.ok(r.skipped.some(s=>s.inspection==='opacity-boundaries'&&s.count===10));
});
test('quick metadata reports text layers whose font is not installed',()=>{
  const f=metadata(),get=f.api.get,type=f.api.getLayerType;
  f.api.getCompLayers=()=>['ok','gone'];
  f.api.getLayerType=id=>['ok','gone'].includes(id)?'textShape':type(id);
  f.api.get=(id,a)=>a==='font'?{font:id==='ok'?'Inter':'Nope Sans',style:'Bold'}:get(id,a);
  const r=run('structure',f.api,{limits:{layers:10,edges:100,ms:1000},cavalry:{fontExists:(fam)=>fam==='Inter'}});
  assert.equal(r.layers[0].missingFont,undefined);
  assert.deepEqual({...r.layers[1].missingFont},{family:'Nope Sans',style:'Bold'});
  assert.equal(f.changed,0);
});
