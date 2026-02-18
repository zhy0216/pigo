package memory

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// MemoryCategory represents the classification of a memory.
// Follows OpenViking's 6-category system.
type MemoryCategory string

const (
	CatProfile     MemoryCategory = "profile"     // User identity/background
	CatPreferences MemoryCategory = "preferences" // User preferences/tendencies
	CatEntities    MemoryCategory = "entities"    // Named entities with lifecycle
	CatEvents      MemoryCategory = "events"      // Things that happened
	CatCases       MemoryCategory = "cases"       // Problem + solution pairs (agent)
	CatPatterns    MemoryCategory = "patterns"    // Reusable process patterns (agent)
)

const (
	memoryDir           = ".pigo/memory"
	memoryFile          = "memories.jsonl"
	SimilarityThreshold = 0.7
)

// ValidCategories lists all valid memory categories.
var ValidCategories = []MemoryCategory{
	CatProfile, CatPreferences, CatEntities,
	CatEvents, CatCases, CatPatterns,
}

// Memory represents a single persisted memory with L0/L1/L2 layers.
type Memory struct {
	ID          string         `json:"id"`
	Category    MemoryCategory `json:"category"`
	Abstract    string         `json:"abstract"`     // L0: one-liner index
	Overview    string         `json:"overview"`     // L1: structured summary
	Content     string         `json:"content"`      // L2: full detail
	Vector      []float64      `json:"vector"`       // embedding vector
	ActiveCount int            `json:"active_count"` // usage frequency
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// MemoryStore manages persistent memories with in-memory index and vector search.
type MemoryStore struct {
	memories map[string]*Memory
	mu       sync.RWMutex
	dirty    bool
}

// NewMemoryStore creates a new empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		memories: make(map[string]*Memory),
	}
}

// memoryStorePath returns the path to the memories file.
func memoryStorePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	dir := filepath.Join(home, memoryDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("cannot create memory directory: %w", err)
	}
	return filepath.Join(dir, memoryFile), nil
}

// GenerateMemoryID creates a random memory ID.
func GenerateMemoryID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("mem_%x", b)
}

// Load reads memories from the JSONL file on disk.
func (ms *MemoryStore) Load() error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	path, err := memoryStorePath()
	if err != nil {
		return err
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("cannot open memory file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var m Memory
		if err := json.Unmarshal(line, &m); err != nil {
			return fmt.Errorf("malformed memory on line %d: %w", lineNum, err)
		}
		ms.memories[m.ID] = &m
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading memory file: %w", err)
	}

	ms.dirty = false
	return nil
}

// Save writes all memories to the JSONL file atomically.
func (ms *MemoryStore) Save() error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if !ms.dirty {
		return nil
	}

	path, err := memoryStorePath()
	if err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("cannot create temp memory file: %w", err)
	}

	enc := json.NewEncoder(f)
	for _, m := range ms.memories {
		if err := enc.Encode(m); err != nil {
			f.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("cannot write memory: %w", err)
		}
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("cannot close temp memory file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("cannot rename temp memory file: %w", err)
	}

	ms.dirty = false
	return nil
}

// Add inserts a new memory into the store.
func (ms *MemoryStore) Add(m *Memory) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if m.ID == "" {
		m.ID = GenerateMemoryID()
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	m.UpdatedAt = time.Now()

	ms.memories[m.ID] = m
	ms.dirty = true
	return nil
}

// Update replaces an existing memory in the store.
func (ms *MemoryStore) Update(m *Memory) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if _, ok := ms.memories[m.ID]; !ok {
		return fmt.Errorf("memory %s not found", m.ID)
	}
	m.UpdatedAt = time.Now()
	ms.memories[m.ID] = m
	ms.dirty = true
	return nil
}

// Delete removes a memory from the store.
func (ms *MemoryStore) Delete(id string) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if _, ok := ms.memories[id]; !ok {
		return fmt.Errorf("memory %s not found", id)
	}
	delete(ms.memories, id)
	ms.dirty = true
	return nil
}

// Get returns a memory by ID, or nil if not found.
func (ms *MemoryStore) Get(id string) *Memory {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return ms.memories[id]
}

// List returns all memories in a given category, sorted by active_count desc.
func (ms *MemoryStore) List(category MemoryCategory) []*Memory {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	var result []*Memory
	for _, m := range ms.memories {
		if category == "" || m.Category == category {
			result = append(result, m)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ActiveCount > result[j].ActiveCount
	})
	return result
}

// All returns all memories sorted by active_count desc.
func (ms *MemoryStore) All() []*Memory {
	return ms.List("")
}

// Count returns the total number of memories.
func (ms *MemoryStore) Count() int {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return len(ms.memories)
}

// Clear removes all memories.
func (ms *MemoryStore) Clear() {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.memories = make(map[string]*Memory)
	ms.dirty = true
}

// IncrementActive bumps the active_count for a memory.
func (ms *MemoryStore) IncrementActive(id string) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if m, ok := ms.memories[id]; ok {
		m.ActiveCount++
		m.UpdatedAt = time.Now()
		ms.dirty = true
	}
}

// SearchByVector finds the top-K memories closest to the given vector using cosine similarity.
// If category is non-empty, only memories of that category are considered.
func (ms *MemoryStore) SearchByVector(vec []float64, topK int, category MemoryCategory) []*Memory {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	if len(vec) == 0 || topK <= 0 {
		return nil
	}

	type scored struct {
		mem   *Memory
		score float64
	}

	var candidates []scored
	for _, m := range ms.memories {
		if category != "" && m.Category != category {
			continue
		}
		if len(m.Vector) == 0 {
			continue
		}
		sim := CosineSimilarity(vec, m.Vector)
		candidates = append(candidates, scored{mem: m, score: sim})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	if topK > len(candidates) {
		topK = len(candidates)
	}

	result := make([]*Memory, topK)
	for i := 0; i < topK; i++ {
		result[i] = candidates[i].mem
	}
	return result
}

// SearchByKeyword finds memories whose abstract or content contains the query substring.
func (ms *MemoryStore) SearchByKeyword(query string, topK int) []*Memory {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	if query == "" || topK <= 0 {
		return nil
	}

	q := strings.ToLower(query)
	var result []*Memory
	for _, m := range ms.memories {
		if strings.Contains(strings.ToLower(m.Abstract), q) ||
			strings.Contains(strings.ToLower(m.Overview), q) ||
			strings.Contains(strings.ToLower(m.Content), q) {
			result = append(result, m)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ActiveCount > result[j].ActiveCount
	})

	if topK > len(result) {
		topK = len(result)
	}
	return result[:topK]
}

// SimilarMemory pairs a memory with its similarity score.
type SimilarMemory struct {
	Memory *Memory
	Score  float64
}

// FindSimilar returns memories with cosine similarity above the threshold.
func (ms *MemoryStore) FindSimilar(vec []float64, threshold float64, category MemoryCategory) []SimilarMemory {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	if len(vec) == 0 {
		return nil
	}

	var result []SimilarMemory
	for _, m := range ms.memories {
		if category != "" && m.Category != category {
			continue
		}
		if len(m.Vector) == 0 {
			continue
		}
		sim := CosineSimilarity(vec, m.Vector)
		if sim >= threshold {
			result = append(result, SimilarMemory{Memory: m, Score: sim})
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})

	return result
}

// FormatForPrompt returns a compact representation of top memories for the system prompt.
// Groups by category, shows L0 abstracts, limited to maxEntries total.
func (ms *MemoryStore) FormatForPrompt(maxEntries int) string {
	all := ms.All()
	if len(all) == 0 || maxEntries <= 0 {
		return ""
	}

	if maxEntries > len(all) {
		maxEntries = len(all)
	}
	top := all[:maxEntries]

	grouped := make(map[MemoryCategory][]*Memory)
	for _, m := range top {
		grouped[m.Category] = append(grouped[m.Category], m)
	}

	var buf strings.Builder
	for _, cat := range ValidCategories {
		mems := grouped[cat]
		if len(mems) == 0 {
			continue
		}
		fmt.Fprintf(&buf, "### %s\n", cat)
		for _, m := range mems {
			fmt.Fprintf(&buf, "- %s\n", m.Abstract)
		}
		buf.WriteString("\n")
	}

	return buf.String()
}

// IsValidCategory checks if a string is a valid memory category.
func IsValidCategory(s string) bool {
	for _, c := range ValidCategories {
		if string(c) == s {
			return true
		}
	}
	return false
}

// CosineSimilarity computes the cosine similarity between two vectors.
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}
	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
