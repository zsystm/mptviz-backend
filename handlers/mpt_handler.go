package handlers

import (
	"github.com/google/uuid"

	"github.com/zsystm/mpt/db"
	"github.com/zsystm/mpt/graph"
	"github.com/zsystm/mpt/trie"
)

type MPTHandler struct {
	sessDB *db.InMemorySessionStorage
}

func NewMPTHandler(sessDB *db.InMemorySessionStorage) *MPTHandler {
	return &MPTHandler{
		sessDB: sessDB,
	}
}

func (h *MPTHandler) CreateSession() (string, error) {
	sessID := uuid.New().String()
	tr, err := db.OpenTrieForSession(sessID)
	if err != nil {
		return "", err
	}
	h.sessDB.Set(sessID, tr)
	return sessID, nil
}

func (h *MPTHandler) GetMPT(sessionID string) (*trie.Trie, error) {
	mpt, err := h.sessDB.Get(sessionID)
	if err != nil {
		return nil, err
	}
	return mpt, nil
}

func (h *MPTHandler) Insert(sessionID string, hashedKey, value []byte, originalKeyHex, hashedKeyHex, valueHex string) (*trie.Trie, error) {
	mpt, err := h.sessDB.Get(sessionID)
	if err != nil {
		return nil, err
	}
	if err = mpt.Update(hashedKey, value); err != nil {
		return nil, err
	}

	// Record the operation
	ops := h.sessDB.GetOperations(sessionID)
	step := len(ops) + 1
	h.sessDB.AddOperation(sessionID, &graph.OperationRecord{
		Action:      "insert",
		OriginalKey: originalKeyHex,
		HashedKey:   hashedKeyHex,
		Value:       valueHex,
		Step:        step,
	})

	return mpt, nil
}

func (h *MPTHandler) Delete(sessionID string, hashedKey []byte, originalKeyHex, hashedKeyHex string) (*trie.Trie, error) {
	mpt, err := h.sessDB.Get(sessionID)
	if err != nil {
		return nil, err
	}
	if err = mpt.Delete(hashedKey); err != nil {
		return nil, err
	}

	// Record the operation
	ops := h.sessDB.GetOperations(sessionID)
	step := len(ops) + 1
	h.sessDB.AddOperation(sessionID, &graph.OperationRecord{
		Action:      "delete",
		OriginalKey: originalKeyHex,
		HashedKey:   hashedKeyHex,
		Step:        step,
	})

	return mpt, nil
}

func (h *MPTHandler) GetOperations(sessionID string) []*graph.OperationRecord {
	return h.sessDB.GetOperations(sessionID)
}
