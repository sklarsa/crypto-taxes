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
	h := &LotHistory{
		Asset: "BTC",
		Lots:  make([]*Lot, 0),
	}

	assert.Empty(t, h.Lots)

	// BUY 100 BTC @ $1
	h.Buy(&Lot{
		PurchaseDate: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		Quantity:     decimal.NewFromInt(100),
		Spot:         decimal.NewFromInt(1),
	})

	assert.Equal(t, 1, len(h.Lots))
	assert.Equal(t, decimal.NewFromInt(100), h.TotalCost())
	assert.Equal(t, decimal.NewFromInt(1), h.AvgCost().Round(0))

	// BUY 100 BTC @ $2
	h.Buy(&Lot{
		PurchaseDate: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
		Quantity:     decimal.NewFromInt(100),
		Spot:         decimal.NewFromInt(2),
	})

	assert.Equal(t, 2, len(h.Lots))
	assert.Equal(t, decimal.NewFromInt(300), h.TotalCost())
	assert.Equal(t, decimal.NewFromFloat(1.5), h.AvgCost().Round(1))

	// SELL 200 BTC @ $3
	sales := make(chan *Sale)
	taxableSales := make([]*Sale, 0)

	go func() {
		h.Sell(decimal.NewFromInt(200), decimal.NewFromInt(3), time.Date(2021, 2, 1, 0, 0, 0, 0, time.UTC), sales)
		defer close(sales)
	}()

	for s := range sales {
		taxableSales = append(taxableSales, s)
	}

	assert.Equal(t, 2, len(taxableSales))
	firstSale := taxableSales[0]
	assert.Equal(t, decimal.NewFromInt(100), firstSale.Quantity)
	assert.Equal(t, decimal.NewFromInt(1*100), firstSale.FifoCost)
	assert.Equal(t, decimal.NewFromFloat(150).Round(0), firstSale.AvgCost.Round(0))

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
