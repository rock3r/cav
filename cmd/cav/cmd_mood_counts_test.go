package main

import "testing"

func TestMoodCountsUseTheSingularForOne(t *testing.T) {
	got := moodCounts(4, map[string]int{"image": 2, "clip": 1, "embed": 1})
	if got != "4 items: 2 images, 1 clip, 1 web video" {
		t.Error(got)
	}
	if got := moodCounts(1, map[string]int{"image": 1}); got != "1 item: 1 image, 0 clips, 0 web videos" {
		t.Error(got)
	}
}
