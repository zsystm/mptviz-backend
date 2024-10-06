package trie

import "github.com/ethereum/go-ethereum/common"

// ID is the identifier for uniquely identifying a trie.
type ID struct {
	StateRoot common.Hash // The root of the corresponding state(block.root)
	Owner     common.Hash // The contract address hash which the trie belongs to
	Root      common.Hash // The root hash of trie
}

// StateTrieID constructs an identifier for state trie with the provided state root.
func StateTrieID(root common.Hash) *ID {
	return &ID{
		StateRoot: root,
		Owner:     common.Hash{},
		Root:      root,
	}
}

// StorageTrieID constructs an identifier for storage trie which belongs to a certain
// state and contract specified by the stateRoot and owner.
func StorageTrieID(stateRoot common.Hash, owner common.Hash, root common.Hash) *ID {
	return &ID{
		StateRoot: stateRoot,
		Owner:     owner,
		Root:      root,
	}
}

// TrieID constructs an identifier for a standard trie(not a second-layer trie)
// with provided root. It's mostly used in tests and some other tries like CHT trie.
func TrieID(root common.Hash) *ID {
	return &ID{
		StateRoot: root,
		Owner:     common.Hash{},
		Root:      root,
	}
}
