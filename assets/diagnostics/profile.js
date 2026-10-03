// Native calls may exceed the wait budget; progress is published between evaluations.
if(api.getActiveComp()!==sample.comp || api.getSceneFilePath()!==sample.scenePath) throw new Error('profile scene changed');
var original=api.getFrame(), results=[], skipped=[], failures=[], began=Date.now(), restored=false, phase='sampling';
var cursor=sample.start, warmup=sample.chronological?{frames:0,planned:sample.warmupCount||0,ms:0,rendered:!!sample.warmupImage}:undefined;
function publish(){try{api.writeToFile(sample.progress,JSON.stringify({profile:results,warmup:warmup,phase:phase,failures:failures,skipped:skipped,complete:false,restored:restored}));}catch(e){failures.push({inspection:'progress-write',error:String(e)});}}
function expired(){return Date.now()-began>sample.ms;}
try {
 sampling: for(var i=0;i<sample.frames.length;i++) {
  if(sample.chronological) {
   phase='warmup';
   while(cursor<sample.frames[i]) {
    if(expired()) {
     skipped.push({inspection:'profile-warmup',reason:'time budget',remaining:sample.frames[i]-cursor});
     skipped.push({inspection:'profile-sample',reason:'warm-up incomplete',remaining:sample.frames.length-i});
     publish();break sampling;
    }
    var t=Date.now();
    try {
     api.setFrame(cursor);
     // setFrame alone can leave lazy simulations unevaluated: render every intervening frame.
     api.renderPNGFrame(sample.warmupImage,10);
     if(!api.filePathExists(sample.warmupImage+'.png'))throw new Error('warm-up PNG output missing');
     warmup.frames++;warmup.evaluatedThrough=cursor;cursor++;
    } catch(e) {
     failures.push({inspection:'profile-warmup',frame:cursor,error:String(e)});
     warmup.ms+=Date.now()-t;publish();break sampling;
    }
    warmup.ms+=Date.now()-t;
    if(warmup.frames%10===0)publish();
   }
  }
  phase='sampling';
  if(expired()){skipped.push({inspection:'profile-sample',reason:'time budget',remaining:sample.frames.length-i});publish();break;}
  var category=i===0?'first-evaluation-cache-state-unknown':'subsequent-evaluation';
  if(i===0 && warmup && warmup.frames)category='first-measured-after-warmup-cache-state-unknown';
  var result={frame:sample.frames[i],category:category,failures:[]},t=Date.now();
  try{
   api.setFrame(result.frame);result.setFrameMs=Date.now()-t;
   if(sample.images){t=Date.now();api.renderPNGFrame(sample.images+'/sample-'+i,10);result.renderPNGMs=Date.now()-t;if(!api.filePathExists(sample.images+'/sample-'+i+'.png'))throw new Error('PNG output missing');}
  }catch(e){result.failures.push({inspection:'profile-sample',error:String(e)});}
  results.push(result);cursor=result.frame+1;publish();
  if(result.failures.length)break;
 }
} finally {
 phase='restoring';var t=Date.now();
 try{api.setFrame(original);restored=true;}catch(e){failures.push({inspection:'playhead-restore',error:String(e)});}
 var restoreMs=Date.now()-t;publish();
}
return {profile:results,warmup:warmup,failures:failures,skipped:skipped,restored:restored,restoreMs:restoreMs};
