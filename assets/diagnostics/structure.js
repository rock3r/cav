// Cheap, bounded metadata pass. No frame changes, bounds or render calls.
var began = Date.now(), failures = [], skipped = [], layers = [], edges = [], inspectedEdges = 0;
var comp = api.getActiveComp(), ids = api.getCompLayers(false);
function failed(id, inspection, e) { failures.push({layer:id, inspection:inspection, error:String(e)}); }
function read(id, inspection, fn, fallback) { try { return fn(); } catch(e) { failed(id,inspection,e); return fallback; } }
var start = api.get(comp,'startFrame'), end = api.get(comp,'endFrame'), fps = api.get(comp,'fps'), original = api.getFrame();
if (ids.length > limits.layers) skipped.push({inspection:'layers', reason:'layer limit', count:ids.length-limits.layers});
for (var i=0; i<Math.min(ids.length,limits.layers); i++) {
 if (Date.now()-began > limits.ms) {skipped.push({inspection:'layers',reason:'time budget',count:Math.min(ids.length,limits.layers)-i});break;}
 var id=ids[i], type=read(id,'type',function(){return api.getLayerType(id);},'unknown');
 var node={id:id,name:read(id,'name',function(){return api.getNiceName(id);},id),type:type,parent:read(id,'parent',function(){return api.getParent(id);},null),copies:null};
 var outs=read(id,'connections',function(){return api.getOutConnectedAttributes(id)||[];},[]);
 for (var j=0;j<outs.length;j++) {
  if(inspectedEdges>=limits.edges){skipped.push({inspection:'connections',layer:id,reason:'edge limit'});break;}
  var targets=read(id,'connections',function(){return api.getOutConnections(id,outs[j])||[];},[]);
  for(var k=0;k<targets.length;k++){
   if(inspectedEdges++>=limits.edges){skipped.push({inspection:'connections',layer:id,reason:'edge limit'});break;}
   var t=String(targets[k]), dot=t.indexOf('.');
   if(dot<0){failed(id,'connections','unrecognized target '+t);continue;}
   edges.push({from:id,fromAttr:outs[j],to:t.slice(0,dot),toAttr:t.slice(dot+1)});
  }
 }
 if(type==='duplicator') {
  // Read counts only on recognized distributions with no connected, animated or expression counts.
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
return {start:start,end:end,fps:fps,frame:original,comp:comp,scenePath:api.getSceneFilePath(),layers:layers,edges:edges,failures:failures,skipped:skipped,totalLayers:ids.length,ms:Date.now()-began};
