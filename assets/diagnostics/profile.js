// Native calls cannot be interrupted by the between-sample budget.
if(api.getActiveComp()!==sample.comp || api.getSceneFilePath()!==sample.scenePath) throw new Error('profile scene changed');
var original=api.getFrame(), results=[], skipped=[], failures=[], began=Date.now(), restored=false;
function publish(){try{api.writeToFile(sample.progress,JSON.stringify({profile:results,failures:failures,skipped:skipped,complete:false,restored:restored}));}catch(e){failures.push({inspection:'progress-write',error:String(e)});}}
try {
 for(var i=0;i<sample.frames.length;i++) {
  if(Date.now()-began>sample.ms){skipped.push({inspection:'profile-sample',reason:'time budget',remaining:sample.frames.length-i});break;}
  var result={frame:sample.frames[i],category:i===0?'first-evaluation-cache-state-unknown':'subsequent-evaluation',failures:[]},t=Date.now();
  try{
   api.setFrame(result.frame);result.setFrameMs=Date.now()-t;
   if(sample.images){t=Date.now();api.renderPNGFrame(sample.images+'/sample-'+i,10);result.renderPNGMs=Date.now()-t;if(!api.filePathExists(sample.images+'/sample-'+i+'.png'))throw new Error('PNG output missing');}
  }catch(e){result.failures.push({inspection:'profile-sample',error:String(e)});}
  results.push(result);publish();
  if(result.failures.length)break;
 }
} finally {
 var t=Date.now();
 try{api.setFrame(original);restored=true;}catch(e){failures.push({inspection:'playhead-restore',error:String(e)});}
 var restoreMs=Date.now()-t;publish();
}
return {profile:results,failures:failures,skipped:skipped,restored:restored,restoreMs:restoreMs};
