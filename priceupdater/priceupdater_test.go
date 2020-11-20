package priceupdater

import (
	"math/big"
	"os"
	"testing"

	ethCommon "github.com/ethereum/go-ethereum/common"
	"github.com/hermeznetwork/hermez-node/common"
	dbUtils "github.com/hermeznetwork/hermez-node/db"
	"github.com/hermeznetwork/hermez-node/db/historydb"
	"github.com/hermeznetwork/hermez-node/test"
	"github.com/stretchr/testify/assert"
)

func TestPriceUpdater(t *testing.T) {
	// Init DB
	pass := os.Getenv("POSTGRES_PASS")
	db, err := dbUtils.InitSQLDB(5432, "localhost", "hermez", pass, "hermez")
	assert.NoError(t, err)
	historyDB := historydb.NewHistoryDB(db)
	// Clean DB
	test.WipeDB(historyDB.DB())
	// Populate DB
	// Gen blocks and add them to DB
	blocks := test.GenBlocks(1, 2)
	assert.NoError(t, historyDB.AddBlocks(blocks))
	// Gen tokens and add them to DB
	tokens := []common.Token{}
	tokens = append(tokens, common.Token{
		TokenID:     1,
		EthBlockNum: blocks[0].Num,
		EthAddr:     ethCommon.BigToAddress(big.NewInt(2)),
		Name:        "DAI",
		Symbol:      "DAI",
		Decimals:    18,
	})
	assert.NoError(t, historyDB.AddTokens(tokens))
	// Init price updater
	pu := NewPriceUpdater("https://api-pub.bitfinex.com/v2/", historyDB)
	// Update token list
	assert.NoError(t, pu.UpdateTokenList())
	// Update prices
	pu.UpdatePrices()
	// Check that prices have been updated
	limit := uint(10)
	fetchedTokens, _, err := historyDB.GetTokens(nil, nil, "", nil, &limit, historydb.OrderAsc)
	assert.NoError(t, err)
	// TokenID 0 (ETH) is always on the DB
	assert.Equal(t, 2, len(fetchedTokens))
	for _, token := range fetchedTokens {
		assert.NotNil(t, token.USD)
		assert.NotNil(t, token.USDUpdate)
	}
}
