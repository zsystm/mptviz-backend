package trie

import (
	"maps"

	"github.com/ethereum/go-ethereum/common"
)

// Tracer tracks the changes of trie nodes. During the trie operations,
// some nodes can be deleted from the trie, while these deleted nodes
// won't be captured by trie.Hasher or trie.Committer. Thus, these deleted
// nodes won't be removed from the disk at all. Tracer is an auxiliary tool
// used to track all Insert and delete operations of trie and capture all
// deleted nodes eventually.
//
// The changed nodes can be mainly divided into two categories: the leaf
// node and intermediate node. The former is inserted/deleted by callers
// while the latter is inserted/deleted in order to follow the rule of trie.
// This tool can track all of them no matter the node is embedded in its
// parent or not, but valueNode is never tracked.
//
// Besides, it's also used for recording the original value of the nodes
// when they are resolved from the disk. The pre-value of the nodes will
// be used to construct trie history in the future.
//
// Note tracer is not thread-safe, callers should be responsible for handling
// the concurrency issues by themselves.
type Tracer struct {
	Inserts    map[string]struct{}
	Deletes    map[string]struct{}
	AccessList map[string][]byte
}

// NewTracer initializes the tracer for capturing trie changes.
func NewTracer() *Tracer {
	return &Tracer{
		Inserts:    make(map[string]struct{}),
		Deletes:    make(map[string]struct{}),
		AccessList: make(map[string][]byte),
	}
}

// OnRead tracks the newly loaded trie node and caches the rlp-encoded
// blob internally. Don't change the value outside of function since
// it's not deep-copied.
func (t *Tracer) OnRead(path []byte, val []byte) {
	t.AccessList[string(path)] = val
}

// OnInsert tracks the newly inserted trie node. If it's already
// in the deletion set (resurrected node), then just wipe it from
// the deletion set as it's "untouched".
func (t *Tracer) OnInsert(path []byte) {
	if _, present := t.Deletes[string(path)]; present {
		delete(t.Deletes, string(path))
		return
	}
	t.Inserts[string(path)] = struct{}{}
}

// OnDelete tracks the newly deleted trie node. If it's already
// in the addition set, then just wipe it from the addition set
// as it's untouched.
func (t *Tracer) OnDelete(path []byte) {
	if _, present := t.Inserts[string(path)]; present {
		delete(t.Inserts, string(path))
		return
	}
	t.Deletes[string(path)] = struct{}{}
}

// Reset clears the content tracked by tracer.
func (t *Tracer) Reset() {
	t.Inserts = make(map[string]struct{})
	t.Deletes = make(map[string]struct{})
	t.AccessList = make(map[string][]byte)
}

// Copy returns a deep copied tracer instance.
func (t *Tracer) Copy() *Tracer {
	accessList := make(map[string][]byte, len(t.AccessList))
	for path, blob := range t.AccessList {
		accessList[path] = common.CopyBytes(blob)
	}
	return &Tracer{
		Inserts:    maps.Clone(t.Inserts),
		Deletes:    maps.Clone(t.Deletes),
		AccessList: accessList,
	}
}

// DeletedNodes returns a list of node paths which are deleted from the trie.
func (t *Tracer) DeletedNodes() []string {
	var paths []string
	for path := range t.Deletes {
		// It's possible a few deleted nodes were embedded
		// in their parent before, the deletions can be no
		// effect by deleting nothing, filter them out.
		_, ok := t.AccessList[path]
		if !ok {
			continue
		}
		paths = append(paths, path)
	}
	return paths
}
