// Bounded metadata only: never changes the active comp or frame, evaluates bounds, or renders.
var began = Date.now(), failures = [], skipped = [], layers = [], edges = [], inspectedEdges = 0;
var comp = api.getActiveComp(), activeIds = api.getCompLayers(false);
var start = api.get(comp, 'startFrame'), end = api.get(comp, 'endFrame'), fps = api.get(comp, 'fps'), original = api.getFrame();
var queue = [{id: comp, path: [comp], ids: activeIds}], visited = {}, totalLayers = 0;
var scanLimit = limits.scan || 10000, compLimit = limits.comps || 64;
var byComp = null, scanComplete = true, coverageComplete = true, scannedComps = 0;
function failed(id, inspection, e) { failures.push({layer:id, inspection:inspection, error:String(e)}); }
function read(id, inspection, fn, fallback) { try { return fn(); } catch(e) { failed(id,inspection,e); return fallback; } }
function timedOut() { return Date.now()-began > limits.ms; }
function compLayers(id) {
 if (byComp === null) {
  byComp = {};
  var all = read(comp, 'scene-layers', function(){return api.getAllSceneLayers();}, null);
  if (all === null) { scanComplete = false; return null; }
  for (var s=0; s<all.length; s++) {
   if (s>=scanLimit || timedOut()) {
    skipped.push({inspection:'comp-membership',reason:s>=scanLimit?'scan limit':'time budget',count:all.length-s});
    scanComplete=false; break;
   }
   var owner=read(all[s], 'parent-comp', function(){return api.getParentComp(all[s]);}, null);
   if(owner) (byComp[owner] || (byComp[owner]=[])).push(all[s]);
  }
 }
 return byComp[id] || [];
}
while (queue.length) {
 var current=queue.shift();
 if(visited[current.id]) continue;
 if (scannedComps >= compLimit || layers.length >= limits.layers || timedOut()) {
  skipped.push({inspection:'precomp-coverage',reason:scannedComps>=compLimit?'composition limit':layers.length>=limits.layers?'layer limit':'time budget',remainingComps:queue.length+1});
  coverageComplete=false; break;
 }
 visited[current.id]=true; scannedComps++;
 var ids=current.ids || compLayers(current.id);
 if(ids===null){coverageComplete=false;continue;}
 totalLayers+=ids.length;
 for (var i=0; i<ids.length; i++) {
  if (layers.length >= limits.layers || timedOut()) {
   skipped.push({inspection:'layers',comp:current.id,reason:layers.length>=limits.layers?'layer limit':'time budget',count:ids.length-i});
   coverageComplete=false;break;
  }
  var id=ids[i], type=read(id,'type',function(){return api.getLayerType(id);},'unknown');
  var node={id:id,name:read(id,'name',function(){return api.getNiceName(id);},id),type:type,parent:read(id,'parent',function(){return api.getParent(id);},null),copies:null,comp:current.id,compPath:current.path};
  var outs=read(id,'connections',function(){return api.getOutConnectedAttributes(id)||[];},[]);
  for (var j=0;j<outs.length;j++) {
   if(timedOut()){skipped.push({inspection:'connections',layer:id,reason:'time budget'});break;}
   if(inspectedEdges>=limits.edges){skipped.push({inspection:'connections',layer:id,reason:'edge limit'});break;}
   var targets=read(id,'connections',function(){return api.getOutConnections(id,outs[j])||[];},[]);
   for(var k=0;k<targets.length;k++){
    if(timedOut()){skipped.push({inspection:'connections',layer:id,reason:'time budget'});break;}
    if(inspectedEdges++>=limits.edges){skipped.push({inspection:'connections',layer:id,reason:'edge limit'});break;}
    var t=String(targets[k]), dot=t.indexOf('.');
    if(dot<0){failed(id,'connections','unrecognized target '+t);continue;}
    edges.push({from:id,fromAttr:outs[j],to:t.slice(0,dot),toAttr:t.slice(dot+1)});
   }
  }
  if(type==='compositionReference') {
   var referenced=read(id,'precomp-reference',function(){return api.getCompFromReference(id);},null);
   if(!referenced) { coverageComplete=false; skipped.push({inspection:'precomp-coverage',layer:id,reason:'unresolved reference'}); }
   else if(current.path.indexOf(referenced)>=0) {
    coverageComplete=false;skipped.push({inspection:'precomp-coverage',layer:id,reason:'cyclic reference',comp:referenced});
   } else if(!visited[referenced]) queue.push({id:referenced,path:current.path.concat([referenced])});
  }
  if(type==='duplicator') {
   // Never evaluate connected, animated, expression-driven or unsupported copy counts.
   var gen=read(id,'distribution',function(){return api.getCurrentGeneratorType(id,'generator');},null);node.distribution=gen;
   var ins=read(id,'count-connections',function(){return api.getInConnectedAttributes(id)||[];},null);
   var anim=read(id,'count-animation',function(){return api.getAnimatedAttributes(id)||[];},null);
   var counts=gen==='gridDistribution'?['generator.count.x','generator.count.y']: /^(linearDistribution|circleDistribution|randomDistribution|roseDistribution)$/.test(gen)?['generator.count']:[];
   if(!counts.length) skipped.push({inspection:'copy-count',layer:id,reason:'unsupported distribution'});
   var dependenciesKnown=ins!==null&&anim!==null;
   var total=1,known=counts.length>0&&dependenciesKnown;
   for(var c=0;known&&c<counts.length;c++){
    var attr=counts[c];
    var expr=read(id,'count-expression',function(){return api.getAttributeExpression(id,attr);},'unknown');
    var driven=ins.concat(anim).some(function(a){return a===attr||a==='generator.count'||a==='generator';});
    if(expr||driven){known=false;break;}
    var n=read(id,'copy-count',function(){return api.get(id,attr);},null);
    if(typeof n!=='number'||n<0||!isFinite(n)){known=false;break;}total*=n;
   }
   if(known&&isFinite(total))node.copies=total;
   else if(counts.length)skipped.push({inspection:'copy-count',layer:id,reason:dependenciesKnown?'connected, animated, expression-driven or invalid count':'dependency inspection failed; count not evaluated'});
  }
  layers.push(node);
 }
}
return {start:start,end:end,fps:fps,frame:original,comp:comp,scenePath:api.getSceneFilePath(),layers:layers,edges:edges,failures:failures,skipped:skipped,totalLayers:totalLayers,activeLayers:activeIds.length,coverageComplete:coverageComplete&&scanComplete&&failures.length===0,compositions:scannedComps,ms:Date.now()-began};
