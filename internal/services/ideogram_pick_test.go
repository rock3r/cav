package services_test

import (
	"context"
	"testing"

	"github.com/rock3r/cav/internal/services"
)

func TestIdeogramIsNotPickedWhenAKeyExists(t *testing.T) {
	t.Setenv("IDEOGRAM_API_KEY", "present")
	t.Setenv("RECRAFT_API_TOKEN", "")
	t.Setenv("RECRAFT_API_KEY", "")
	ch, err := services.Pick(context.Background(), &services.Config{}, "image.vector", "")
	if err != nil {
		t.Fatal(err)
	}
	if ch.Service == "ideogram" {
		t.Fatal("a key selected ideogram; it must be named with --service")
	}
	named, err := services.Pick(context.Background(), &services.Config{}, "image.vector", "ideogram")
	if err != nil || named.Service != "ideogram" || named.Key != "present" {
		t.Fatalf("named: %+v %v", named, err)
	}
}
