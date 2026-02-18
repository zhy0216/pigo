package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewMemoryStore(t *testing.T) {
	ms := NewMemoryStore()
	if ms == nil {
		t.Fatal("NewMemoryStore returned nil")
	}
	if ms.Count() != 0 {
		t.Errorf("expected 0 memories, got %d", ms.Count())
	}
}

func TestMemoryAddAndGet(t *testing.T) {
	ms := NewMemoryStore()
	mem := &Memory{
		ID:       "test_1",
		Category: CatProfile,
		Abstract: "User is a Go developer",
		Overview: "The user primarily writes Go code.",
		Content:  "The user is a backend developer who primarily writes Go.",
	}
	if err := ms.Add(mem); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	got := ms.Get("test_1")
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.Abstract != "User is a Go developer" {
		t.Errorf("expected abstract 'User is a Go developer', got '%s'", got.Abstract)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}
}

func TestMemoryAddGeneratesID(t *testing.T) {
	ms := NewMemoryStore()
	mem := &Memory{
		Category: CatEntities,
		Abstract: "pigo project",
	}
	ms.Add(mem)
	if mem.ID == "" {
		t.Error("expected generated ID")
	}
	if ms.Get(mem.ID) == nil {
		t.Error("could not retrieve by generated ID")
	}
}

func TestMemoryUpdate(t *testing.T) {
	ms := NewMemoryStore()
	mem := &Memory{
		ID:       "test_1",
		Category: CatProfile,
		Abstract: "v1",
	}
	ms.Add(mem)

	mem.Abstract = "v2"
	if err := ms.Update(mem); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got := ms.Get("test_1")
	if got.Abstract != "v2" {
		t.Errorf("expected 'v2', got '%s'", got.Abstract)
	}
}

func TestMemoryUpdateNotFound(t *testing.T) {
	ms := NewMemoryStore()
	err := ms.Update(&Memory{ID: "nonexistent"})
	if err == nil {
		t.Error("expected error for nonexistent memory")
	}
}

func TestMemoryDelete(t *testing.T) {
	ms := NewMemoryStore()
	ms.Add(&Memory{ID: "test_1", Category: CatEvents, Abstract: "event"})

	if err := ms.Delete("test_1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if ms.Get("test_1") != nil {
		t.Error("memory should be deleted")
	}
	if ms.Count() != 0 {
		t.Errorf("expected 0 memories, got %d", ms.Count())
	}
}

func TestMemoryDeleteNotFound(t *testing.T) {
	ms := NewMemoryStore()
	if err := ms.Delete("nonexistent"); err == nil {
		t.Error("expected error for nonexistent memory")
	}
}

func TestMemoryList(t *testing.T) {
	ms := NewMemoryStore()
	ms.Add(&Memory{ID: "p1", Category: CatProfile, Abstract: "profile 1", ActiveCount: 5})
	ms.Add(&Memory{ID: "e1", Category: CatEvents, Abstract: "event 1", ActiveCount: 2})
	ms.Add(&Memory{ID: "p2", Category: CatProfile, Abstract: "profile 2", ActiveCount: 10})

	profiles := ms.List(CatProfile)
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(profiles))
	}
	// Should be sorted by active_count desc
	if profiles[0].ID != "p2" {
		t.Errorf("expected p2 first (higher active_count), got %s", profiles[0].ID)
	}

	events := ms.List(CatEvents)
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestMemoryAll(t *testing.T) {
	ms := NewMemoryStore()
	ms.Add(&Memory{ID: "1", Category: CatProfile, Abstract: "a", ActiveCount: 1})
	ms.Add(&Memory{ID: "2", Category: CatEvents, Abstract: "b", ActiveCount: 3})

	all := ms.All()
	if len(all) != 2 {
		t.Fatalf("expected 2, got %d", len(all))
	}
	if all[0].ID != "2" {
		t.Errorf("expected highest active_count first")
	}
}

func TestMemoryClear(t *testing.T) {
	ms := NewMemoryStore()
	ms.Add(&Memory{ID: "1", Category: CatProfile, Abstract: "a"})
	ms.Add(&Memory{ID: "2", Category: CatEvents, Abstract: "b"})
	ms.Clear()
	if ms.Count() != 0 {
		t.Errorf("expected 0 after clear, got %d", ms.Count())
	}
}

func TestMemoryIncrementActive(t *testing.T) {
	ms := NewMemoryStore()
	ms.Add(&Memory{ID: "1", Category: CatProfile, Abstract: "a", ActiveCount: 0})
	ms.IncrementActive("1")
	ms.IncrementActive("1")

	got := ms.Get("1")
	if got.ActiveCount != 2 {
		t.Errorf("expected active_count 2, got %d", got.ActiveCount)
	}

	// Incrementing nonexistent should not panic
	ms.IncrementActive("nonexistent")
}

func TestMemoryPersistence(t *testing.T) {
	// Use temp dir for test
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create memory dir
	os.MkdirAll(filepath.Join(tmpDir, memoryDir), 0755)

	// Save some memories
	ms1 := NewMemoryStore()
	ms1.Add(&Memory{
		ID:       "test_persist",
		Category: CatProfile,
		Abstract: "persistent memory",
		Overview: "testing persistence",
		Content:  "full content here",
	})
	ms1.dirty = true
	if err := ms1.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load into new store
	ms2 := NewMemoryStore()
	if err := ms2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	got := ms2.Get("test_persist")
	if got == nil {
		t.Fatal("loaded memory not found")
	}
	if got.Abstract != "persistent memory" {
		t.Errorf("expected 'persistent memory', got '%s'", got.Abstract)
	}
	if got.Content != "full content here" {
		t.Errorf("expected 'full content here', got '%s'", got.Content)
	}
}

func TestMemoryLoadNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	ms := NewMemoryStore()
	// Should not error on missing file
	if err := ms.Load(); err != nil {
		t.Errorf("Load should not error on missing file: %v", err)
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []float64
		expected float64
		delta    float64
	}{
		{"identical", []float64{1, 0, 0}, []float64{1, 0, 0}, 1.0, 0.001},
		{"orthogonal", []float64{1, 0, 0}, []float64{0, 1, 0}, 0.0, 0.001},
		{"opposite", []float64{1, 0, 0}, []float64{-1, 0, 0}, -1.0, 0.001},
		{"similar", []float64{1, 1, 0}, []float64{1, 0, 0}, 0.707, 0.01},
		{"empty_a", []float64{}, []float64{1, 0}, 0.0, 0.001},
		{"empty_b", []float64{1, 0}, []float64{}, 0.0, 0.001},
		{"different_len", []float64{1, 0}, []float64{1, 0, 0}, 0.0, 0.001},
		{"zero_a", []float64{0, 0, 0}, []float64{1, 0, 0}, 0.0, 0.001},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CosineSimilarity(tt.a, tt.b)
			if got < tt.expected-tt.delta || got > tt.expected+tt.delta {
				t.Errorf("expected ~%.3f, got %.3f", tt.expected, got)
			}
		})
	}
}

func TestMemorySearchByVector(t *testing.T) {
	ms := NewMemoryStore()
	ms.Add(&Memory{ID: "1", Category: CatProfile, Abstract: "a", Vector: []float64{1, 0, 0}})
	ms.Add(&Memory{ID: "2", Category: CatProfile, Abstract: "b", Vector: []float64{0.9, 0.1, 0}})
	ms.Add(&Memory{ID: "3", Category: CatEvents, Abstract: "c", Vector: []float64{0, 1, 0}})
	ms.Add(&Memory{ID: "4", Category: CatProfile, Abstract: "d"}) // no vector

	// Search for vector close to [1, 0, 0]
	results := ms.SearchByVector([]float64{1, 0, 0}, 2, "")
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ID != "1" {
		t.Errorf("expected ID 1 first, got %s", results[0].ID)
	}

	// Filter by category
	results = ms.SearchByVector([]float64{1, 0, 0}, 10, CatEvents)
	if len(results) != 1 {
		t.Fatalf("expected 1 result with CatEvents filter, got %d", len(results))
	}
	if results[0].ID != "3" {
		t.Errorf("expected ID 3, got %s", results[0].ID)
	}

	// Empty vector
	results = ms.SearchByVector(nil, 5, "")
	if len(results) != 0 {
		t.Errorf("expected 0 results for nil vector, got %d", len(results))
	}
}

func TestMemorySearchByKeyword(t *testing.T) {
	ms := NewMemoryStore()
	ms.Add(&Memory{ID: "1", Category: CatProfile, Abstract: "Go developer", ActiveCount: 5})
	ms.Add(&Memory{ID: "2", Category: CatEntities, Abstract: "pigo project", Content: "A Go CLI tool", ActiveCount: 2})
	ms.Add(&Memory{ID: "3", Category: CatEvents, Abstract: "fixed a bug", ActiveCount: 1})

	results := ms.SearchByKeyword("Go", 10)
	if len(results) != 2 {
		t.Fatalf("expected 2 results for 'Go', got %d", len(results))
	}
	// Should be sorted by active_count
	if results[0].ID != "1" {
		t.Errorf("expected highest active_count first, got %s", results[0].ID)
	}

	results = ms.SearchByKeyword("bug", 1)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	results = ms.SearchByKeyword("nonexistent", 10)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}

	results = ms.SearchByKeyword("", 10)
	if len(results) != 0 {
		t.Errorf("expected 0 for empty query, got %d", len(results))
	}
}

func TestMemoryFindSimilar(t *testing.T) {
	ms := NewMemoryStore()
	ms.Add(&Memory{ID: "1", Category: CatProfile, Abstract: "a", Vector: []float64{1, 0, 0}})
	ms.Add(&Memory{ID: "2", Category: CatProfile, Abstract: "b", Vector: []float64{0.5, 0.5, 0.707}})
	ms.Add(&Memory{ID: "3", Category: CatProfile, Abstract: "c", Vector: []float64{0, 1, 0}})

	similar := ms.FindSimilar([]float64{1, 0, 0}, 0.9, "")
	if len(similar) != 1 {
		t.Fatalf("expected 1 result above 0.9 threshold, got %d", len(similar))
	}
	if similar[0].Memory.ID != "1" {
		t.Errorf("expected ID 1, got %s", similar[0].Memory.ID)
	}

	similar = ms.FindSimilar([]float64{1, 0, 0}, 0.3, "")
	if len(similar) != 2 {
		t.Fatalf("expected 2 results above 0.3 threshold, got %d", len(similar))
	}
}

func TestMemoryFormatForPrompt(t *testing.T) {
	ms := NewMemoryStore()
	ms.Add(&Memory{ID: "1", Category: CatProfile, Abstract: "Go developer", ActiveCount: 10})
	ms.Add(&Memory{ID: "2", Category: CatPreferences, Abstract: "Prefers minimal code", ActiveCount: 5})
	ms.Add(&Memory{ID: "3", Category: CatEntities, Abstract: "pigo project", ActiveCount: 3})

	output := ms.FormatForPrompt(20)
	if output == "" {
		t.Error("expected non-empty output")
	}
	if !strings.Contains(output, "Go developer") {
		t.Error("should contain 'Go developer'")
	}
	if !strings.Contains(output, "profile") {
		t.Error("should contain category header 'profile'")
	}

	// Empty store
	empty := NewMemoryStore()
	if empty.FormatForPrompt(20) != "" {
		t.Error("expected empty output for empty store")
	}
}

func TestIsValidCategory(t *testing.T) {
	if !IsValidCategory("profile") {
		t.Error("profile should be valid")
	}
	if !IsValidCategory("events") {
		t.Error("events should be valid")
	}
	if IsValidCategory("invalid") {
		t.Error("invalid should not be valid")
	}
	if IsValidCategory("") {
		t.Error("empty string should not be valid")
	}
}

func TestMemorySaveNoopWhenClean(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	ms := NewMemoryStore()
	// Save without dirty flag should be noop
	if err := ms.Save(); err != nil {
		t.Errorf("Save should succeed when clean: %v", err)
	}

	// File should not exist
	path := filepath.Join(tmpDir, memoryDir, memoryFile)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should not exist when no dirty save")
	}
}

func TestMemoryTimestamps(t *testing.T) {
	ms := NewMemoryStore()
	before := time.Now()
	ms.Add(&Memory{ID: "1", Category: CatProfile, Abstract: "test"})
	after := time.Now()

	mem := ms.Get("1")
	if mem.CreatedAt.Before(before) || mem.CreatedAt.After(after) {
		t.Error("CreatedAt should be between before and after")
	}
	if mem.UpdatedAt.Before(before) || mem.UpdatedAt.After(after) {
		t.Error("UpdatedAt should be between before and after")
	}
}
