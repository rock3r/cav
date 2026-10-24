package services_test

import (
	"context"
	"testing"

	"github.com/rock3r/cav/internal/services"
)

func TestIdeogramStaysBehindTheFreeTracers(t *testing.T) {
	t.Setenv("IDEOGRAM_API_KEY", "present")
	t.Setenv("RECRAFT_API_TOKEN", "present")
	ch, err := services.Pick(context.Background(), &services.Config{}, "image.vector", "")
	if err != nil {
		t.Fatal(err)
	}
	if ch.Service != "recraft" {
		t.Fatalf("ideogram must not jump ahead of recraft, got %s", ch.Service)
	}
	// A custom order can still put it first. The default is what keeps it last.
	c := &services.Config{}
	if err := c.SetOrder("image.vector", []string{"ideogram", "recraft"}); err != nil {
		t.Fatal(err)
	}
	first, err := services.Pick(context.Background(), c, "image.vector", "")
	if err != nil || first.Service != "ideogram" || first.Key != "present" {
		t.Fatalf("custom order: %+v %v", first, err)
	}
}
