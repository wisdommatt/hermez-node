package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/hermeznetwork/hermez-node/api/parsers"
	"github.com/hermeznetwork/hermez-node/common"
	"github.com/yourbasic/graph"
)

// AtomicGroup represents a set of atomic transactions
type AtomicGroup struct {
	ID  common.AtomicGroupID `json:"atomicGroupId"`
	Txs []common.PoolL2Tx    `json:"transactions"`
}

// SetAtomicGroupID set the atomic group ID for an atomic group that already has Txs
func (ag *AtomicGroup) SetAtomicGroupID() {
	ids := []common.TxID{}
	for _, tx := range ag.Txs {
		ids = append(ids, tx.TxID)
	}
	ag.ID = common.CalculateAtomicGroupID(ids)
}

// IsAtomicGroupIDValid return false if the atomic group ID that is set
// doesn't match with the calculated
func (ag AtomicGroup) IsAtomicGroupIDValid() bool {
	ids := []common.TxID{}
	for _, tx := range ag.Txs {
		ids = append(ids, tx.TxID)
	}
	actualAGID := common.CalculateAtomicGroupID(ids)
	return actualAGID == ag.ID
}

func (a *API) postAtomicPool(c *gin.Context) {
	// Parse body
	var receivedAtomicGroup AtomicGroup
	if err := c.ShouldBindJSON(&receivedAtomicGroup); err != nil {
		retBadReq(err, c)
		return
	}
	// Validate atomic group id
	if !receivedAtomicGroup.IsAtomicGroupIDValid() {
		retBadReq(errors.New(ErrInvalidAtomicGroupID), c)
		return
	}
	nTxs := len(receivedAtomicGroup.Txs)
	if nTxs <= 1 {
		retBadReq(errors.New(ErrSingleTxInAtomicEndpoint), c)
		return
	}
	// Validate txs
	txIDStrings := make([]string, nTxs) // used for successful response
	clientIP := c.ClientIP()
	for i, tx := range receivedAtomicGroup.Txs {
		// Find requested transaction
		relativePosition, err := requestOffset2RelativePosition(tx.RqOffset)
		if err != nil {
			retBadReq(err, c)
			return
		}
		requestedPosition := i + relativePosition
		if requestedPosition > len(receivedAtomicGroup.Txs)-1 || requestedPosition < 0 {
			retBadReq(errors.New(ErrRqOffsetOutOfBounds), c)
			return
		}
		// Set fields that are omitted in the JSON
		requestedTx := receivedAtomicGroup.Txs[requestedPosition]
		receivedAtomicGroup.Txs[i].RqFromIdx = requestedTx.FromIdx
		receivedAtomicGroup.Txs[i].RqToIdx = requestedTx.ToIdx
		receivedAtomicGroup.Txs[i].RqToEthAddr = requestedTx.ToEthAddr
		receivedAtomicGroup.Txs[i].RqToBJJ = requestedTx.ToBJJ
		receivedAtomicGroup.Txs[i].RqTokenID = requestedTx.TokenID
		receivedAtomicGroup.Txs[i].RqAmount = requestedTx.Amount
		receivedAtomicGroup.Txs[i].RqFee = requestedTx.Fee
		receivedAtomicGroup.Txs[i].RqNonce = requestedTx.Nonce
		receivedAtomicGroup.Txs[i].ClientIP = clientIP
		receivedAtomicGroup.Txs[i].AtomicGroupID = receivedAtomicGroup.ID

		// Validate transaction
		if err := a.verifyPoolL2Tx(receivedAtomicGroup.Txs[i]); err != nil {
			retBadReq(err, c)
			return
		}

		// Prepare response
		txIDStrings[i] = receivedAtomicGroup.Txs[i].TxID.String()
	}

	// Validate that all txs in the payload represent a single atomic group
	if !isSingleAtomicGroup(receivedAtomicGroup.Txs) {
		retBadReq(errors.New(ErrTxsNotAtomic), c)
		return
	}
	// Insert to DB
	if err := a.l2.AddAtomicTxsAPI(receivedAtomicGroup.Txs); err != nil {
		retSQLErr(err, c)
		return
	}
	// Return IDs of the added txs in the pool
	c.JSON(http.StatusOK, txIDStrings)
}

// requestOffset2RelativePosition translates from 0 to 7 to protocol position
func requestOffset2RelativePosition(rqoffset uint8) (int, error) {
	const rqOffsetZero = 0
	const rqOffsetOne = 1
	const rqOffsetTwo = 2
	const rqOffsetThree = 3
	const rqOffsetFour = 4
	const rqOffsetFive = 5
	const rqOffsetSix = 6
	const rqOffsetSeven = 7
	const rqOffsetMinusFour = -4
	const rqOffsetMinusThree = -3
	const rqOffsetMinusTwo = -2
	const rqOffsetMinusOne = -1

	switch rqoffset {
	case rqOffsetZero:
		return rqOffsetZero, errors.New(ErrTxsNotAtomic)
	case rqOffsetOne:
		return rqOffsetOne, nil
	case rqOffsetTwo:
		return rqOffsetTwo, nil
	case rqOffsetThree:
		return rqOffsetThree, nil
	case rqOffsetFour:
		return rqOffsetMinusFour, nil
	case rqOffsetFive:
		return rqOffsetMinusThree, nil
	case rqOffsetSix:
		return rqOffsetMinusTwo, nil
	case rqOffsetSeven:
		return rqOffsetMinusOne, nil
	default:
		return rqOffsetZero, errors.New(ErrInvalidRqOffset)
	}
}

// isSingleAtomicGroup returns true if all the txs are needed to be forged
// (all txs will be forged in the same batch or non of them will be forged)
func isSingleAtomicGroup(txs []common.PoolL2Tx) bool {
	// Create a graph from the given txs to represent requests between transactions
	g := graph.New(len(txs))
	// Create vertices that connect nodes of the graph (txs) using RqOffset
	for i, tx := range txs {
		requestedRelativePosition, err := requestOffset2RelativePosition(tx.RqOffset)
		if err != nil {
			return false
		}
		requestedPosition := i + requestedRelativePosition
		if requestedPosition < 0 || requestedPosition >= len(txs) {
			// Safety check: requested tx is not out of array bounds
			return false
		}
		g.Add(i, requestedPosition)
	}
	// A graph with a single strongly connected component,
	// means that all the nodes can be reached from all the nodes.
	// If tx A "can reach" tx B it means that tx A requests tx B.
	// Therefore we can say that if there is a single strongly connected component in the graph,
	// all the transactions require all trnsactions to be forged, in other words: they are an atomic group
	strongComponents := graph.StrongComponents(g)
	return len(strongComponents) == 1
}

func (a *API) getAtomicGroup(c *gin.Context) {
	// Get TxID
	atomicGroupID, err := parsers.ParseParamAtomicGroupID(c)
	if err != nil {
		retBadReq(err, c)
		return
	}
	// Fetch tx from l2DB
	txs, err := a.l2.GetPoolTxsByAtomicGroupIDAPI(atomicGroupID)
	if err != nil {
		retSQLErr(err, c)
		return
	}
	// Build successful response
	c.JSON(http.StatusOK, txs)
}
