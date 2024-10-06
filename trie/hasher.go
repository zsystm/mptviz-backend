package trie

import (
	"sync"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

// Hasher is a type used for the trie Hash operation. A hasher has some
// internal preallocated temp space
type Hasher struct {
	Sha      crypto.KeccakState
	Tmp      []byte
	Encbuf   rlp.EncoderBuffer
	Parallel bool // Whether to use parallel threads when hashing
}

// HasherPool holds pureHashers
var HasherPool = sync.Pool{
	New: func() interface{} {
		return &Hasher{
			Tmp:    make([]byte, 0, 550), // cap is as large as a full fullNode.
			Sha:    crypto.NewKeccakState(),
			Encbuf: rlp.NewEncoderBuffer(nil),
		}
	},
}

func NewHasher(parallel bool) *Hasher {
	h := HasherPool.Get().(*Hasher)
	h.Parallel = parallel
	return h
}

func ReturnHasherToPool(h *Hasher) {
	HasherPool.Put(h)
}

// Hash collapses a node down into a hash node, also returning a copy of the
// original node initialized with the computed hash to replace the original one.
func (h *Hasher) Hash(n Node, force bool) (hashed Node, cached Node) {
	// Return the cached hash if it's available
	if hash, _ := n.Cache(); hash != nil {
		return hash, n
	}
	// Trie not processed yet, walk the children
	switch n := n.(type) {
	case *ShortNode:
		collapsed, cached := h.HashShortNodeChildren(n)
		hashed := h.ShortnodeToHash(collapsed, force)
		// We need to retain the possibly _not_ hashed node, in case it was too
		// small to be hashed
		if hn, ok := hashed.(HashNode); ok {
			cached.Flags.Hash = hn
		} else {
			cached.Flags.Hash = nil
		}
		return hashed, cached
	case *FullNode:
		collapsed, cached := h.HashFullNodeChildren(n)
		hashed = h.FullnodeToHash(collapsed, force)
		if hn, ok := hashed.(HashNode); ok {
			cached.Flags.Hash = hn
		} else {
			cached.Flags.Hash = nil
		}
		return hashed, cached
	default:
		// Value and hash nodes don't have children, so they're left as were
		return n, n
	}
}

// HashShortNodeChildren collapses the short node. The returned collapsed node
// holds a live reference to the Key, and must not be modified.
func (h *Hasher) HashShortNodeChildren(n *ShortNode) (collapsed, cached *ShortNode) {
	// Hash the short node's child, caching the newly hashed subtree
	collapsed, cached = n.Copy(), n.Copy()
	// Previously, we did copy this one. We don't seem to need to actually
	// do that, since we don't overwrite/reuse keys
	// cached.Key = common.CopyBytes(n.Key)
	collapsed.Key = HexToCompact(n.Key)
	// Unless the child is a valuenode or hashnode, hash it
	switch n.Val.(type) {
	case *FullNode, *ShortNode:
		collapsed.Val, cached.Val = h.Hash(n.Val, false)
	}
	return collapsed, cached
}

func (h *Hasher) HashFullNodeChildren(n *FullNode) (collapsed *FullNode, cached *FullNode) {
	// Hash the full node's children, caching the newly hashed subtrees
	cached = n.Copy()
	collapsed = n.Copy()
	if h.Parallel {
		var wg sync.WaitGroup
		wg.Add(16)
		for i := 0; i < 16; i++ {
			go func(i int) {
				hasher := NewHasher(false)
				if child := n.Children[i]; child != nil {
					collapsed.Children[i], cached.Children[i] = hasher.Hash(child, false)
				} else {
					collapsed.Children[i] = NilValueNode
				}
				ReturnHasherToPool(hasher)
				wg.Done()
			}(i)
		}
		wg.Wait()
	} else {
		for i := 0; i < 16; i++ {
			if child := n.Children[i]; child != nil {
				collapsed.Children[i], cached.Children[i] = h.Hash(child, false)
			} else {
				collapsed.Children[i] = NilValueNode
			}
		}
	}
	return collapsed, cached
}

// ShortnodeToHash creates a hashNode from a shortNode. The supplied shortnode
// should have hex-type Key, which will be converted (without modification)
// into compact form for RLP encoding.
// If the rlp data is smaller than 32 bytes, `nil` is returned.
func (h *Hasher) ShortnodeToHash(n *ShortNode, force bool) Node {
	n.Encode(h.Encbuf)
	enc := h.EncodedBytes()

	if len(enc) < 32 && !force {
		return n // Nodes smaller than 32 bytes are stored inside their parent
	}
	return h.HashData(enc)
}

// FullnodeToHash is used to create a hashNode from a fullNode, (which
// may contain nil values)
func (h *Hasher) FullnodeToHash(n *FullNode, force bool) Node {
	n.Encode(h.Encbuf)
	enc := h.EncodedBytes()

	if len(enc) < 32 && !force {
		return n // Nodes smaller than 32 bytes are stored inside their parent
	}
	return h.HashData(enc)
}

// EncodedBytes returns the result of the last encoding operation on h.Encbuf.
// This also resets the encoder buffer.
//
// All node encoding must be done like this:
//
//	node.Encode(h.Encbuf)
//	enc := h.EncodedBytes()
//
// This convention exists because node.Encode can only be inlined/escape-analyzed when
// called on a concrete receiver type.
func (h *Hasher) EncodedBytes() []byte {
	h.Tmp = h.Encbuf.AppendToBytes(h.Tmp[:0])
	h.Encbuf.Reset(nil)
	return h.Tmp
}

// HashData hashes the provided data
func (h *Hasher) HashData(data []byte) HashNode {
	n := make(HashNode, 32)
	h.Sha.Reset()
	h.Sha.Write(data)
	h.Sha.Read(n)
	return n
}

// ProofHash is used to construct trie proofs, and returns the 'collapsed'
// node (for later RLP encoding) as well as the hashed node -- unless the
// node is smaller than 32 bytes, in which case it will be returned as is.
// This method does not do anything on value- or hash-nodes.
func (h *Hasher) ProofHash(original Node) (collapsed, hashed Node) {
	switch n := original.(type) {
	case *ShortNode:
		sn, _ := h.HashShortNodeChildren(n)
		return sn, h.ShortnodeToHash(sn, false)
	case *FullNode:
		fn, _ := h.HashFullNodeChildren(n)
		return fn, h.FullnodeToHash(fn, false)
	default:
		// Value and hash nodes don't have children, so they're left as were
		return n, n
	}
}
