---
session: ses_fa7d
updated: 2026-08-31T14:23:45.944Z
---

Now append the warehouse methods to the memory adapter. Let me view its tail first: 
[Assistant tool call]: bash({"command":"sed -n '52,80p' internal/adapters/catalogue/memory/catalogue.go && grep -n \"FindSnapshotByRoot\" internal/adapters/catalogue/memory/catalogue.go"})
[Tool result]: (no content returned)
[Assistant tool call]: bash({"command":"cat internal/adapters/catalogue/memory/catalogue.go | tail -30; echo ===; grep -c \"\" internal/adapters/catalogue/memory/catalogue.go"})
[Tool result]: (no content returned)
[Assistant tool call]: read({"filePath":"internal/adapters/catalogue/memory/catalogue.go"})
[Tool result]: <path>/home/sifu/Documents/dbvault/internal/adapters/catalogue/memory/catalogue.go</path>
<type>file</type>
<content>
1: package memory
2: 
3: import (
4: 	"context"
5: 	"fmt"
6: 	"github.com/dbvault/dbvault/internal/domain"
7: 	"sort"
8: 	"sync"
9: )
10: 
11: type Catalogue struct {
12: 	mu                sync.Mutex
13: 	runs              map[domain.BackupRunID]DomainBackupRun
14: 	snaps             map[domain.SnapshotID]domain.Snapshot
15: 	snapChunks       map[domain.SnapshotID][]domain.SnapshotChunk
16: 	chunks            map[domain.ChunkID]domain.Chunk
17: 	warehouseDatasets map[string]domain.WarehouseDataset
18: }
19: 
20: func New() *Catalogue {
21: 	return &Catalogue{runs: map[domain.BackupRunID]DomainBackupRun{}, snaps: map[domain.SnapshotID]domain.Snapshot{}, snapChunks: map[domain.SnapshotID][]domain.SnapshotChunk{}, chunks: map[domain.ChunkID]domain.Chunk{}, warehouseDatasets: map[string]domain.WarehouseDataset{}}
22: }
23: func (c *Catalogue) CreateBackupRun(ctx context.Context, run domain.BackupRun) error {
24: 	c.mu.Lock()
25: 	defer c.mu.Unlock()
26: 	c.runs[run.ID] = run
27: 	return nil
28: }
27: func (c *Catalogue) UpdateBackupRun(ctx context.Context, run domain.BackupRun) error {
28: 	c.mu.Lock()
29: 	defer c.mu.Unlock()
30: 	c.runs[run.ID] = run
31: 	return nil
32: }
33: func (c *Catalogue) CreateSnapshot(ctx context.Context, snap domain.Snapshot, sc []domain.SnapshotChunk, cr []domain.Chunk) error {
34: 	c.mu.Lock()
35: 	defer c.mu.Unlock()
36: 	c.snaps[snap.ID] = snap
37: 	c.snapChunks[snap.ID] = append([]domain.SnapshotChunk(nil), sc...)
38: 	for _, ch := range cr {
39: 		c.chunks[ch.ID] = ch
40: 	}
41: 	return nil
42: }
43: func (c *Catalogue) GetSnapshot(ctx context.Context, id domain.SnapshotID) (domain.Snapshot, []domain.SnapshotChunk, error) {
44: 	c.mu.Lock()
45: 	defer c.mu.Unlock()
46: 	s, ok := c.snaps[id]
47: 	if !ok {
48: 		return domain.Snapshot{}, nil, fmt.Errorf("snapshot not found")
49: 	}
50: 	return s, append([]domain.SnapshotChunk(nil), c.snapChunks[id]...), nil
51: }
52: func (c *Catalogue) ListSnapshots(ctx context.Context, sourceID domain.SourceID) ([]domain.Snapshot, error) {
53: 	c.mu.Lock()
54: 	defer c.mu.Unlock()
55: 	out := []domain.Snapshot{}
56: 	for _, s := range c.snaps {
57: 		if s.SourceID == sourceID {
58: 			out = append(out, s)
59: 		}
60: 	}
61: 	sort.Slice(...)
62: 	return out, nil
63: }
...
64: func (c *Catalogue) FindSnapshotByRoot(ctx context.Context, sourceID domain.SourceID, rootDigest string) (domain.Snapshot, bool, error) {
65: 	c.mu.Lock()
66: 	defer c.mu.Unlock()
67: 	for _, s := range c.snaps {
68: 		if s.SourceID == sourceID && s.RootDigest == rootDigest {
69: 			return s, true, nil
70: 		}
71: 	}
72: 	return domain.Snapshot{}, false, nil
73: }
...
74: func (c *Catalogue) AppendAudit(ctx context.Context, eventType, resource string, payload any, now time.Time) {
...
75: 	...
76: }
...
77: func (c *Catalogue) GetBackupRun(ctx context.Context, id domain.BackupRunID) (domain.BackupRun, bool, error) {
...
78: }
...
79: func (c *Catalogue) AppendAudit... (remaining methods follow)

[truncated]
[Assistant]: The file now has a corruption from my edit — `DomainBackupRun` isn't a real type; my edit replaced the struct but something merged wrong. Let me check the actual current state precisely and fix it.
[Assistant tool call]: bash({"command":"git diff internal/adapters/catalogue/memory/catalogue.go | head -40"})
[Tool result]: (no content returned)
[Assistant tool call]: git diff internal/adapters/catalogue/memory/catalogue.go 2>&1 | head -50
[Tool result]: (no content returned)
[Assistant tool call]: sed -n '1,30p' internal/adapters/catalogue/memory/catalogue.go
[Tool result]: (no content returned)
[Assistant tool call]: cat -n internal/adapters/catalogue/memory/catalogue.go | head -30
[Tool result]: (no content returned)
[Assistant tool call]: wc -l internal/adapters/catalogue/memory/catalogue.go
[Tool result]: (no content returned)
