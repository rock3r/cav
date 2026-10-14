package guide

import (
	"strings"
	"testing"
)

func TestBulletCutsOneBulletOutOfASection(t *testing.T) {
	md := "# Guide\n\n## 5. Motion\n\n- **First rule.** Text one\n  goes on here.\n- **Check both sides.** Text two\n  and more.\n\n  A second paragraph of that bullet is not kept.\n- **Last rule.** Text three.\n"
	var sec Section
	for _, s := range Split("t", md) {
		if len(s.Path) == 1 && s.Path[0] == "5. Motion" {
			sec = s
		}
	}
	b, ok := sec.Bullet("Check both")
	if !ok {
		t.Fatal("bullet not found")
	}
	want := "## 5. Motion\n\n- **Check both sides.** Text two\n  and more."
	if b.Text != want {
		t.Errorf("text %q, want %q", b.Text, want)
	}
	if len(b.Path) != 2 || b.Path[1] != "Check both sides" {
		t.Errorf("path %v", b.Path)
	}
	if last, ok := sec.Bullet("Last rule"); !ok || !strings.HasSuffix(last.Text, "- **Last rule.** Text three.") {
		t.Errorf("last bullet %q %v", last.Text, ok)
	}
	if _, ok := sec.Bullet("Nope"); ok {
		t.Error("found a bullet that is not there")
	}
}
