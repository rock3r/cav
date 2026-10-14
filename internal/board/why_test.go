package board

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestWhyRoundTripsAndIsOmittedWhenEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "storyboard.json")
	sb := &Board{BPM: 120, Seconds: 8, Shots: []Shot{
		{ID: "s1", Beats: [2]float64{0, 4}, What: "a", Why: "the viewer learns the name"},
		{ID: "s2", Beats: [2]float64{4, 8}, What: "b"},
	}}
	if err := sb.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got.Shots[0].Why != "the viewer learns the name" || got.Shots[1].Why != "" {
		t.Fatalf("%+v %v", got, err)
	}
	b, _ := json.Marshal(sb.Shots[1])
	if strings.Contains(string(b), "why") {
		t.Errorf("a shot without why must not write the key: %s", b)
	}
	if err := sb.Validate(); err != nil {
		t.Errorf("why is not required: %v", err)
	}
}
