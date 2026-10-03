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
