package main

import (
	"strings"
	"testing"
)

// eased is an attribute of two keys, 0 then 100, whose first key has the ease e.
func eased(attr string, e *keyEase) motionAttr {
	m := keyed([]float64{10, 40}, []float64{0, 100})
	m.Attr = attr
	m.Eases = []*keyEase{e, nil}
	return m
}

func custom(min, max float64, mono bool) *keyEase {
	return &keyEase{Magic: easeCustom, Custom: &customEase{Min: min, Max: max, Mono: mono}}
}

func TestEaseOvershoots(t *testing.T) {
	bez := func(right, left float64) (*keyEase, *keyEase) {
		return &keyEase{Right: []float64{5, right}, Left: []float64{-5, 0}}, &keyEase{Left: []float64{-5, left}, Right: []float64{5, 0}}
	}
	for magic, name := range magicEases {
		if ok, why := easeOvershoots(&keyEase{Magic: magic}, nil, fp(0), fp(1)); !ok || why != name {
			t.Errorf("magic %d: %v %q", magic, ok, why)
		}
	}
	for _, magic := range []int{1, 2, 3, 10, 11, 12} {
		if ok, _ := easeOvershoots(&keyEase{Magic: magic}, nil, fp(0), fp(1)); ok {
			t.Errorf("magic %d is monotonic", magic)
		}
	}
	inside0, inside1 := bez(40, -40)
	out0, out1 := bez(140, 0)
	back0, back1 := bez(-30, 0)
	late0, late1 := bez(0, 60)
	down0, down1 := bez(-40, 40)
	tests := []struct {
		name   string
		e0, e1 *keyEase
		v0, v1 *float64
		want   bool
	}{
		{"nil ease is linear", nil, nil, fp(0), fp(100), false},
		{"custom inside 0..1", custom(0, 1, true), nil, fp(0), fp(100), false},
		{"custom that reaches 1.1", custom(0, 1.1, false), nil, fp(0), fp(100), true},
		{"custom that dips below 0 first", custom(-0.1, 1, false), nil, fp(0), fp(100), true},
		{"custom that is not monotonic", custom(0, 1, false), nil, fp(0), fp(100), true},
		{"custom that was not evaluated", &keyEase{Magic: easeCustom, Custom: &customEase{Error: "x"}}, nil, fp(0), fp(100), false},
		{"bezier with handles inside", inside0, inside1, fp(0), fp(100), false},
		{"bezier whose first handle is past the end", out0, out1, fp(0), fp(100), true},
		{"bezier that starts the wrong way", back0, back1, fp(0), fp(100), true},
		{"bezier whose end handle is above the end", late0, late1, fp(0), fp(100), true},
		{"bezier going down inside", down0, down1, fp(100), fp(0), false},
		{"bezier with unknown values", out0, out1, nil, fp(100), false},
		{"bezier hump between equal values", out0, out1, fp(5), fp(5), true},
	}
	for _, tc := range tests {
		if got, _ := easeOvershoots(tc.e0, tc.e1, tc.v0, tc.v1); got != tc.want {
			t.Errorf("%s: got %v", tc.name, got)
		}
	}
}

func TestOvershootFindings(t *testing.T) {
	spring := &keyEase{Magic: 23}
	textShape := structureLayer{ID: "txt", Type: "textShape", Name: "Headline"}
	notRead := &keyEase{Magic: easeCustom, Custom: &customEase{Error: "not a plain formula"}}
	step := eased("scale.x", spring)
	step.Interp = []int{interpStep, 0}
	hold := keyed([]float64{10, 40}, []float64{5, 5})
	hold.Attr, hold.Eases = "scale.x", []*keyEase{spring, nil}
	tests := []struct {
		name    string
		layers  []structureLayer
		edges   []structureEdge
		want    int
		role    string
		skipped int
	}{
		{"a progress bar scaled with a spring", []structureLayer{{ID: "a", Name: "progress bar", Motion: []motionAttr{eased("scale.x", spring)}}}, nil, 1, "name", 0},
		{"the same bar with a plain ease", []structureLayer{{ID: "a", Name: "progress bar", Motion: []motionAttr{eased("scale.x", &keyEase{Magic: 1})}}}, nil, 0, "", 0},
		{"a card group with a spring on scale", []structureLayer{{ID: "g", Type: "group", Name: "progress card", Motion: []motionAttr{eased("scale.x", spring)}}}, nil, 0, "", 0},
		{"a layer with children is a container", []structureLayer{{ID: "a", Name: "progress bar", Motion: []motionAttr{eased("scale.x", spring)}}, {ID: "k", Parent: "a"}}, nil, 0, "", 0},
		{"a plain card with a spring", []structureLayer{{ID: "a", Name: "Card", Motion: []motionAttr{eased("scale.x", spring)}}}, nil, 0, "", 0},
		{"position never counts", []structureLayer{{ID: "a", Name: "progress bar", Motion: []motionAttr{eased("position.x", spring), eased("opacity", spring), eased("rotation.z", spring)}}}, nil, 0, "", 0},
		{"a counter feeds a text layer", []structureLayer{{ID: "n", Type: "stringGenerator", Name: "Headline number", Motion: []motionAttr{eased("generator.number", &keyEase{Magic: 32})}}, textShape},
			[]structureEdge{{From: "n", FromAttr: "id", To: "txt", ToAttr: "text"}}, 1, "text", 0},
		{"a counter feeds text.string", []structureLayer{{ID: "n", Type: "stringGenerator", Name: "Headline", Motion: []motionAttr{eased("generator.precision", &keyEase{Magic: 32})}}, textShape},
			[]structureEdge{{From: "n", FromAttr: "id", To: "txt", ToAttr: "text.string"}}, 1, "text", 0},
		{"a keyed number is data by its attribute name", []structureLayer{{ID: "n", Type: "stringGenerator", Name: "Headline", Motion: []motionAttr{eased("generator.number", &keyEase{Magic: 32})}}}, nil, 1, "attr", 0},
		{"a keyed size that feeds nothing and has a plain name", []structureLayer{{ID: "n", Name: "Headline", Motion: []motionAttr{eased("generator.dimensions.x", &keyEase{Magic: 32})}}}, nil, 0, "", 0},
		{"an attribute called value", []structureLayer{{ID: "n", Name: "Thing", Motion: []motionAttr{eased("generator.value", spring)}}}, nil, 1, "attr", 0},
		{"a custom outBack on a meter", []structureLayer{{ID: "a", Name: "meter", Motion: []motionAttr{eased("stroke.trimEnd", custom(0, 1.1, false))}}}, nil, 1, "name", 0},
		{"a custom outCubic on a meter", []structureLayer{{ID: "a", Name: "meter", Motion: []motionAttr{eased("stroke.trimEnd", custom(0, 1, true))}}}, nil, 0, "", 0},
		{"cavRole container turns the warning off", []structureLayer{{ID: "a", Name: "progress bar", Role: "container", Motion: []motionAttr{eased("scale.x", spring)}}}, nil, 0, "", 0},
		{"cavRole data flags a plain shape", []structureLayer{{ID: "a", Name: "Dot", Role: "data", Motion: []motionAttr{eased("scale.x", spring)}}}, nil, 1, "userData", 0},
		{"cavRole data does not flag position, rotation or opacity", []structureLayer{{ID: "a", Name: "Dot", Role: "data", Motion: []motionAttr{eased("position.y", spring), eased("rotation.z", spring), eased("opacity", spring)}}}, nil, 0, "", 0},
		{"an attribute with more than 64 keys is skipped, not ignored", []structureLayer{{ID: "a", Name: "progress bar", Motion: []motionAttr{{Attr: "scale.x", Span: []float64{0, 398}}}}}, nil, 0, "", 1},
		{"a long attribute that is not data is not listed", []structureLayer{{ID: "a", Name: "Card", Motion: []motionAttr{{Attr: "scale.x", Span: []float64{0, 398}}}}}, nil, 0, "", 0},
		{"a bezier hump between equal values is flagged", []structureLayer{{ID: "a", Name: "progress bar", Motion: []motionAttr{humpKeys(40, 40)}}}, nil, 1, "name", 0},
		{"a flat bezier between equal values is a hold", []structureLayer{{ID: "a", Name: "progress bar", Motion: []motionAttr{humpKeys(0, 0)}}}, nil, 0, "", 0},
		{"a custom ease that was not evaluated is skipped", []structureLayer{{ID: "a", Name: "progress bar", Motion: []motionAttr{eased("scale.x", notRead)}}}, nil, 0, "", 1},
		{"a custom ease that was not evaluated on a card is not listed", []structureLayer{{ID: "a", Name: "Card", Motion: []motionAttr{eased("scale.x", notRead)}}}, nil, 0, "", 0},
		{"a spring layer connected to a bar", []structureLayer{{ID: "a", Name: "progress bar"}, {ID: "sp", Type: "spring"}}, []structureEdge{{From: "sp", FromAttr: "output", To: "a", ToAttr: "scale.x"}}, 1, "name", 0},
		{"a step key does not overshoot", []structureLayer{{ID: "a", Name: "progress bar", Motion: []motionAttr{step}}}, nil, 0, "", 0},
		{"a hold between equal values does not overshoot", []structureLayer{{ID: "a", Name: "progress bar", Motion: []motionAttr{hold}}}, nil, 0, "", 0},
	}
	for _, tc := range tests {
		d := withEdges(motionScene(tc.layers...), tc.edges...)
		got, skipped := overshootFindings(d)
		if len(got) != tc.want || len(skipped) != tc.skipped {
			t.Errorf("%s: %d findings %+v, %d skipped %+v", tc.name, len(got), got, len(skipped), skipped)
			continue
		}
		if tc.want == 1 {
			if ev := got[0].Evidence.(map[string]any); ev["role"] != tc.role {
				t.Errorf("%s: role %v, want %s", tc.name, ev["role"], tc.role)
			}
			if got[0].Kind != "data-overshoot" || got[0].Severity != "warning" {
				t.Errorf("%s: %+v", tc.name, got[0])
			}
		}
	}
}

func TestOvershootReportsOneFindingPerAttribute(t *testing.T) {
	m := keyed([]float64{0, 10, 20, 30}, []float64{0, 50, 20, 80})
	m.Attr, m.Eases = "scale.x", []*keyEase{nil, {Magic: 23}, {Magic: 32}, nil}
	got, _ := overshootFindings(motionScene(structureLayer{ID: "a", Name: "progress bar", Motion: []motionAttr{m}}))
	if len(got) != 1 || got[0].Evidence.(map[string]any)["frame"] != 10 {
		t.Fatalf("%+v", got)
	}
}

func TestOvershootNeedsMotionMetadata(t *testing.T) {
	d := motionScene(structureLayer{ID: "a", Name: "progress bar", Motion: []motionAttr{eased("scale.x", &keyEase{Magic: 23})}})
	d.Motion = nil
	if got, skipped := overshootFindings(d); len(got) != 0 || len(skipped) != 0 {
		t.Errorf("%+v %+v", got, skipped)
	}
}

func TestQuickCheckReportsDataOvershootWithGuide(t *testing.T) {
	d := motionScene(
		structureLayer{ID: "bar", Name: "progress bar", Motion: []motionAttr{eased("scale.x", &keyEase{Magic: 23}), keyed([]float64{150, 299}, []float64{0, 1})}},
		structureLayer{ID: "card", Type: "group", Name: "card", Motion: []motionAttr{eased("scale.x", &keyEase{Magic: 23}), keyed([]float64{150, 299}, []float64{0, 1})}},
	)
	total, cov := 2, true
	d.TotalLayers, d.CoverageComplete, d.ScenePath = &total, &cov, "scratch.cv"
	t.Chdir(t.TempDir())
	motionFixture(t, d)
	out := runDeadCheck(t)
	var hits []finding
	for _, f := range out.Findings {
		if f.Kind == "data-overshoot" {
			hits = append(hits, f)
		}
	}
	if len(hits) != 1 || hits[0].Layer != "bar" || out.Clean {
		t.Fatalf("findings %+v clean %v", out.Findings, out.Clean)
	}
	if len(hits[0].Guide) == 0 || !strings.Contains(hits[0].Guide[0], "Springs on containers") {
		t.Errorf("guide %+v", hits[0].Guide)
	}
}

// humpKeys is a scale.x attribute with two keys of the same value and Bezier handles whose
// value offsets are right and left.
func humpKeys(right, left float64) motionAttr {
	m := keyed([]float64{10, 40}, []float64{50, 50})
	m.Attr = "scale.x"
	m.Eases = []*keyEase{{Right: []float64{5, right}, Left: []float64{-5, 0}}, {Right: []float64{5, 0}, Left: []float64{-5, left}}}
	return m
}
