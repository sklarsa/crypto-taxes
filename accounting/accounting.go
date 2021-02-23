package accounting

import (
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// NegativeQuantityErr is an error for a transaction with a negative quantity
type NegativeQuantityErr struct{}

func (m *NegativeQuantityErr) Error() string {
	return "Quantity must be > 0"
}

// NegativeSpotErr is an error for a transaction with a negative spot price
type NegativeSpotErr struct{}

func (m *NegativeSpotErr) Error() string {
	return "Spot price must be > 0"
}

// Action is either a buy or sale of a crypto
type Action int

const (
	// BUY is a purchase event of crypto
	BUY Action = iota
	// SELL is a crypto sale event, including conversion into a different asset or paying for an order
	SELL Action = iota
)

// TransactionTypeToAction converts Coinbase transaction types into BUY or SELL Actions
var TransactionTypeToAction = map[string]Action{
	"Buy":               BUY,
	"Sell":              SELL,
	"Paid for an order": SELL,
	"Send":              SELL,
	"Convert":           SELL,
	"Coinbase Earn":     BUY,
}

// Transaction is a crypto transaction as reported by Coinbase
type Transaction struct {
	Timestamp time.Time
	Action    Action
	Asset     string
	Quantity  decimal.Decimal
	Spot      decimal.Decimal
	Currency  string
}

// ToLot converts a transaction to a Lot used for accounting purposes
func (t Transaction) ToLot() *Lot {
	return &Lot{
		PurchaseDate: t.Timestamp,
		Quantity:     t.Quantity,
		Spot:         t.Spot,
	}
}

// Lot is an amount of crypto purchased in a single event.  Used for
// calculating cost basis and date purchased for accounting purposes.
type Lot struct {
	PurchaseDate time.Time
	Quantity     decimal.Decimal
	Spot         decimal.Decimal
}

// TotalCost is the cost (in USD) of a lot
func (l Lot) TotalCost() decimal.Decimal {
	return l.Quantity.Mul(l.Spot)
}

// LotHistory is a queue data structure that is used to account for all lots
// of a specific crypto asset.
type LotHistory struct {
	Asset string
	Lots  []*Lot
}

// Buy adds a lot to the lot record
func (h *LotHistory) Buy(l *Lot) error {
	if l.Quantity.LessThanOrEqual(decimal.Zero) {
		return &NegativeQuantityErr{}
	}

	if l.Spot.LessThanOrEqual(decimal.Zero) {
		return &NegativeSpotErr{}
	}

	if len(h.Lots) > 0 {
		if l.PurchaseDate.Before(h.tail().PurchaseDate) {
			return fmt.Errorf("Transactions must be in chronological order.  BUY on %s is prior to most recent BUY dated %s", l.PurchaseDate, h.tail().PurchaseDate)
		}
	}

	h.Lots = append(h.Lots, l)

	return nil
}

func (h *LotHistory) pop() (*Lot, error) {
	if len(h.Lots) == 0 {
		return nil, fmt.Errorf("%s len is 0, cannot pop element off empty slice", h.Asset)
	}
	lot := h.Lots[0]
	h.Lots = h.Lots[1:]
	return lot, nil
}

func (h *LotHistory) peek() *Lot {
	if len(h.Lots) == 0 {
		return nil
	}

	return h.Lots[0]
}

func (h *LotHistory) tail() *Lot {
	if len(h.Lots) == 0 {
		return nil
	}

	return h.Lots[len(h.Lots)-1]
}

// Sell processes a transaction against this LotHistory, adding any
// resulting Sale events to the sales channel
func (h *LotHistory) Sell(quantity decimal.Decimal, spot decimal.Decimal, date time.Time, sales chan<- *Sale) error {

	if quantity.LessThanOrEqual(decimal.Zero) {
		return &NegativeQuantityErr{}
	}

	if spot.LessThanOrEqual(decimal.Zero) {
		return &NegativeSpotErr{}
	}

	var cost decimal.Decimal
	remaining := quantity
	for ok := true; ok; ok = remaining.GreaterThan(decimal.Zero) {
		lot := h.peek()
		if lot == nil {
			return fmt.Errorf("No more lots available. Sold more shares than bought. %s shares remaining", remaining)
		}
		avgCost := h.AvgCost()
		switch remaining.Cmp(lot.Quantity) {
		case -1:
			avgCost = avgCost.Mul(remaining)
			cost = remaining.Mul(lot.Spot)
			lot.Quantity = lot.Quantity.Sub(remaining)
			remaining = decimal.Zero
		default:
			lot, err := h.pop()
			if err != nil {
				return err
			}
			cost = lot.TotalCost()
			avgCost = lot.Quantity.Mul(avgCost)
			remaining = remaining.Sub(lot.TotalCost())
		}

		sale := &Sale{
			Asset:        h.Asset,
			AvgCost:      avgCost,
			FifoCost:     cost,
			Proceeds:     quantity.Mul(spot),
			Quantity:     quantity.Sub(remaining),
			SaleDate:     date,
			PurchaseDate: lot.PurchaseDate,
		}
		sales <- sale

	}
	return nil
}

// Quantity returns the total number of shares in the LotHistory
func (h *LotHistory) Quantity() decimal.Decimal {
	quantity := decimal.Zero
	for _, l := range h.Lots {
		quantity = quantity.Add(l.Quantity)
	}
	return quantity
}

// TotalCost returns the total cost (in USD) of the shares in the LotHistory
func (h *LotHistory) TotalCost() decimal.Decimal {
	totalCost := decimal.Zero
	for _, l := range h.Lots {
		totalCost = totalCost.Add(l.Spot.Mul(l.Quantity))
	}
	return totalCost
}

// AvgCost returns the average cost (in USD) of the shares in the LotHistory
func (h *LotHistory) AvgCost() decimal.Decimal {
	num := decimal.Zero
	denom := decimal.Zero
	for _, l := range h.Lots {
		num = num.Add(l.Spot.Mul(l.Quantity))
		denom = denom.Add(l.Quantity)
	}
	if denom == decimal.Zero {
		return decimal.Zero
	}
	return num.Div(denom)
}

// Sale is a taxable sale event
type Sale struct {
	Asset        string
	SaleDate     time.Time
	PurchaseDate time.Time
	Quantity     decimal.Decimal
	AvgCost      decimal.Decimal
	FifoCost     decimal.Decimal
	Proceeds     decimal.Decimal
}

// Account is a Coinbase account, containing a LotHistory per crypto asset
type Account struct {
	Holdings map[string]*LotHistory
}

// NewAccount initializes an Account struct
func NewAccount() *Account {
	return &Account{
		Holdings: make(map[string]*LotHistory),
	}
}

// ProcessTransaction replays a transaction in the account, sending any resulting
// Sales to the sales channel
func (a *Account) ProcessTransaction(t *Transaction, sales chan<- *Sale) error {

	asset := t.Asset
	holding, ok := a.Holdings[asset]
	if !ok {
		holding = &LotHistory{
			Asset: t.Asset,
			Lots:  make([]*Lot, 0),
		}
		a.Holdings[asset] = holding
	}

	switch t.Action {
	case BUY:
		lot := t.ToLot()
		err := holding.Buy(lot)
		if err != nil {
			return err
		}

	case SELL:
		err := holding.Sell(t.Quantity, t.Spot, t.Timestamp, sales)
		if err != nil {
			return err
		}
	}
	return nil
}

// Report returns a string containing an account summary
func (a *Account) Report() string {
	header := "Account Summary"
	report := strings.Repeat("-", len(header)) + "\n"
	report += header + "\n" + strings.Repeat("-", len(header)) + "\n"
	for asset, holding := range a.Holdings {
		report += fmt.Sprintf("%s: %s\n", asset, holding.Quantity())
	}
	return report
}
