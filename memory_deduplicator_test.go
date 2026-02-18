package main

import (
	"testing"
)

func TestAlwaysMergeCategories(t *testing.T) {
	if !alwaysMergeCategories[CatProfile] {
		t.Error("profile should be in alwaysMergeCategories")
	}
	if alwaysMergeCategories[CatEvents] {
		t.Error("events should not be in alwaysMergeCategories")
	}
}

func TestNeverMergeCategories(t *testing.T) {
	if !neverMergeCategories[CatEvents] {
		t.Error("events should be in neverMergeCategories")
	}
	if !neverMergeCategories[CatCases] {
		t.Error("cases should be in neverMergeCategories")
	}
	if neverMergeCategories[CatProfile] {
		t.Error("profile should not be in neverMergeCategories")
	}
}

func TestDedupDecisionConstants(t *testing.T) {
	if DedupCreate != "CREATE" {
		t.Errorf("expected CREATE, got %s", DedupCreate)
	}
	if DedupMerge != "MERGE" {
		t.Errorf("expected MERGE, got %s", DedupMerge)
	}
	if DedupSkip != "SKIP" {
		t.Errorf("expected SKIP, got %s", DedupSkip)
	}
}
