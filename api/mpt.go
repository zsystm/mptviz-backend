package api

import (
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/zsystm/mpt/graph"
	"github.com/zsystm/mpt/handlers"
	"github.com/zsystm/mpt/utils"
)

type ApiMPT struct {
	h *handlers.MPTHandler
}

func NewApiMPT(h *handlers.MPTHandler) *ApiMPT {
	return &ApiMPT{h: h}
}

func (a *ApiMPT) CreateSession(c echo.Context) error {
	sessID, err := a.h.CreateSession()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"message": "Failed to create session",
		})
	}
	return c.JSON(http.StatusOK, echo.Map{
		"sessionId": sessID,
	})
}

func (a *ApiMPT) GetMPT(c echo.Context) error {
	sessID := c.QueryParam("sessionId")
	// Validate the session ID
	_, err := uuid.Parse(sessID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"message": "Invalid session ID",
		})
	}

	mpt, err := a.h.GetMPT(sessID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"message": "Failed to get session",
		})
	}

	nodeData, metadata := graph.BuildTrieGraph(mpt.Root)
	operations := a.h.GetOperations(sessID)

	return c.JSON(http.StatusOK, &graph.TrieResponse{
		Root:       nodeData,
		Operations: operations,
		Metadata:   metadata,
	})
}

func (a *ApiMPT) Insert(c echo.Context) error {
	sessID := c.QueryParam("sessionId")
	// Validate the session ID
	_, err := uuid.Parse(sessID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"message": "Invalid session ID",
		})
	}

	key := c.QueryParam("key")
	if key == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"message": "Invalid key",
		})
	}
	// Preserve original key with 0x prefix for the response
	originalKey := key
	if len(originalKey) > 2 && originalKey[:2] != "0x" {
		originalKey = "0x" + originalKey
	}

	// remove 0x prefix for decoding
	if len(key) > 2 && key[:2] == "0x" {
		key = key[2:]
	}
	// key must be hexa-decimal
	keyBytes, err := hex.DecodeString(key)
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"message": "Invalid key",
		})
	}

	// Hash the key with keccak256 to match go-ethereum's MPT behavior
	hashedKey := utils.HashKey(keyBytes)
	hashedKeyHex := hex.EncodeToString(hashedKey)

	value := c.QueryParam("value")
	if value == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"message": "Invalid value",
		})
	}
	sa := types.NewEmptyStateAccount()
	if err = json.Unmarshal([]byte(value), &sa); err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"message": "Invalid value",
		})
	}
	valueBytes, err := rlp.EncodeToBytes(&sa)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"message": "Failed to encode value",
		})
	}

	valueHex := hex.EncodeToString(valueBytes)

	mpt, err := a.h.Insert(sessID, hashedKey, valueBytes, originalKey, hashedKeyHex, valueHex)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"message": "Failed to insert",
		})
	}

	nodeData, metadata := graph.BuildTrieGraph(mpt.Root)
	operations := a.h.GetOperations(sessID)

	return c.JSON(http.StatusOK, &graph.TrieResponse{
		Root:       nodeData,
		Operations: operations,
		Metadata:   metadata,
	})
}

func (a *ApiMPT) Delete(c echo.Context) error {
	sessID := c.QueryParam("sessionId")
	// Validate the session ID
	_, err := uuid.Parse(sessID)
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"message": "Invalid session ID",
		})
	}

	key := c.QueryParam("key")
	if key == "" {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"message": "Invalid key",
		})
	}
	// Preserve original key with 0x prefix for the response
	originalKey := key
	if len(originalKey) > 2 && originalKey[:2] != "0x" {
		originalKey = "0x" + originalKey
	}

	// remove 0x prefix for decoding
	if len(key) > 2 && key[:2] == "0x" {
		key = key[2:]
	}
	// key must be hexa-decimal
	keyBytes, err := hex.DecodeString(key)
	if err != nil {
		return c.JSON(http.StatusBadRequest, echo.Map{
			"message": "Invalid key",
		})
	}

	// Hash the key with keccak256 to match go-ethereum's MPT behavior
	hashedKey := utils.HashKey(keyBytes)
	hashedKeyHex := hex.EncodeToString(hashedKey)

	mpt, err := a.h.Delete(sessID, hashedKey, originalKey, hashedKeyHex)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{
			"message": "Failed to delete",
		})
	}

	nodeData, metadata := graph.BuildTrieGraph(mpt.Root)
	operations := a.h.GetOperations(sessID)

	return c.JSON(http.StatusOK, &graph.TrieResponse{
		Root:       nodeData,
		Operations: operations,
		Metadata:   metadata,
	})
}
