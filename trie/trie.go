package trie

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/trie/trienode"
	"github.com/ethereum/go-ethereum/triedb/database"
)

// Trie is a Merkle Patricia Trie. Use New to create a trie that sits on
// top of a database. Whenever trie performs a commit operation, the generated
// nodes will be gathered and returned in a set. Once the trie is committed,
// it's not usable anymore. Callers have to re-create the trie with new root
// based on the updated trie database.
//
// Trie is not safe for concurrent use.
type Trie struct {
	Root  Node
	Owner common.Hash

	// Flag whether the commit operation is already performed. If so the
	// trie is not usable(latest states is invisible).
	Committed bool

	// Keep track of the number leaves which have been inserted since the last
	// hashing operation. This number will not directly map to the number of
	// actually unhashed nodes.
	Unhashed int

	// reader is the handler trie can retrieve nodes from.
	Reader *TrieReader

	// tracer is the tool to track the trie changes.
	Tracer *Tracer
}

// NewFlag returns the cache flag value for a newly created node.
func (t *Trie) NewFlag() NodeFlag {
	return NodeFlag{Dirty: true}
}

// Copy returns a copy of Trie.
func (t *Trie) Copy() *Trie {
	return &Trie{
		Root:      t.Root,
		Owner:     t.Owner,
		Committed: t.Committed,
		Unhashed:  t.Unhashed,
		Reader:    t.Reader,
		Tracer:    t.Tracer.Copy(),
	}
}

// New creates the trie instance with provided trie id and the read-only
// database. The state specified by trie id must be available, otherwise
// an error will be returned. The trie root specified by trie id can be
// zero hash or the sha3 hash of an empty string, then trie is initially
// empty, otherwise, the root node must be present in database or returns
// a MissingNodeError if not.
func New(id *ID, db database.Database) (*Trie, error) {
	reader, err := NewTrieReader(id.StateRoot, id.Owner, db)
	if err != nil {
		return nil, err
	}
	trie := &Trie{
		Owner:  id.Owner,
		Reader: reader,
		Tracer: NewTracer(),
	}
	if id.Root != (common.Hash{}) && id.Root != types.EmptyRootHash {
		rootnode, err := trie.ResolveAndTrack(id.Root[:], nil)
		if err != nil {
			return nil, err
		}
		trie.Root = rootnode
	}
	return trie, nil
}

// NewEmpty is a shortcut to create empty tree. It's mostly used in tests.
func NewEmpty(db database.Database) *Trie {
	tr, _ := New(TrieID(types.EmptyRootHash), db)
	return tr
}

// MustNodeIterator is a wrapper of NodeIterator and will omit any encountered
// error but just print out an error message.
func (t *Trie) MustNodeIterator(start []byte) NodeIterator {
	it, err := t.NodeIterator(start)
	if err != nil {
		log.Error("Unhandled trie error in Trie.NodeIterator", "err", err)
	}
	return it
}

// NodeIterator returns an iterator that returns nodes of the trie. Iteration starts at
// the key after the given start key.
func (t *Trie) NodeIterator(start []byte) (NodeIterator, error) {
	// Short circuit if the trie is already committed and not usable.
	if t.Committed {
		return nil, ErrCommitted
	}
	return NewDefaultNodeIterator(t, start), nil
}

// MustGet is a wrapper of Get and will omit any encountered error but just
// print out an error message.
func (t *Trie) MustGet(key []byte) []byte {
	res, err := t.Get(key)
	if err != nil {
		log.Error("Unhandled trie error in Trie.Get", "err", err)
	}
	return res
}

// Get returns the value for key stored in the trie.
// The value bytes must not be modified by the caller.
//
// If the requested node is not present in trie, no error will be returned.
// If the trie is corrupted, a MissingNodeError is returned.
func (t *Trie) Get(key []byte) ([]byte, error) {
	// Short circuit if the trie is already committed and not usable.
	if t.Committed {
		return nil, ErrCommitted
	}
	value, newroot, didResolve, err := t.RecursiveGet(t.Root, KeybytesToHex(key), 0)
	if err == nil && didResolve {
		t.Root = newroot
	}
	return value, err
}

// TODO: Debug for each node type
func (t *Trie) RecursiveGet(origNode Node, key []byte, pos int) (value []byte, newnode Node, didResolve bool, err error) {
	switch n := (origNode).(type) {
	case nil:
		return nil, nil, false, nil
	case ValueNode:
		return n, n, false, nil
	case *ShortNode:
		if len(key)-pos < len(n.Key) || !bytes.Equal(n.Key, key[pos:pos+len(n.Key)]) {
			// key not found in trie
			return nil, n, false, nil
		}
		value, newnode, didResolve, err = t.RecursiveGet(n.Val, key, pos+len(n.Key))
		if err == nil && didResolve {
			n = n.Copy()
			n.Val = newnode
		}
		return value, n, didResolve, err
	case *FullNode:
		value, newnode, didResolve, err = t.RecursiveGet(n.Children[key[pos]], key, pos+1)
		if err == nil && didResolve {
			n = n.Copy()
			n.Children[key[pos]] = newnode
		}
		return value, n, didResolve, err
	case HashNode:
		child, err := t.ResolveAndTrack(n, key[:pos])
		if err != nil {
			return nil, n, true, err
		}
		value, newnode, _, err := t.RecursiveGet(child, key, pos)
		return value, newnode, true, err
	default:
		panic(fmt.Sprintf("%T: invalid node: %v", origNode, origNode))
	}
}

// MustGetNode is a wrapper of GetNode and will omit any encountered error but
// just print out an error message.
func (t *Trie) MustGetNode(path []byte) ([]byte, int) {
	item, resolved, err := t.GetNode(path)
	if err != nil {
		log.Error("Unhandled trie error in Trie.GetNode", "err", err)
	}
	return item, resolved
}

// GetNode retrieves a trie node by compact-encoded path. It is not possible
// to use keybyte-encoding as the path might contain odd nibbles.
//
// If the requested node is not present in trie, no error will be returned.
// If the trie is corrupted, a MissingNodeError is returned.
func (t *Trie) GetNode(path []byte) ([]byte, int, error) {
	// Short circuit if the trie is already committed and not usable.
	if t.Committed {
		return nil, 0, ErrCommitted
	}
	item, newroot, resolved, err := t.RecursiveGetNode(t.Root, CompactToHex(path), 0)
	if err != nil {
		return nil, resolved, err
	}
	if resolved > 0 {
		t.Root = newroot
	}
	if item == nil {
		return nil, resolved, nil
	}
	return item, resolved, nil
}
func (t *Trie) RecursiveGetNode(origNode Node, path []byte, pos int) (item []byte, newnode Node, resolved int, err error) {
	// If non-existent path requested, abort
	if origNode == nil {
		return nil, nil, 0, nil
	}
	// If we reached the requested path, return the current node
	if pos >= len(path) {
		// Although we most probably have the original node expanded, encoding
		// that into consensus form can be nasty (needs to cascade down) and
		// time consuming. Instead, just pull the hash up from disk directly.
		var hash HashNode
		if node, ok := origNode.(HashNode); ok {
			hash = node
		} else {
			hash, _ = origNode.Cache()
		}
		if hash == nil {
			return nil, origNode, 0, errors.New("non-consensus node")
		}
		blob, err := t.Reader.Node(path, common.BytesToHash(hash))
		return blob, origNode, 1, err
	}
	// Path still needs to be traversed, descend into children
	switch n := (origNode).(type) {
	case ValueNode:
		// Path prematurely ended, abort
		return nil, nil, 0, nil

	case *ShortNode:
		if len(path)-pos < len(n.Key) || !bytes.Equal(n.Key, path[pos:pos+len(n.Key)]) {
			// Path branches off from short node
			return nil, n, 0, nil
		}
		item, newnode, resolved, err = t.RecursiveGetNode(n.Val, path, pos+len(n.Key))
		if err == nil && resolved > 0 {
			n = n.Copy()
			n.Val = newnode
		}
		return item, n, resolved, err

	case *FullNode:
		item, newnode, resolved, err = t.RecursiveGetNode(n.Children[path[pos]], path, pos+1)
		if err == nil && resolved > 0 {
			n = n.Copy()
			n.Children[path[pos]] = newnode
		}
		return item, n, resolved, err

	case HashNode:
		child, err := t.ResolveAndTrack(n, path[:pos])
		if err != nil {
			return nil, n, 1, err
		}
		item, newnode, resolved, err := t.RecursiveGetNode(child, path, pos)
		return item, newnode, resolved + 1, err

	default:
		panic(fmt.Sprintf("%T: invalid node: %v", origNode, origNode))
	}
}

// MustUpdate is a wrapper of Update and will omit any encountered error but
// just print out an error message.
func (t *Trie) MustUpdate(key, value []byte) {
	if err := t.Update(key, value); err != nil {
		log.Error("Unhandled trie error in Trie.Update", "err", err)
	}
}

// Update associates key with value in the trie. Subsequent calls to
// Get will return value. If value has length zero, any existing value
// is deleted from the trie and calls to Get will return nil.
//
// The value bytes must not be modified by the caller while they are
// stored in the trie.
//
// If the requested node is not present in trie, no error will be returned.
// If the trie is corrupted, a MissingNodeError is returned.
func (t *Trie) Update(key, value []byte) error {
	// Short circuit if the trie is already committed and not usable.
	if t.Committed {
		return ErrCommitted
	}
	return t.TryUpdate(key, value)
}

func (t *Trie) TryUpdate(key, value []byte) error {
	t.Unhashed++
	k := KeybytesToHex(key)
	if len(value) != 0 {
		_, n, err := t.Insert(t.Root, nil, k, ValueNode(value))
		if err != nil {
			return err
		}
		t.Root = n
	} else {
		_, n, err := t.TryDelete(t.Root, nil, k)
		if err != nil {
			return err
		}
		t.Root = n
	}
	return nil
}

func (t *Trie) Insert(n Node, prefix, key []byte, value Node) (bool, Node, error) {
	if len(key) == 0 {
		if v, ok := n.(ValueNode); ok {
			return !bytes.Equal(v, value.(ValueNode)), value, nil
		}
		return true, value, nil
	}
	switch n := n.(type) {
	case *ShortNode:
		matchlen := PrefixLen(key, n.Key)
		// If the whole key matches, keep this short node as is
		// and only TryUpdate the value.
		if matchlen == len(n.Key) {
			dirty, nn, err := t.Insert(n.Val, append(prefix, key[:matchlen]...), key[matchlen:], value)
			if !dirty || err != nil {
				return false, n, err
			}
			return true, &ShortNode{n.Key, nn, t.NewFlag()}, nil
		}
		// Otherwise branch out at the index where they differ.
		branch := &FullNode{Flags: t.NewFlag()}
		var err error
		_, branch.Children[n.Key[matchlen]], err = t.Insert(nil, append(prefix, n.Key[:matchlen+1]...), n.Key[matchlen+1:], n.Val)
		if err != nil {
			return false, nil, err
		}
		_, branch.Children[key[matchlen]], err = t.Insert(nil, append(prefix, key[:matchlen+1]...), key[matchlen+1:], value)
		if err != nil {
			return false, nil, err
		}
		// Replace this shortNode with the branch if it occurs at index 0.
		if matchlen == 0 {
			return true, branch, nil
		}
		// New branch node is created as a child of the original short node.
		// Track the newly inserted node in the tracer. The node identifier
		// passed is the path from the root node.
		t.Tracer.OnInsert(append(prefix, key[:matchlen]...))

		// Replace it with a short node leading up to the branch.
		return true, &ShortNode{key[:matchlen], branch, t.NewFlag()}, nil

	case *FullNode: // TODO: When does this happen?
		dirty, nn, err := t.Insert(n.Children[key[0]], append(prefix, key[0]), key[1:], value)
		if !dirty || err != nil {
			return false, n, err
		}
		n = n.Copy()
		n.Flags = t.NewFlag()
		n.Children[key[0]] = nn
		return true, n, nil

	case nil: // TODO: When does this happen?
		// New short node is created and track it in the tracer. The node identifier
		// passed is the path from the root node. Note the valueNode won't be tracked
		// since it's always embedded in its parent.
		t.Tracer.OnInsert(prefix)

		return true, &ShortNode{key, value, t.NewFlag()}, nil

	case HashNode:
		// We've hit a part of the trie that isn't loaded yet. Load
		// the node and Insert into it. This leaves all child nodes on
		// the path to the value in the trie.
		rn, err := t.ResolveAndTrack(n, prefix)
		if err != nil {
			return false, nil, err
		}
		dirty, nn, err := t.Insert(rn, prefix, key, value)
		if !dirty || err != nil {
			return false, rn, err
		}
		return true, nn, nil

	default:
		panic(fmt.Sprintf("%T: invalid node: %v", n, n))
	}
}

// MustDelete is a wrapper of Delete and will omit any encountered error but
// just print out an error message.
func (t *Trie) MustDelete(key []byte) {
	if err := t.Delete(key); err != nil {
		log.Error("Unhandled trie error in Trie.Delete", "err", err)
	}
}

// Delete removes any existing value for key from the trie.
//
// If the requested node is not present in trie, no error will be returned.
// If the trie is corrupted, a MissingNodeError is returned.
func (t *Trie) Delete(key []byte) error {
	// Short circuit if the trie is already committed and not usable.
	if t.Committed {
		return ErrCommitted
	}
	t.Unhashed++
	k := KeybytesToHex(key)
	_, n, err := t.TryDelete(t.Root, nil, k)
	if err != nil {
		return err
	}
	t.Root = n
	return nil
}

// TryDelete returns the new root of the trie with key deleted.
// It reduces the trie to minimal form by simplifying
// nodes on the way up after deleting recursively.
func (t *Trie) TryDelete(n Node, prefix, key []byte) (bool, Node, error) {
	switch n := n.(type) {
	case *ShortNode:
		matchlen := PrefixLen(key, n.Key)
		if matchlen < len(n.Key) {
			return false, n, nil // don't replace n on mismatch
		}
		if matchlen == len(key) {
			// The matched short node is deleted entirely and track
			// it in the deletion set. The same the valueNode doesn't
			// need to be tracked at all since it's always embedded.
			t.Tracer.OnDelete(prefix)

			return true, nil, nil // remove n entirely for whole matches
		}
		// The key is longer than n.Key. Remove the remaining suffix
		// from the subtrie. Child can never be nil here since the
		// subtrie must contain at least two other values with keys
		// longer than n.Key.
		dirty, child, err := t.TryDelete(n.Val, append(prefix, key[:len(n.Key)]...), key[len(n.Key):])
		if !dirty || err != nil {
			return false, n, err
		}
		switch child := child.(type) {
		case *ShortNode:
			// The child shortNode is merged into its parent, track
			// is deleted as well.
			t.Tracer.OnDelete(append(prefix, n.Key...))

			// Deleting from the subtrie reduced it to another
			// short node. Merge the nodes to avoid creating a
			// shortNode{..., shortNode{...}}. Use concat (which
			// always creates a new slice) instead of append to
			// avoid modifying n.Key since it might be shared with
			// other nodes.
			return true, &ShortNode{Concat(n.Key, child.Key...), child.Val, t.NewFlag()}, nil
		default:
			return true, &ShortNode{n.Key, child, t.NewFlag()}, nil
		}

	case *FullNode:
		dirty, nn, err := t.TryDelete(n.Children[key[0]], append(prefix, key[0]), key[1:])
		if !dirty || err != nil {
			return false, n, err
		}
		n = n.Copy()
		n.Flags = t.NewFlag()
		n.Children[key[0]] = nn

		// Because n is a full node, it must've contained at least two children
		// before the delete operation. If the new child value is non-nil, n still
		// has at least two children after the deletion, and cannot be reduced to
		// a short node.
		if nn != nil {
			return true, n, nil
		}
		// Reduction:
		// Check how many non-nil entries are left after deleting and
		// reduce the full node to a short node if only one entry is
		// left. Since n must've contained at least two children
		// before deletion (otherwise it would not be a full node) n
		// can never be reduced to nil.
		//
		// When the loop is done, pos contains the index of the single
		// value that is left in n or -2 if n contains at least two
		// values.
		pos := -1
		for i, cld := range &n.Children {
			if cld != nil {
				if pos == -1 {
					pos = i
				} else {
					pos = -2
					break
				}
			}
		}
		if pos >= 0 {
			if pos != 16 {
				// If the remaining entry is a short node, it replaces
				// n and its key gets the missing nibble tacked to the
				// front. This avoids creating an invalid
				// shortNode{..., shortNode{...}}.  Since the entry
				// might not be loaded yet, resolve it just for this
				// check.
				cnode, err := t.Resolve(n.Children[pos], append(prefix, byte(pos)))
				if err != nil {
					return false, nil, err
				}
				if cnode, ok := cnode.(*ShortNode); ok {
					// Replace the entire full node with the short node.
					// Mark the original short node as deleted since the
					// value is embedded into the parent now.
					t.Tracer.OnDelete(append(prefix, byte(pos)))

					k := append([]byte{byte(pos)}, cnode.Key...)
					return true, &ShortNode{k, cnode.Val, t.NewFlag()}, nil
				}
			}
			// Otherwise, n is replaced by a one-nibble short node
			// containing the child.
			return true, &ShortNode{[]byte{byte(pos)}, n.Children[pos], t.NewFlag()}, nil
		}
		// n still contains at least two values and cannot be reduced.
		return true, n, nil

	case ValueNode:
		return true, nil, nil

	case nil:
		return false, nil, nil

	case HashNode:
		// We've hit a part of the trie that isn't loaded yet. Load
		// the node and delete from it. This leaves all child nodes on
		// the path to the value in the trie.
		rn, err := t.ResolveAndTrack(n, prefix)
		if err != nil {
			return false, nil, err
		}
		dirty, nn, err := t.TryDelete(rn, prefix, key)
		if !dirty || err != nil {
			return false, rn, err
		}
		return true, nn, nil

	default:
		panic(fmt.Sprintf("%T: invalid node: %v (%v)", n, n, key))
	}
}

func Concat(s1 []byte, s2 ...byte) []byte {
	r := make([]byte, len(s1)+len(s2))
	copy(r, s1)
	copy(r[len(s1):], s2)
	return r
}

// ResolveAndTrack loads node from the underlying store with the given node hash
// and path prefix and also tracks the loaded node blob in tracer treated as the
// node's original value. The rlp-encoded blob is preferred to be loaded from
// database because it's easy to decode node while complex to encode node to blob.
func (t *Trie) ResolveAndTrack(n HashNode, prefix []byte) (Node, error) {
	blob, err := t.Reader.Node(prefix, common.BytesToHash(n))
	if err != nil {
		return nil, err
	}
	t.Tracer.OnRead(prefix, blob)
	return MustDecodeNode(n, blob), nil
}

func (t *Trie) Resolve(n Node, prefix []byte) (Node, error) {
	if n, ok := n.(HashNode); ok {
		return t.ResolveAndTrack(n, prefix)
	}
	return n, nil
}

// hashRoot calculates the root hash of the given trie
func (t *Trie) HashRoot() (Node, Node) {
	if t.Root == nil {
		return HashNode(types.EmptyRootHash.Bytes()), nil
	}
	// If the number of changes is below 100, we let one thread handle it
	h := NewHasher(t.Unhashed >= 100)
	defer func() {
		ReturnHasherToPool(h)
		t.Unhashed = 0
	}()
	hashed, cached := h.Hash(t.Root, true)
	return hashed, cached
}

// Hash returns the root hash of the trie. It does not write to the
// database and can be used even if the trie doesn't have one.
func (t *Trie) Hash() common.Hash {
	hash, cached := t.HashRoot()
	t.Root = cached
	return common.BytesToHash(hash.(HashNode))
}

// Commit collects all dirty nodes in the trie and replaces them with the
// corresponding node hash. All collected nodes (including dirty leaves if
// collectLeaf is true) will be encapsulated into a nodeset for return.
// The returned nodeset can be nil if the trie is clean (nothing to commit).
// Once the trie is committed, it's not usable anymore. A new trie must
// be created with new root and updated trie database for following usage
func (t *Trie) Commit(collectLeaf bool) (common.Hash, *trienode.NodeSet) {
	defer func() {
		t.Committed = true
	}()
	// Trie is empty and can be classified into two types of situations:
	// (a) The trie was empty and no update happens => return nil
	// (b) The trie was non-empty and all nodes are dropped => return
	//     the node set includes all deleted nodes
	if t.Root == nil {
		paths := t.Tracer.DeletedNodes()
		if len(paths) == 0 {
			return types.EmptyRootHash, nil // case (a)
		}
		nodes := trienode.NewNodeSet(t.Owner)
		for _, path := range paths {
			nodes.AddNode([]byte(path), trienode.NewDeleted())
		}
		return types.EmptyRootHash, nodes // case (b)
	}
	// Derive the hash for all dirty nodes first. We hold the assumption
	// in the following procedure that all nodes are hashed.
	rootHash := t.Hash()

	// Do a quick check if we really need to commit. This can happen e.g.
	// if we load a trie for reading storage values, but don't write to it.
	if hashedNode, dirty := t.Root.Cache(); !dirty {
		// Replace the root node with the origin hash in order to
		// ensure all resolved nodes are dropped after the commit.
		t.Root = hashedNode
		return rootHash, nil
	}
	nodes := trienode.NewNodeSet(t.Owner)
	for _, path := range t.Tracer.DeletedNodes() {
		nodes.AddNode([]byte(path), trienode.NewDeleted())
	}
	t.Root = NewCommitter(nodes, t.Tracer, collectLeaf).Commit(t.Root)
	return rootHash, nodes
}
