package accounting

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

// TestBasicLotHistoryUsage performs basic functionality tests
// for the LotHistory struct.  Similar to a smoketest.
func TestBasicLotHistoryUsage(t *testing.T) {
	t0 := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	asset := "BTC"

	h := &LotHistory{
		Asset: "BTC",
		Lots:  make([]*Lot, 0),
	}

	assert.Empty(t, h.Lots)

	transactions := []Transaction{
		// BUY 100 BTC @ $1
		{
			Timestamp: t0,
			Action:    BUY,
			Asset:     asset,
			Quantity:  decimal.NewFromInt(100),
			Spot:      decimal.NewFromInt(1),
		},
		// BUY 100 BTC @ $2
		{
			Timestamp: t0.AddDate(0, 0, 1),
			Action:    BUY,
			Asset:     asset,
			Quantity:  decimal.NewFromInt(100),
			Spot:      decimal.NewFromInt(2),
		},
		// SELL 200 BTC @ 3
		{
			Timestamp: t0.AddDate(0, 0, 2),
			Action:    SELL,
			Asset:     asset,
			Quantity:  decimal.NewFromInt(200),
			Spot:      decimal.NewFromInt(3),
		},
		// BUY 100 BTC @ $10
		{
			Timestamp: t0.AddDate(0, 0, 3),
			Action:    BUY,
			Asset:     asset,
			Quantity:  decimal.NewFromInt(100),
			Spot:      decimal.NewFromInt(10),
		},
		// BUY 100 BTC @ $5
		{
			Timestamp: t0.AddDate(0, 0, 4),
			Action:    BUY,
			Asset:     asset,
			Quantity:  decimal.NewFromInt(100),
			Spot:      decimal.NewFromInt(5),
		},
		// SELL 5 BTC @ 2
		{
			Timestamp: t0.AddDate(0, 0, 5),
			Action:    SELL,
			Asset:     asset,
			Quantity:  decimal.NewFromInt(5),
			Spot:      decimal.NewFromInt(2),
		},
		// SELL 94 BTC @ 100
		{
			Timestamp: t0.AddDate(0, 0, 6),
			Action:    SELL,
			Asset:     asset,
			Quantity:  decimal.NewFromInt(94),
			Spot:      decimal.NewFromInt(100),
		},
		// SELL 2 BTC @ 5
		{
			Timestamp: t0.AddDate(0, 0, 7),
			Action:    SELL,
			Asset:     asset,
			Quantity:  decimal.NewFromInt(2),
			Spot:      decimal.NewFromInt(5),
		},
	}

	sales := make(chan *Sale)
	go func() {
		defer close(sales)
		for _, t := range transactions {
			switch t.Action {
			case BUY:
				h.Buy(t.ToLot())
			case SELL:
				h.Sell(t.Quantity, t.Spot, t.Timestamp, sales)
			}
		}
	}()

	ctr := 0
	for s := range sales {
		switch ctr {
		case 0:
			// Sale #1 (200@3) Lot #1 100@1
			assert.Equal(t, decimal.NewFromInt(100*1), s.FifoCost)
			assert.Equal(t, decimal.NewFromInt(100*3), s.Proceeds)
		case 1:
			// Sale #1 (200@3) Lot #2 100@2
			assert.Equal(t, decimal.NewFromInt(100*2), s.FifoCost)
			assert.Equal(t, decimal.NewFromInt(100*3), s.Proceeds)
		case 2:
			// Sale #2 (5@2) Lot #1 100@10
			assert.Equal(t, decimal.NewFromInt(5*10), s.FifoCost)
			assert.Equal(t, decimal.NewFromInt(5*2), s.Proceeds)
		case 3:
			// Sale #3 (94@100) Lot #1 95@10
			assert.Equal(t, decimal.NewFromInt(94*10), s.FifoCost)
			assert.Equal(t, decimal.NewFromInt(94*100), s.Proceeds)
		case 4:
			// Sale #4 (2@5) Lot #1 1@10
			assert.Equal(t, decimal.NewFromInt(1*10), s.FifoCost)
			assert.Equal(t, decimal.NewFromInt(1*5), s.Proceeds)
		case 5:
			// Sale #4 (2@5) Lot #2 1@5
			assert.Equal(t, decimal.NewFromInt(1*5), s.FifoCost)
			assert.Equal(t, decimal.NewFromInt(1*5), s.Proceeds)

		default:
			t.Errorf("Too many sales!")
		}
		ctr++
	}

	assert.Equal(t, 1, len(h.Lots))
	assert.Equal(t, decimal.NewFromInt(99), h.Quantity())
}

func TestLotHistoryEdgeCases(t *testing.T) {
	h := &LotHistory{
		Asset: "BTC",
		Lots:  make([]*Lot, 0),
	}

	quantity := decimal.NewFromInt(100)
	price := decimal.NewFromInt(5)
	t0 := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := time.Date(2020, 1, 1, 1, 1, 1, 1, time.UTC)

	sales := make(chan *Sale)

	// Cannot sell with no lots
	err := h.Sell(quantity, price, t0, nil)
	assert.Error(t, err)

	// Cannot sell more shares than bought
	h.Buy(&Lot{
		PurchaseDate: t0,
		Quantity:     quantity,
		Spot:         price,
	})

	go func() {
		err = h.Sell(quantity.Add(decimal.NewFromInt(1000)), price, t1, sales)
		assert.Error(t, err)
	}()
	sale := <-sales
	assert.Equal(t, t0, sale.PurchaseDate)

	// Buys must be in chronological order
	err = h.Buy(&Lot{
		PurchaseDate: t1,
		Quantity:     quantity,
		Spot:         price,
	})
	assert.Nil(t, err)

	err = h.Buy(&Lot{
		PurchaseDate: t0,
		Quantity:     quantity,
		Spot:         price,
	})
	assert.Error(t, err)
}
