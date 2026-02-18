# pkg/memory

Persistent long-term memory for the pigo agent. Provides storage, LLM-based extraction, deduplication, and agent-callable tools.

Storage path: `~/.pigo/memory/memories.jsonl`

## Architecture

### Three-Layer Content Model

Each memory stores content at three detail levels:

| Layer | Field | Purpose |
|---|---|---|
| L0 | `Abstract` | One-line summary for system prompt injection |
| L1 | `Overview` | Paragraph-level detail for recall results |
| L2 | `Content` | Full detail for merging and deep retrieval |

### Six Categories

| Category | Description |
|---|---|
| `profile` | User identity and background |
| `preferences` | User preferences and tendencies |
| `entities` | Named entities with lifecycle |
| `events` | Things that happened |
| `cases` | Problem/solution pairs (agent-owned) |
| `patterns` | Reusable process patterns (agent-owned) |

## Key Types

| Type | Description |
|---|---|
| `Memory` | A single persisted memory with L0/L1/L2 content, embedding vector, usage counter, timestamps |
| `MemoryStore` | Thread-safe in-memory JSONL-backed store with vector and keyword search |
| `MemoryExtractor` | LLM-powered extraction of memories from discarded conversation messages |
| `MemoryDeduplicator` | Prevents redundant memories via vector similarity + LLM-based CREATE/MERGE/SKIP decisions |

## Store API

| Method | Description |
|---|---|
| `Load() error` | Reads memories from disk |
| `Save() error` | Atomically writes memories to disk (skip if clean) |
| `Add(m) error` | Inserts a memory |
| `Update(m) error` | Replaces an existing memory |
| `Delete(id) error` | Removes a memory |
| `Get(id) *Memory` | Looks up by ID |
| `List(category) []*Memory` | Returns memories filtered by category, sorted by usage |
| `SearchByVector(vec, topK, category) []*Memory` | Cosine similarity search |
| `SearchByKeyword(query, topK) []*Memory` | Substring search across all layers |
| `FindSimilar(vec, threshold, category) []SimilarMemory` | Returns memories above a similarity threshold |
| `FormatForPrompt(maxEntries) string` | Renders top memories as markdown for the system prompt |

## Agent Tools

Three tools registered in the tool registry:

| Tool | Description |
|---|---|
| `memory_recall` | Embeds query, vector search (top-K), keyword fallback, increments usage counter |
| `memory_remember` | Validates input, embeds, deduplicates (CREATE/MERGE/SKIP), then stores |
| `memory_forget` | Deletes a memory by ID |

## Deduplication

Uses a two-stage approach:

1. **Vector similarity** — Finds existing memories with cosine similarity >= 0.7
2. **LLM decision** — Asks the model to choose CREATE, MERGE, or SKIP

Category-specific overrides:
- `profile` memories always merge with the most similar existing memory
- `events` and `cases` always create new entries (MERGE is overridden to CREATE)

## Extraction Flow

During context compaction, `MemoryExtractor.ExtractMemories` processes discarded messages:

1. Format discarded messages as text
2. Call LLM to extract candidate memories
3. Embed each candidate
4. Run deduplication
5. CREATE new memories or MERGE into existing ones
