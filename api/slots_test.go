package api

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/hermeznetwork/hermez-node/common"
	"github.com/hermeznetwork/hermez-node/db/historydb"
	"github.com/mitchellh/copystructure"
	"github.com/stretchr/testify/assert"
)

type testSlot struct {
	ItemID      uint64   `json:"itemId"`
	SlotNum     int64    `json:"slotNum"`
	FirstBlock  int64    `json:"firstBlock"`
	LastBlock   int64    `json:"lastBlock"`
	OpenAuction bool     `json:"openAuction"`
	WinnerBid   *testBid `json:"winnerBid"`
}

type testSlotsResponse struct {
	Slots        []testSlot `json:"slots"`
	PendingItems uint64     `json:"pendingItems"`
}

func (t testSlotsResponse) GetPending() (pendingItems, lastItemID uint64) {
	pendingItems = t.PendingItems
	lastItemID = t.Slots[len(t.Slots)-1].ItemID
	return pendingItems, lastItemID
}

func (t testSlotsResponse) Len() int {
	return len(t.Slots)
}

func (a *API) genTestSlots(nSlots int, lastBlockNum int64, bids []testBid, auctionVars common.AuctionVariables) []testSlot {
	tSlots := []testSlot{}
	bestBids := make(map[int64]testBid)
	// It's assumed that bids for each slot will be received in increasing order
	for i := range bids {
		bestBids[bids[i].SlotNum] = bids[i]
	}

	for i := int64(0); i < int64(nSlots); i++ {
		bid, ok := bestBids[i]
		firstBlock, lastBlock := a.getFirstLastBlock(int64(i))
		tSlot := testSlot{
			SlotNum:     int64(i),
			FirstBlock:  firstBlock,
			LastBlock:   lastBlock,
			OpenAuction: a.isOpenAuction(lastBlockNum, int64(i), auctionVars),
		}
		if ok {
			tSlot.WinnerBid = &bid
		}
		tSlots = append(tSlots, tSlot)
	}
	return tSlots
}

func (a *API) getEmptyTestSlot(slotNum int64) testSlot {
	firstBlock, lastBlock := a.getFirstLastBlock(slotNum)
	slot := testSlot{
		SlotNum:     slotNum,
		FirstBlock:  firstBlock,
		LastBlock:   lastBlock,
		OpenAuction: false,
		WinnerBid:   nil,
	}
	return slot
}

func TestGetSlot(t *testing.T) {
	endpoint := apiURL + "slots/"
	for _, slot := range tc.slots {
		fetchedSlot := testSlot{}
		assert.NoError(
			t, doGoodReq(
				"GET",
				endpoint+strconv.Itoa(int(slot.SlotNum)),
				nil, &fetchedSlot,
			),
		)
		assertSlot(t, slot, fetchedSlot)
	}

	// Slot with WinnerBid == nil
	slotNum := int64(15)
	fetchedSlot := testSlot{}
	assert.NoError(
		t, doGoodReq(
			"GET",
			endpoint+strconv.Itoa(int(slotNum)),
			nil, &fetchedSlot,
		),
	)
	emptySlot := api.getEmptyTestSlot(slotNum)
	assertSlot(t, emptySlot, fetchedSlot)

	// Invalid slotNum
	path := endpoint + strconv.Itoa(-2)
	err := doBadReq("GET", path, nil, 400)
	assert.NoError(t, err)
}

func TestGetSlots(t *testing.T) {
	endpoint := apiURL + "slots"
	fetchedSlots := []testSlot{}
	appendIter := func(intr interface{}) {
		for i := 0; i < len(intr.(*testSlotsResponse).Slots); i++ {
			tmp, err := copystructure.Copy(intr.(*testSlotsResponse).Slots[i])
			if err != nil {
				panic(err)
			}
			fetchedSlots = append(fetchedSlots, tmp.(testSlot))
		}
	}
	// All slots with maxSlotNum filter
	maxSlotNum := tc.slots[len(tc.slots)-1].SlotNum + 5
	limit := 1
	path := fmt.Sprintf("%s?maxSlotNum=%d&limit=%d&fromItem=", endpoint, maxSlotNum, limit)
	err := doGoodReqPaginated(path, historydb.OrderAsc, &testSlotsResponse{}, appendIter)
	assert.NoError(t, err)
	allSlots := tc.slots
	for i := tc.slots[len(tc.slots)-1].SlotNum; i < maxSlotNum; i++ {
		emptySlot := api.getEmptyTestSlot(i + 1)
		allSlots = append(allSlots, emptySlot)
	}
	assertSlots(t, allSlots, fetchedSlots)

	// All slots with maxSlotNum filter, in reverse order
	fetchedSlots = []testSlot{}
	limit = 3
	path = fmt.Sprintf("%s?maxSlotNum=%d&limit=%d&fromItem=", endpoint, maxSlotNum, limit)
	err = doGoodReqPaginated(path, historydb.OrderDesc, &testSlotsResponse{}, appendIter)
	assert.NoError(t, err)

	flippedAllSlots := []testSlot{}
	for i := len(allSlots) - 1; i >= 0; i-- {
		flippedAllSlots = append(flippedAllSlots, allSlots[i])
	}
	assertSlots(t, flippedAllSlots, fetchedSlots)

	// maxSlotNum & wonByEthereumAddress
	fetchedSlots = []testSlot{}
	limit = 1
	bidderAddr := tc.coordinators[2].Bidder
	path = fmt.Sprintf("%s?maxSlotNum=%d&wonByEthereumAddress=%s&limit=%d&fromItem=", endpoint, maxSlotNum, bidderAddr.String(), limit)
	err = doGoodReqPaginated(path, historydb.OrderAsc, &testSlotsResponse{}, appendIter)
	assert.NoError(t, err)
	bidderAddressSlots := []testSlot{}
	for i := 0; i < len(tc.slots); i++ {
		if tc.slots[i].WinnerBid != nil {
			if tc.slots[i].WinnerBid.Bidder == bidderAddr {
				bidderAddressSlots = append(bidderAddressSlots, tc.slots[i])
			}
		}
	}
	assertSlots(t, bidderAddressSlots, fetchedSlots)

	// maxSlotNum & wonByEthereumAddress, in reverse order
	fetchedSlots = []testSlot{}
	limit = 1
	path = fmt.Sprintf("%s?maxSlotNum=%d&wonByEthereumAddress=%s&limit=%d&fromItem=", endpoint, maxSlotNum, bidderAddr.String(), limit)
	err = doGoodReqPaginated(path, historydb.OrderDesc, &testSlotsResponse{}, appendIter)
	assert.NoError(t, err)
	flippedBidderAddressSlots := []testSlot{}
	for i := len(bidderAddressSlots) - 1; i >= 0; i-- {
		flippedBidderAddressSlots = append(flippedBidderAddressSlots, bidderAddressSlots[i])
	}
	assertSlots(t, flippedBidderAddressSlots, fetchedSlots)

	// finishedAuction
	fetchedSlots = []testSlot{}
	limit = 15
	path = fmt.Sprintf("%s?finishedAuction=%t&limit=%d&fromItem=", endpoint, true, limit)
	err = doGoodReqPaginated(path, historydb.OrderAsc, &testSlotsResponse{}, appendIter)
	assert.NoError(t, err)

	currentSlot := api.getCurrentSlot(tc.blocks[len(tc.blocks)-1].EthBlockNum)
	finishedAuctionSlots := []testSlot{}
	for i := 0; i < len(tc.slots); i++ {
		finishAuction := currentSlot + int64(tc.auctionVars.ClosedAuctionSlots)
		if tc.slots[i].SlotNum <= finishAuction {
			finishedAuctionSlots = append(finishedAuctionSlots, tc.slots[i])
		} else {
			break
		}
	}
	assertSlots(t, finishedAuctionSlots, fetchedSlots)

	//minSlot + maxSlot
	limit = 10
	minSlotNum := tc.slots[3].SlotNum
	maxSlotNum = tc.slots[len(tc.slots)-1].SlotNum - 1
	fetchedSlots = []testSlot{}
	path = fmt.Sprintf("%s?maxSlotNum=%d&minSlotNum=%d&limit=%d&fromItem=", endpoint, maxSlotNum, minSlotNum, limit)
	err = doGoodReqPaginated(path, historydb.OrderAsc, &testSlotsResponse{}, appendIter)
	assert.NoError(t, err)
	minMaxBatchNumSlots := []testSlot{}
	for i := 0; i < len(tc.slots); i++ {
		if tc.slots[i].SlotNum >= minSlotNum && tc.slots[i].SlotNum <= maxSlotNum {
			minMaxBatchNumSlots = append(minMaxBatchNumSlots, tc.slots[i])
		}
	}
	assertSlots(t, minMaxBatchNumSlots, fetchedSlots)

	//minSlot + maxSlot
	limit = 15
	minSlotNum = tc.slots[0].SlotNum
	maxSlotNum = tc.slots[0].SlotNum
	fetchedSlots = []testSlot{}
	path = fmt.Sprintf("%s?maxSlotNum=%d&minSlotNum=%d&limit=%d&fromItem=", endpoint, maxSlotNum, minSlotNum, limit)
	err = doGoodReqPaginated(path, historydb.OrderAsc, &testSlotsResponse{}, appendIter)
	assert.NoError(t, err)
	minMaxBatchNumSlots = []testSlot{}
	for i := 0; i < len(tc.slots); i++ {
		if tc.slots[i].SlotNum >= minSlotNum && tc.slots[i].SlotNum <= maxSlotNum {
			minMaxBatchNumSlots = append(minMaxBatchNumSlots, tc.slots[i])
		}
	}
	assertSlots(t, minMaxBatchNumSlots, fetchedSlots)

	// Only empty Slots
	limit = 2
	minSlotNum = tc.slots[len(tc.slots)-1].SlotNum + 1
	maxSlotNum = tc.slots[len(tc.slots)-1].SlotNum + 5
	fetchedSlots = []testSlot{}
	path = fmt.Sprintf("%s?maxSlotNum=%d&minSlotNum=%d&limit=%d&fromItem=", endpoint, maxSlotNum, minSlotNum, limit)
	err = doGoodReqPaginated(path, historydb.OrderAsc, &testSlotsResponse{}, appendIter)
	assert.NoError(t, err)
	emptySlots := []testSlot{}
	for i := 0; i < len(allSlots); i++ {
		if allSlots[i].SlotNum >= minSlotNum && allSlots[i].SlotNum <= maxSlotNum {
			emptySlots = append(emptySlots, allSlots[i])
		}
	}
	assertSlots(t, emptySlots, fetchedSlots)

	// Only empty Slots, in reverse order
	limit = 4
	minSlotNum = tc.slots[len(tc.slots)-1].SlotNum + 1
	maxSlotNum = tc.slots[len(tc.slots)-1].SlotNum + 5
	fetchedSlots = []testSlot{}
	path = fmt.Sprintf("%s?maxSlotNum=%d&minSlotNum=%d&limit=%d&fromItem=", endpoint, maxSlotNum, minSlotNum, limit)
	err = doGoodReqPaginated(path, historydb.OrderDesc, &testSlotsResponse{}, appendIter)
	assert.NoError(t, err)
	flippedEmptySlots := []testSlot{}
	for i := 0; i < len(flippedAllSlots); i++ {
		if flippedAllSlots[i].SlotNum >= minSlotNum && flippedAllSlots[i].SlotNum <= maxSlotNum {
			flippedEmptySlots = append(flippedEmptySlots, flippedAllSlots[i])
		}
	}
	assertSlots(t, flippedEmptySlots, fetchedSlots)

	// 400
	// No filters
	path = fmt.Sprintf("%s?limit=%d&fromItem=", endpoint, limit)
	err = doBadReq("GET", path, nil, 400)
	assert.NoError(t, err)
	// Invalid maxSlotNum
	path = fmt.Sprintf("%s?maxSlotNum=%d", endpoint, -2)
	err = doBadReq("GET", path, nil, 400)
	assert.NoError(t, err)
	// Invalid wonByEthereumAddress
	path = fmt.Sprintf("%s?maxSlotNum=%d&wonByEthereumAddress=%s", endpoint, maxSlotNum, "0xG0000001")
	err = doBadReq("GET", path, nil, 400)
	assert.NoError(t, err)
	// Invalid minSlotNum / maxSlotNum (minSlotNum > maxSlotNum)
	maxSlotNum = tc.slots[1].SlotNum
	minSlotNum = tc.slots[4].SlotNum
	path = fmt.Sprintf("%s?maxSlotNum=%d&minSlotNum=%d&limit=%d&fromItem=", endpoint, maxSlotNum, minSlotNum, limit)
	err = doBadReq("GET", path, nil, 400)
	assert.NoError(t, err)
	// 404
	maxSlotNum = tc.slots[1].SlotNum
	path = fmt.Sprintf("%s?maxSlotNum=%d&wonByEthereumAddress=%s&limit=%d&fromItem=", endpoint, maxSlotNum, tc.coordinators[3].Bidder.String(), limit)
	err = doBadReq("GET", path, nil, 404)
	assert.NoError(t, err)
}

func assertSlots(t *testing.T, expected, actual []testSlot) {
	assert.Equal(t, len(expected), len(actual))
	for i := 0; i < len(expected); i++ {
		assertSlot(t, expected[i], actual[i])
	}
}

func assertSlot(t *testing.T, expected, actual testSlot) {
	if actual.WinnerBid != nil {
		assert.Equal(t, expected.WinnerBid.Timestamp.Unix(), actual.WinnerBid.Timestamp.Unix())
		expected.WinnerBid.Timestamp = actual.WinnerBid.Timestamp
		actual.WinnerBid.ItemID = expected.WinnerBid.ItemID
	}
	actual.ItemID = expected.ItemID
	assert.Equal(t, expected, actual)
}