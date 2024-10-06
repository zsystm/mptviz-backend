package trie

import (
	"bytes"
	"errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// NodeResolver is used for looking up trie nodes before reaching into the real
// persistent layer. This is not mandatory, rather is an optimization for cases
// where trie nodes can be recovered from some external mechanism without reading
// from disk. In those cases, this resolver allows short circuiting accesses and
// returning them from memory.
type NodeResolver func(owner common.Hash, path []byte, hash common.Hash) []byte

// NodeIterator is an iterator to traverse the trie pre-order.
type NodeIterator interface {
	// Next moves the iterator to the next node. If the parameter is false, any child
	// nodes will be skipped.
	Next(bool) bool

	// Error returns the error status of the iterator.
	Error() error

	// Hash returns the hash of the current node.
	Hash() common.Hash

	// Parent returns the hash of the parent of the current node. The hash may be the one
	// grandparent if the immediate parent is an internal node with no hash.
	Parent() common.Hash

	// Path returns the hex-encoded path to the current node.
	// Callers must not retain references to the return value after calling Next.
	// For leaf nodes, the last element of the path is the 'terminator symbol' 0x10.
	Path() []byte

	// NodeBlob returns the rlp-encoded value of the current iterated node.
	// If the node is an embedded node in its parent, nil is returned then.
	NodeBlob() []byte

	// Leaf returns true if the current node is a leaf node.
	Leaf() bool

	// LeafKey returns the key of the leaf. The method panics if the iterator is not
	// positioned at a leaf. Callers must not retain references to the value after
	// calling Next.
	LeafKey() []byte

	// LeafBlob returns the content of the leaf. The method panics if the iterator
	// is not positioned at a leaf. Callers must not retain references to the value
	// after calling Next.
	LeafBlob() []byte

	// LeafProof returns the Merkle proof of the leaf. The method panics if the
	// iterator is not positioned at a leaf. Callers must not retain references
	// to the value after calling Next.
	LeafProof() [][]byte

	// AddResolver sets a node resolver to use for looking up trie nodes before
	// reaching into the real persistent layer.
	//
	// This is not required for normal operation, rather is an optimization for
	// cases where trie nodes can be recovered from some external mechanism without
	// reading from disk. In those cases, this resolver allows short circuiting
	// accesses and returning them from memory.
	//
	// Before adding a similar mechanism to any other place in Geth, consider
	// making trie.Database an interface and wrapping at that level. It's a huge
	// refactor, but it could be worth it if another occurrence arises.
	AddResolver(NodeResolver)
}

// Iterator is a key-value trie iterator that traverses a Trie.
type Iterator struct {
	nodeIt NodeIterator

	Key   []byte // Current data key on which the iterator is positioned on
	Value []byte // Current data value on which the iterator is positioned on
	Err   error
}

// NewIterator creates a new key-value iterator from a node iterator.
// Note that the value returned by the iterator is raw. If the content is encoded
// (e.g. storage value is RLP-encoded), it's caller's duty to decode it.
func NewIterator(it NodeIterator) *Iterator {
	return &Iterator{
		nodeIt: it,
	}
}

// Next moves the iterator forward one key-value entry.
func (it *Iterator) Next() bool {
	for it.nodeIt.Next(true) {
		if it.nodeIt.Leaf() {
			it.Key = it.nodeIt.LeafKey()
			it.Value = it.nodeIt.LeafBlob()
			return true
		}
	}
	it.Key = nil
	it.Value = nil
	it.Err = it.nodeIt.Error()
	return false
}

// Prove generates the Merkle proof for the leaf node the iterator is currently
// positioned on.
func (it *Iterator) Prove() [][]byte {
	return it.nodeIt.LeafProof()
}

// NodeIteratorState represents the iteration state at one particular node of the
// trie, which can be resumed at a later invocation.
type NodeIteratorState struct {
	Hash    common.Hash // Hash of the node being iterated (nil if not standalone)
	Node    Node        // Trie node being iterated
	Parent  common.Hash // Hash of the first full ancestor node (nil if current is the root)
	Index   int         // Child to be processed next
	Pathlen int         // Length of the path to the parent node
}

type DefaultNodeIterator struct {
	Trie  *Trie                // Trie being iterated
	Stack []*NodeIteratorState // Hierarchy of trie nodes persisting the iteration state
	path  []byte               // Path to the current node
	Err   error                // Failure set in case of an internal error in the iterator

	Resolver NodeResolver         // optional node resolver for avoiding disk hits
	Pool     []*NodeIteratorState // local pool for iterator states
}

// ErrIteratorEnd is stored in nodeIterator.err when iteration is done.
var ErrIteratorEnd = errors.New("end of iteration")

// SeekError is stored in nodeIterator.err if the initial seek has failed.
type SeekError struct {
	Key []byte
	Err error
}

func (e SeekError) Error() string {
	return "seek error: " + e.Err.Error()
}

func NewDefaultNodeIterator(trie *Trie, start []byte) NodeIterator {
	if trie.Hash() == types.EmptyRootHash {
		return &DefaultNodeIterator{
			Trie: trie,
			Err:  ErrIteratorEnd,
		}
	}
	it := &DefaultNodeIterator{Trie: trie}
	it.Err = it.Seek(start)
	return it
}

func (it *DefaultNodeIterator) Seek(prefix []byte) error {
	// The path we're looking for is the hex encoded key without terminator.
	key := KeybytesToHex(prefix)
	key = key[:len(key)-1]

	// Move forward until we're just before the closest match to key.
	for {
		state, parentIndex, path, err := it.PeekSeek(key)
		if err == ErrIteratorEnd {
			return ErrIteratorEnd
		} else if err != nil {
			return SeekError{prefix, err}
		} else if ReachedPath(path, key) {
			return nil
		}
		it.Push(state, parentIndex, path)
	}
}

func (it *DefaultNodeIterator) PutInPool(item *NodeIteratorState) {
	if len(it.Pool) < 40 {
		item.Node = nil
		it.Pool = append(it.Pool, item)
	}
}

func (it *DefaultNodeIterator) GetFromPool() *NodeIteratorState {
	idx := len(it.Pool) - 1
	if idx < 0 {
		return new(NodeIteratorState)
	}
	item := it.Pool[idx]
	it.Pool[idx] = nil
	it.Pool = it.Pool[:idx]
	return item
}

func (it *DefaultNodeIterator) AddResolver(resolver NodeResolver) {
	it.Resolver = resolver
}

func (it *DefaultNodeIterator) Hash() common.Hash {
	if len(it.Stack) == 0 {
		return common.Hash{}
	}
	return it.Stack[len(it.Stack)-1].Hash
}

func (it *DefaultNodeIterator) Parent() common.Hash {
	if len(it.Stack) == 0 {
		return common.Hash{}
	}
	return it.Stack[len(it.Stack)-1].Parent
}

func (it *DefaultNodeIterator) Leaf() bool {
	return HasTerm(it.Path())
}

func (it *DefaultNodeIterator) LeafKey() []byte {
	if len(it.Stack) > 0 {
		if _, ok := it.Stack[len(it.Stack)-1].Node.(*ValueNode); ok {
			return HexToKeybytes(it.Path())
		}
	}
	return it.Path()
}

func (it *DefaultNodeIterator) LeafBlob() []byte {
	if len(it.Stack) > 0 {
		if node, ok := it.Stack[len(it.Stack)-1].Node.(ValueNode); ok {
			return node
		}
	}
	panic("not a leaf node")
}

func (it *DefaultNodeIterator) LeafProof() [][]byte {
	if len(it.Stack) > 0 {
		if _, ok := it.Stack[len(it.Stack)-1].Node.(ValueNode); ok {
			hasher := NewHasher(false)
			defer ReturnHasherToPool(hasher)
			proofs := make([][]byte, 0, len(it.Stack))

			for i, item := range it.Stack[:len(it.Stack)-1] {
				// Gather nodes that end up as hash nodes (or the root)
				node, hashed := hasher.ProofHash(item.Node)
				if _, ok := hashed.(HashNode); ok || i == 0 {
					proofs = append(proofs, nodeToBytes(node))
				}
			}
			return proofs
		}
	}
	panic("not at leaf")

}

func (it *DefaultNodeIterator) Path() []byte {
	return it.path
}

func (it *DefaultNodeIterator) NodeBlob() []byte {
	if it.Hash() == (common.Hash{}) {
		return nil // skip the non-standalone node
	}
	blob, err := it.ResolveBlob(it.Hash().Bytes(), it.Path())
	if err != nil {
		it.Err = err
		return nil
	}
	return blob
}

func (it *DefaultNodeIterator) Error() error {
	if it.Err == ErrIteratorEnd {
		return nil
	}
	if seek, ok := it.Err.(SeekError); ok {
		return seek.Err
	}
	return it.Err
}

// Next moves the iterator to the next node, returning whether there are any
// further nodes. In case of an internal error this method returns false and
// sets the Error field to the encountered failure. If `descend` is false,
// skips iterating over any subnodes of the current node.
func (it *DefaultNodeIterator) Next(descend bool) bool {
	if it.Err == ErrIteratorEnd {
		return false
	}
	if seek, ok := it.Err.(SeekError); ok {
		if it.Err = it.Seek(seek.Key); it.Err != nil {
			return false
		}
	}
	// Otherwise step forward with the iterator and report any errors.
	state, parentIndex, path, err := it.Peek(descend)
	it.Err = err
	if it.Err != nil {
		return false
	}
	it.Push(state, parentIndex, path)
	return true
}

func (it *DefaultNodeIterator) seek(prefix []byte) error {
	// The path we're looking for is the hex encoded key without terminator.
	key := KeybytesToHex(prefix)
	key = key[:len(key)-1]

	// Move forward until we're just before the closest match to key.
	for {
		state, parentIndex, path, err := it.PeekSeek(key)
		if err == ErrIteratorEnd {
			return ErrIteratorEnd
		} else if err != nil {
			return SeekError{prefix, err}
		} else if ReachedPath(path, key) {
			return nil
		}
		it.Push(state, parentIndex, path)
	}
}

// init initializes the iterator.
func (it *DefaultNodeIterator) init() (*NodeIteratorState, error) {
	root := it.Trie.Hash()
	state := &NodeIteratorState{Node: it.Trie.Root, Index: -1}
	if root != types.EmptyRootHash {
		state.Hash = root
	}
	return state, state.Resolve(it, nil)
}

// Peek creates the next state of the iterator.
func (it *DefaultNodeIterator) Peek(descend bool) (*NodeIteratorState, *int, []byte, error) {
	// Initialize the iterator if we've just started.
	if len(it.Stack) == 0 {
		state, err := it.init()
		return state, nil, nil, err
	}
	if !descend {
		// If we're skipping children, pop the current node first
		it.Pop()
	}
	// Continue iteration to the next child
	for len(it.Stack) > 0 {
		parent := it.Stack[len(it.Stack)-1]
		ancestor := parent.Hash
		if (ancestor == common.Hash{}) {
			ancestor = parent.Parent
		}
		state, path, ok := it.NextChild(parent, ancestor)
		if ok {
			if err := state.Resolve(it, path); err != nil {
				return parent, &parent.Index, path, err
			}
			return state, &parent.Index, path, nil
		}
		// No more child nodes, move back up.
		it.Pop()
	}
	return nil, nil, nil, ErrIteratorEnd
}

// PeekSeek is like peek, but it also tries to skip resolving hashes by skipping
// over the siblings that do not lead towards the desired seek position.
func (it *DefaultNodeIterator) PeekSeek(seekKey []byte) (*NodeIteratorState, *int, []byte, error) {
	// Initialize the iterator if we've just started.
	if len(it.Stack) == 0 {
		state, err := it.init()
		return state, nil, nil, err
	}
	if !bytes.HasPrefix(seekKey, it.Path()) {
		// If we're skipping children, pop the current node first
		it.Pop()
	}
	// Continue iteration to the next child
	for len(it.Stack) > 0 {
		parent := it.Stack[len(it.Stack)-1]
		ancestor := parent.Hash
		if (ancestor == common.Hash{}) {
			ancestor = parent.Parent
		}
		state, path, ok := it.NextChildAt(parent, ancestor, seekKey)
		if ok {
			if err := state.Resolve(it, path); err != nil {
				return parent, &parent.Index, path, err
			}
			return state, &parent.Index, path, nil
		}
		// No more child nodes, move back up.
		it.Pop()
	}
	return nil, nil, nil, ErrIteratorEnd
}

func (it *DefaultNodeIterator) ResolveHash(hash HashNode, path []byte) (Node, error) {
	if it.Resolver != nil {
		if blob := it.Resolver(it.Trie.Owner, path, common.BytesToHash(hash)); len(blob) > 0 {
			if resolved, err := DecodeNode(hash, blob); err == nil {
				return resolved, nil
			}
		}
	}
	// Retrieve the specified node from the underlying node reader.
	// it.trie.resolveAndTrack is not used since in that function the
	// loaded blob will be tracked, while it's not required here since
	// all loaded nodes won't be linked to trie at all and track nodes
	// may lead to out-of-memory issue.
	blob, err := it.Trie.Reader.Node(path, common.BytesToHash(hash))
	if err != nil {
		return nil, err
	}
	// The raw-blob format nodes are loaded either from the
	// clean cache or the database, they are all in their own
	// copy and safe to use unsafe decoder.
	return MustDecodeNodeUnsafe(hash, blob), nil
}

func (it *DefaultNodeIterator) ResolveBlob(hash HashNode, path []byte) ([]byte, error) {
	if it.Resolver != nil {
		if blob := it.Resolver(it.Trie.Owner, path, common.BytesToHash(hash)); len(blob) > 0 {
			return blob, nil
		}
	}
	// Retrieve the specified node from the underlying node reader.
	// it.trie.resolveAndTrack is not used since in that function the
	// loaded blob will be tracked, while it's not required here since
	// all loaded nodes won't be linked to trie at all and track nodes
	// may lead to out-of-memory issue.
	return it.Trie.Reader.Node(path, common.BytesToHash(hash))
}

func (st *NodeIteratorState) Resolve(it *DefaultNodeIterator, path []byte) error {
	if hash, ok := st.Node.(HashNode); ok {
		resolved, err := it.ResolveHash(hash, path)
		if err != nil {
			return err
		}
		st.Node = resolved
		st.Hash = common.BytesToHash(hash)
	}
	return nil
}

func (it *DefaultNodeIterator) FindChild(n *FullNode, index int, ancestor common.Hash) (Node, *NodeIteratorState, []byte, int) {
	var (
		path      = it.Path()
		child     Node
		state     *NodeIteratorState
		childPath []byte
	)
	for ; index < len(n.Children); index = NextChildIndex(index) {
		if n.Children[index] != nil {
			child = n.Children[index]
			hash, _ := child.Cache()

			state = it.GetFromPool()
			state.Hash = common.BytesToHash(hash)
			state.Node = child
			state.Parent = ancestor
			state.Index = -1
			state.Pathlen = len(path)

			childPath = append(childPath, path...)
			childPath = append(childPath, byte(index))
			return child, state, childPath, index
		}
	}
	return nil, nil, nil, 0
}

func (it *DefaultNodeIterator) NextChild(parent *NodeIteratorState, ancestor common.Hash) (*NodeIteratorState, []byte, bool) {
	switch node := parent.Node.(type) {
	case *FullNode:
		// Full node, move to the first non-nil child.
		if child, state, path, index := it.FindChild(node, NextChildIndex(parent.Index), ancestor); child != nil {
			parent.Index = PrevChildIndex(index)
			return state, path, true
		}
	case *ShortNode:
		// Short node, return the pointer singleton child
		if parent.Index < 0 {
			hash, _ := node.Val.Cache()
			state := it.GetFromPool()
			state.Hash = common.BytesToHash(hash)
			state.Node = node.Val
			state.Parent = ancestor
			state.Index = -1
			state.Pathlen = len(it.Path())
			path := append(it.Path(), node.Key...)
			return state, path, true
		}
	}
	return parent, it.Path(), false
}

// NextChildAt is similar to nextChild, except that it targets a child as close to the
// target key as possible, thus skipping siblings.
func (it *DefaultNodeIterator) NextChildAt(parent *NodeIteratorState, ancestor common.Hash, key []byte) (*NodeIteratorState, []byte, bool) {
	switch n := parent.Node.(type) {
	case *FullNode:
		// Full node, move to the first non-nil child before the desired key position
		child, state, path, index := it.FindChild(n, NextChildIndex(parent.Index), ancestor)
		if child == nil {
			// No more children in this fullnode
			return parent, it.Path(), false
		}
		// If the child we found is already past the seek position, just return it.
		if ReachedPath(path, key) {
			parent.Index = PrevChildIndex(index)
			return state, path, true
		}
		// The child is before the seek position. Try advancing
		for {
			nextChild, nextState, nextPath, nextIndex := it.FindChild(n, NextChildIndex(index), ancestor)
			// If we run out of children, or skipped past the target, return the
			// previous one
			if nextChild == nil || ReachedPath(nextPath, key) {
				parent.Index = PrevChildIndex(index)
				return state, path, true
			}
			// We found a better child closer to the target
			state, path, index = nextState, nextPath, nextIndex
		}
	case *ShortNode:
		// Short node, return the pointer singleton child
		if parent.Index < 0 {
			hash, _ := n.Val.Cache()
			state := it.GetFromPool()
			state.Hash = common.BytesToHash(hash)
			state.Node = n.Val
			state.Parent = ancestor
			state.Index = -1
			state.Pathlen = len(it.Path())
			path := append(it.Path(), n.Key...)
			return state, path, true
		}
	}
	return parent, it.Path(), false
}

func (it *DefaultNodeIterator) Push(state *NodeIteratorState, parentIndex *int, path []byte) {
	it.path = path
	it.Stack = append(it.Stack, state)
	if parentIndex != nil {
		*parentIndex = NextChildIndex(*parentIndex)
	}
}

func (it *DefaultNodeIterator) Pop() {
	last := it.Stack[len(it.Stack)-1]
	it.path = it.path[:last.Pathlen]
	it.Stack[len(it.Stack)-1] = nil
	it.Stack = it.Stack[:len(it.Stack)-1]

	it.PutInPool(last) // last is now unused
}

// NextChildIndex returns the index of a child in a full node which follows
// the given index when performing a pre-order traversal.
func NextChildIndex(index int) int {
	switch index {
	case -1: // Jump from the placeholder index to the embedded value slot
		return 16
	case 15: // Skip the value slot after iterating the children
		return 17
	case 16: // From the embedded value slot, jump back to iterate the children
		return 0
	default: // Iterate children in sequence
		return index + 1
	}
}

// ReachedPath normalizes a path by truncating a terminator if present, and
// returns true if it is greater than or equal to the target. Using this,
// the path of a value node embedded a full node will compare less than the
// full node's children.
func ReachedPath(path, target []byte) bool {
	if HasTerm(path) {
		path = path[:len(path)-1]
	}
	return bytes.Compare(path, target) >= 0
}

// A value embedded in a full node occupies the last slot (16) of the array of
// children. In order to produce a pre-order traversal when iterating children,
// we jump to this last slot first, then go back iterate the child nodes (and
// skip the last slot at the end):

// PrevChildIndex returns the index of a child in a full node which precedes
// the given index when performing a pre-order traversal.
func PrevChildIndex(index int) int {
	switch index {
	case 0: // We jumped back to iterate the children, from the value slot
		return 16
	case 16: // We jumped to the embedded value slot at the end, from the placeholder index
		return -1
	case 17: // We skipped the value slot after iterating all the children
		return 15
	default: // We are iterating the children in sequence
		return index - 1
	}
}
