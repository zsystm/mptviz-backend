package handlers

import (
	"github.com/google/uuid"

	"github.com/zsystm/mpt/db"
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

func (h *MPTHandler) Insert(sessionID string, key, value []byte) (*trie.Trie, error) {
	mpt, err := h.sessDB.Get(sessionID)
	if err != nil {
		return nil, err
	}
	if err = mpt.Update(key, value); err != nil {
		return nil, err
	}
	return mpt, nil
}

func (h *MPTHandler) Delete(sessionID string, key []byte) (*trie.Trie, error) {
	mpt, err := h.sessDB.Get(sessionID)
	if err != nil {
		return nil, err
	}
	if err = mpt.Delete(key); err != nil {
		return nil, err
	}
	return mpt, nil
}
