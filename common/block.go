package common

import (
	"time"

	ethCommon "github.com/ethereum/go-ethereum/common"
)

// Block represents of an Ethereum block
type Block struct {
	EthBlockNum uint64         `meddler:"eth_block_num"`
	Timestamp   time.Time      `meddler:"timestamp"`
	Hash        ethCommon.Hash `meddler:"hash"`
}
