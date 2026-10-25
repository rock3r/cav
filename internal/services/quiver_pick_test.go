package services_test

import (
	"context"
	"testing"

	"github.com/rock3r/cav/internal/services"
)

func TestQuiverStaysBehindTheFreeTracers(t *testing.T) {
	t.Setenv("QUIVERAI_API_KEY", "present")
	t.Setenv("RECRAFT_API_TOKEN", "present")
	ch, err := services.Pick(context.Background(), &services.Config{}, "image.vector", "")
	if err != nil {
		t.Fatal(err)
	}
	if ch.Service != "recraft" {
		t.Fatalf("quiverai must not jump ahead of recraft, got %s", ch.Service)
	}
	// A plain image job never picks it: SVG generation is opt-in.
	if _, err := services.Pick(context.Background(), &services.Config{}, "image", "quiverai"); err != nil {
		t.Fatal(err)
	}
	plain, err := services.Pick(context.Background(), &services.Config{}, "image", "")
	if err != nil {
		t.Fatal(err)
	}
	if plain.Service == "quiverai" {
		t.Fatal("a plain image job picked quiverai")
	}
	named, err := services.Pick(context.Background(), &services.Config{}, "image.vector", "quiverai")
	if err != nil || named.Service != "quiverai" || named.Key != "present" {
		t.Fatalf("named: %+v %v", named, err)
	}
}
