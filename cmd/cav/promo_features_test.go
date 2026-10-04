package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rock3r/cav/assets"
	"github.com/rock3r/cav/internal/bridge"
	"github.com/rock3r/cav/internal/config"
	"github.com/rock3r/cav/internal/operation"
)

func TestSeamPixelDifferenceAndBox(t *testing.T) {
	dir := t.TempDir()
	before := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	after := image.NewNRGBA(before.Bounds())
	after.Set(2, 1, color.NRGBA{R: 255, A: 255})
	files := []string{filepath.Join(dir, "before.png"), filepath.Join(dir, "after.png")}
	for i, im := range []image.Image{before, after} {
		f, err := os.Create(files[i])
		if err != nil {
			t.Fatal(err)
		}
		if err = png.Encode(f, im); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	r, err := compareSeam(context.Background(), files[0], files[1], 8)
	if err != nil {
		t.Fatal(err)
	}
	if r.ChangedFraction != 1.0/16 || r.Box == nil || *r.Box != [4]int{2, 1, 3, 2} {
		t.Fatalf("wrong pixel evidence: %+v", r)
	}
	r, err = compareSeam(context.Background(), files[0], files[0], 8)
	if err != nil || r.Box != nil || r.ChangedFraction != 0 {
		t.Fatalf("still frame flagged: %+v %v", r, err)
	}
}
func TestShaderKeyAndGroupRisks(t *testing.T) {
	d := structureData{Comp: "root", Layers: []structureLayer{
		{ID: "shader", Type: "skslFilter", Code: "uniform float amount;", Uniforms: []string{"amount"}},
		{ID: "group", Type: "group", Keyframes: 10001, SampledCurves: []string{"position.x"}},
		{ID: "ref", Type: "compositionReference", Reference: "sub"}, {ID: "child", Comp: "sub"},
	}, Edges: []structureEdge{{From: "shader", To: "group", ToAttr: "filters"}}}
	kinds := map[string]bool{}
	for _, f := range performanceFindings(d) {
		kinds[f.Kind] = true
		if f.Attribution != "structural-risk-not-measured" {
			t.Fatal("risk mislabelled as measurement")
		}
	}
	for _, kind := range []string{"perf-sksl", "sksl-input-redeclared", "group-filter", "perf-keyframe-volume", "perf-sampled-keys", "perf-precomp"} {
		if !kinds[kind] {
			t.Fatal("missing " + kind)
		}
	}
	if uniformDeclaration("// uniform float amount;\n/* uniform float amount; */", "amount") {
		t.Fatal("comment mislabelled")
	}
}
func TestChunkRestartRequiresReconciliationAndPreservesAttempt(t *testing.T) {
	posts := 0
	fixtureBridgeWithGet(t, func(bridge.Request) { posts++ }, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"type":"hello","bridge":"cav-bridge","protocol":1,"bridgeSession":"new"}`))
	})
	stage := t.TempDir()
	dir := filepath.Join(stage, "chunk-0001")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "partial.png"), []byte("evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &operation.Record{Schema: 1, ID: "restart-fixture", Host: config.Host(), Port: config.Port(), Session: "old", Render: &operation.Render{Stage: stage, ChunkSize: 10}, Jobs: []*operation.Job{{ID: "uncertain", Phase: "chunk-0001-render", Submission: "accepted", State: bridge.StateUnknown}}}
	if err := restartChunk(chunkRestartTestContext(t), r, false); err == nil {
		t.Fatal("restart accepted without reconciliation")
	}
	if err := restartChunk(chunkRestartTestContext(t), r, true); err != nil {
		t.Fatal(err)
	}
	if r.Session != "new" || len(r.ReconciledJobs) != 1 || r.Jobs[0].ID == "uncertain" || r.Jobs[0].Submission != "prepared" || posts != 0 {
		t.Fatalf("unsafe restart state: %+v", r)
	}
	b, err := os.ReadFile(filepath.Join(dir+"-attempt-uncertain", "partial.png"))
	if err != nil || string(b) != "evidence" {
		t.Fatal("uncertain attempt lost")
	}
	if err = restartChunk(chunkRestartTestContext(t), r, true); err == nil {
		t.Fatal("same session accepted for restart")
	}
}
func TestChunkRestartRejectsBusyBridge(t *testing.T) {
	fixtureBridgeWithGet(t, func(bridge.Request) { t.Error("unexpected submission") }, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"type":"running","bridgeSession":"new","protocol":1}`))
	})
	r := &operation.Record{Session: "old", Render: &operation.Render{ChunkSize: 10}}
	if err := restartChunk(chunkRestartTestContext(t), r, true); err == nil {
		t.Fatal("busy native job silently retried")
	}
}
func TestChunkSourceHashCannotChange(t *testing.T) {
	t.Setenv("CAV_HOME", t.TempDir())
	t.Setenv("GORACE", os.Getenv("GORACE")+" atexit_sleep_ms=0")
	p := filepath.Join(t.TempDir(), "scene.cv")
	if err := os.WriteFile(p, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a := &app{ctx: ctx, op: &operation.Record{Schema: 1, ID: "source-fixture"}}
	if err := a.bindSceneInput(p); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.bindSceneInput(p); err == nil {
		t.Fatal("changed scene accepted")
	}
}

func TestPlaybackRangeEndpoints(t *testing.T) {
	for _, spec := range []string{"0-0", "100-120"} {
		if _, err := playbackRange(spec); err != nil {
			t.Fatal(err)
		}
	}
	for _, spec := range []string{"0-10:2", "1,2", "10-1", "0-100000", "-1-5"} {
		if _, err := playbackRange(spec); err == nil {
			t.Fatal("accepted " + spec)
		}
	}
}
func TestSelectedCompRestoresOnSuccessAndError(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js unavailable")
	}
	setup := `const assert=require('node:assert/strict');let comp='root';let frames={root:17,sub:3};let moves=0;
 const api={getComps:()=>['root','sub'],getNiceName:()=> 'same',getActiveComp:()=>comp,getFrame:()=>frames[comp],setActiveComp:c=>{comp=c;moves++},setFrame:f=>{frames[comp]=f}};`
	for _, code := range []string{"api.setFrame(99);return true", "api.setFrame(99);throw Error('failed')"} {
		script := setup + `try{(function(){` + selectedCompJS("sub", code) + `})()}catch(e){assert.match(e.message,/failed/)}assert.equal(comp,'root');assert.deepEqual(frames,{root:17,sub:3});`
		if b, e := exec.Command(node, "-e", script).CombinedOutput(); e != nil {
			t.Fatalf("restoration: %s %v", b, e)
		}
	}
	script := setup + `assert.throws(()=>{(function(){` + selectedCompJS("same", "return true") + `})()},/resolve to one ID/);assert.equal(moves,0);`
	if b, e := exec.Command(node, "-e", script).CombinedOutput(); e != nil {
		t.Fatalf("ambiguity: %s %v", b, e)
	}
}
func TestShaderPrecisionAndCommaDeclarations(t *testing.T) {
	if !uniformDeclaration("uniform highp float a, amount;", "amount") {
		t.Fatal("missed comma uniform")
	}
	if uniformDeclaration("uniform float amountOther;", "amount") {
		t.Fatal("partial name")
	}
}
func TestSeamDeadlineStopsDecode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := compareSeam(ctx, "missing", "missing", 8)
	if err == nil {
		t.Fatal("missing input accepted")
	}
	reader := seamReader{ctx: ctx, r: strings.NewReader("bytes")}
	if _, err = reader.Read(make([]byte, 8)); err != context.Canceled {
		t.Fatal(err)
	}
}

func TestChronologicalPreviewEvaluatesGapsAndRestores(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js unavailable")
	}
	setup := `const assert=require('node:assert/strict');let frame=17;let evaluations=[];
 const sample={comp:'comp',scenePath:'scratch.cv',start:0,frames:[0,5,10],dir:'/scratch',scale:25,requireSaved:true};
 const api={getActiveComp:()=> 'comp',getSceneFilePath:()=> 'scratch.cv',sceneHasUnsavedChanges:()=>false,getFrame:()=>frame,setFrame:f=>{frame=f},renderPNGFrame:()=>{evaluations.push(frame)},filePathExists:()=>true};`
	script := setup + `const kept=(function(){` + chronologicalFramesJS + `})();assert.deepEqual(evaluations,[0,1,2,3,4,5,6,7,8,9,10]);assert.equal(kept.length,3);assert.equal(frame,17);`
	if b, e := exec.Command(node, "-e", script).CombinedOutput(); e != nil {
		t.Fatalf("preview: %s %v", b, e)
	}
	script = setup + `api.renderPNGFrame=()=>{throw Error('failed')};assert.throws(()=>{(function(){` + chronologicalFramesJS + `})()},/failed/);assert.equal(frame,17);`
	if b, e := exec.Command(node, "-e", script).CombinedOutput(); e != nil {
		t.Fatalf("preview failure: %s %v", b, e)
	}
	script = setup + `api.sceneHasUnsavedChanges=()=>true;assert.throws(()=>{(function(){` + chronologicalFramesJS + `})()},/unsaved/);assert.deepEqual(evaluations,[]);`
	if b, e := exec.Command(node, "-e", script).CombinedOutput(); e != nil {
		t.Fatalf("save guard: %s %v", b, e)
	}
}
func TestBridgeWarningsRemainOffline(t *testing.T) {
	fixtureBridgeWithGet(t, func(bridge.Request) { t.Error("version submitted native work") }, func(http.ResponseWriter, *http.Request) { t.Error("offline version probed bridge") })
	t.Setenv("CAV_SCRIPTS_DIR", t.TempDir())
	p := filepath.Join(config.ScriptsDir(), config.BridgeFile)
	if err := os.WriteFile(p, []byte("old bridge"), 0600); err != nil {
		t.Fatal(err)
	}
	if len(bridgeWarnings(nil)) != 1 {
		t.Fatal("installed mismatch omitted")
	}
	a := &app{}
	if err := cmdVersion(a, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, assets.BridgeJS, 0600); err != nil {
		t.Fatal(err)
	}
	if len(bridgeWarnings(map[string]any{"bridgeVersion": "old"})) != 1 {
		t.Fatal("running mismatch omitted")
	}
	if len(bridgeWarnings(map[string]any{"bridgeVersion": bridgeVersionFromJS()})) != 0 {
		t.Fatal("matching bridge warned")
	}
}

func chunkRestartTestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)
	return ctx
}
func TestChunkRestartHashingUsesRequestedBudgetBeyondProbeCap(t *testing.T) {
	mkfifo, err := exec.LookPath("mkfifo")
	if err != nil {
		t.Skip("FIFO fixture requires Unix mkfifo")
	}
	fixtureBridgeWithGet(t, func(bridge.Request) { t.Error("unexpected native submission") }, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"type":"hello","bridgeSession":"new","protocol":1}`))
	})
	fifo := filepath.Join(t.TempDir(), "slow-audio")
	if b, e := exec.Command(mkfifo, fifo).CombinedOutput(); e != nil {
		t.Fatalf("mkfifo: %s %v", b, e)
	}
	writer, err := os.OpenFile(fifo, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer writer.Close()
		timer := time.NewTimer(10500 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
			writer.Write([]byte("audio"))
		case <-ctx.Done():
		}
	}()
	digest := sha256.Sum256([]byte("audio"))
	r := &operation.Record{Schema: 1, ID: "slow-input-restart", Session: "old", Render: &operation.Render{ChunkSize: 10}, Inputs: map[string]string{fifo: hex.EncodeToString(digest[:])}}
	err = restartChunk(ctx, r, true)
	cancel()
	<-done
	if err != nil {
		t.Fatalf("requested 15s hash budget was capped to probe budget: %v", err)
	}
	if r.Session != "new" {
		t.Fatal("recovery was not rebound after input validation")
	}
}
