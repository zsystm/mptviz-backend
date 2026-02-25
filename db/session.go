package db

import (
	"fmt"

	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/triedb"

	"github.com/zsystm/mpt/graph"
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

// sessionData holds both the trie and its operation log
type sessionData struct {
	trie       *trie.Trie
	operations []*graph.OperationRecord
}

type InMemorySessionStorage struct {
	sessions map[string]*sessionData
}

func NewInMemorySessionStorage() *InMemorySessionStorage {
	return &InMemorySessionStorage{
		sessions: make(map[string]*sessionData),
	}
}

func (s *InMemorySessionStorage) Get(sessionID string) (*trie.Trie, error) {
	sd, ok := s.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found")
	}
	return sd.trie, nil
}

func (s *InMemorySessionStorage) Set(sessionID string, mpt *trie.Trie) {
	sd, ok := s.sessions[sessionID]
	if ok {
		sd.trie = mpt
	} else {
		s.sessions[sessionID] = &sessionData{
			trie:       mpt,
			operations: make([]*graph.OperationRecord, 0),
		}
	}
}

func (s *InMemorySessionStorage) Delete(sessionID string) {
	delete(s.sessions, sessionID)
}

func (s *InMemorySessionStorage) AddOperation(sessionID string, op *graph.OperationRecord) {
	sd, ok := s.sessions[sessionID]
	if !ok {
		return
	}
	sd.operations = append(sd.operations, op)
}

func (s *InMemorySessionStorage) GetOperations(sessionID string) []*graph.OperationRecord {
	sd, ok := s.sessions[sessionID]
	if !ok {
		return nil
	}
	return sd.operations
}
