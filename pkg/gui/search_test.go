package gui

import "testing"

func TestNextMatchIndexFindsClosestMatch(t *testing.T) {
	labels := []string{"alpha", "beta one", "gamma", "delta", "beta two"}

	if got := nextMatchIndex(labels, "beta", 2); got != 1 {
		t.Fatalf("expected closest beta near index 2 to be 1, got %d", got)
	}

	if got := nextMatchIndex(labels, "beta", 1); got != 1 {
		t.Fatalf("expected current matching index to stay selected, got %d", got)
	}
}

func TestNextMatchIndexHandlesEmptyAndMissingQueries(t *testing.T) {
	labels := []string{"alpha", "beta", "gamma"}

	if got := nextMatchIndex(labels, "", 9); got != 2 {
		t.Fatalf("expected empty query to clamp selection to 2, got %d", got)
	}

	if got := nextMatchIndex(labels, "missing", 1); got != -1 {
		t.Fatalf("expected missing query to return -1, got %d", got)
	}
}
