package trie

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/triedb/database"
)

// trieReader is a wrapper of the underlying node reader. It's not safe
// for concurrent usage.
type TrieReader struct {
	Owner  common.Hash
	Reader database.Reader
	Banned map[string]struct{} // Marker to prevent node from being accessed, for tests
}

// NewTrieReader initializes the trie reader with the given node reader.
func NewTrieReader(stateRoot, owner common.Hash, db database.Database) (*TrieReader, error) {
	if stateRoot == (common.Hash{}) || stateRoot == types.EmptyRootHash {
		if stateRoot == (common.Hash{}) {
			log.Error("Zero state root hash!")
		}
		return &TrieReader{Owner: owner}, nil
	}
	reader, err := db.Reader(stateRoot)
	if err != nil {
		return nil, &MissingNodeError{Owner: owner, NodeHash: stateRoot, Err: err}
	}
	return &TrieReader{Owner: owner, Reader: reader}, nil
}

// NewEmptyReader initializes the pure in-memory reader. All read operations
// should be forbidden and returns the MissingNodeError.
func NewEmptyReader() *TrieReader {
	return &TrieReader{}
}

// Node retrieves the rlp-encoded trie node with the provided trie node
// information. An MissingNodeError will be returned in case the node is
// not found or any error is encountered.
func (r *TrieReader) Node(path []byte, hash common.Hash) ([]byte, error) {
	// Perform the logics in tests for preventing trie node access.
	if r.Banned != nil {
		if _, ok := r.Banned[string(path)]; ok {
			return nil, &MissingNodeError{Owner: r.Owner, NodeHash: hash, Path: path}
		}
	}
	if r.Reader == nil {
		return nil, &MissingNodeError{Owner: r.Owner, NodeHash: hash, Path: path}
	}
	blob, err := r.Reader.Node(r.Owner, path, hash)
	if err != nil || len(blob) == 0 {
		return nil, &MissingNodeError{Owner: r.Owner, NodeHash: hash, Path: path, Err: err}
	}
	return blob, nil
}
