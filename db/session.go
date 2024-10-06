package db

import (
	"fmt"

	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/triedb"

	"github.com/zsystm/mpt/trie"
)

// OpenTrieForSession creates a new trie database for the session
func OpenTrieForSession(sessionID string) (*trie.Trie, error) {
	// Use sessionID as the unique namespace or directory
	db, err := rawdb.Open(rawdb.OpenOptions{
		Type:      "leveldb",
		Directory: fmt.Sprintf("triedb/%s", sessionID),
		Namespace: "chaindata",
		Cache:     0,
		Handles:   0,
		ReadOnly:  false,
		Ephemeral: false,
	})
	if err != nil {
		return nil, err
	}

	trieDB := triedb.NewDatabase(db, nil)
	mpt := trie.NewEmpty(trieDB)

	return mpt, nil
}

type InMemorySessionStorage struct {
	sessions map[string]*trie.Trie
}

func NewInMemorySessionStorage() *InMemorySessionStorage {
	return &InMemorySessionStorage{
		sessions: make(map[string]*trie.Trie),
	}
}

func (s *InMemorySessionStorage) Get(sessionID string) (*trie.Trie, error) {
	tr, ok := s.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found")
	}
	return tr, nil
}

func (s *InMemorySessionStorage) Set(sessionID string, mpt *trie.Trie) {
	s.sessions[sessionID] = mpt
}

func (s *InMemorySessionStorage) Delete(sessionID string) {
	delete(s.sessions, sessionID)
}
