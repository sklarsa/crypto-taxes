package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
)

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

func usage() {
	fmt.Printf("Usage: %s [OPTIONS] filename.csv\n", os.Args[0])
	flag.PrintDefaults()
}

func main() {
	badTransactions := make(chan *Transaction)
	sales := make(chan *Sale)

	flag.Usage = usage

	var verbose bool
	flag.BoolVar(&verbose, "v", false, "Turns on debug logging")

	var avgCost bool
	flag.BoolVar(&avgCost, "avg", false, "Average cost basis (FIFO is default)")

	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}

	if verbose {
		log.SetLevel(log.DebugLevel)
	}
	filename := flag.Arg(0)

	transactions := ReadStandardFile(filename)
	sort.Slice(transactions, func(i, j int) bool {
		return transactions[i].Timestamp.Unix() < transactions[j].Timestamp.Unix()
	})
	account := NewAccount()

	go func() {
		defer close(sales)
		defer close(badTransactions)

		for _, t := range transactions {
			err := account.ProcessTransaction(t, sales)
			if err != nil {
				badTransactions <- t
				continue
			}
		}
	}()

	go func() {
		for t := range badTransactions {
			fmt.Printf("Error processing %s sale of %s %s\n", t.Timestamp.Format("2006-01-02"), t.Quantity, t.Asset)
		}
	}()

	for s := range sales {
		cost := s.FifoCost
		if avgCost {
			cost = s.AvgCost
		}
		fmt.Printf("%s: Sold %s of %s with P&L of $%s purchased on %s\n", s.SaleDate.Format("2006-01-02"), s.Quantity, s.Asset, s.Proceeds.Sub(cost), s.PurchaseDate.Format("2006-01-02"))
	}

	fmt.Println("\n" + account.Report())

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
func (h *LotHistory) Buy(l *Lot) {
	h.Lots = append(h.Lots, l)
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

// Sell processes a transaction against this LotHistory, adding any
// resulting Sale events to the sales channel
func (h *LotHistory) Sell(t *Transaction, sales chan<- *Sale) error {
	var cost decimal.Decimal
	remaining := t.Quantity
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
			Asset:        t.Asset,
			AvgCost:      avgCost,
			FifoCost:     cost,
			Proceeds:     t.Quantity.Mul(t.Spot),
			Quantity:     t.Quantity,
			SaleDate:     t.Timestamp,
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
		totalCost = totalCost.Add(l.Spot)
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
		holding.Buy(lot)

	case SELL:
		err := holding.Sell(t, sales)
		if err != nil {
			return err
		}
	}
	return nil
}

// ReadStandardFile reads a transaction history csv file exported from Coinbase for a standard account,
// returning a slice of Transactions to be processed by an Account struct
func ReadStandardFile(filename string) []*Transaction {
	transactions := make([]*Transaction, 0)

	file, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	skipper := bufio.NewReader(file)
	newlineCt := 0
	for ok := true; ok; ok = newlineCt < 7 {
		rune, _, err := skipper.ReadRune()
		if err != nil {
			log.Fatal(err)
		}

		if rune == '\n' {
			newlineCt++
		}
	}

	r := csv.NewReader(skipper)
	headerRecordFound := false
	for {
		record, err := r.Read()

		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}

		log.Debug(record)
		if headerRecordFound {

			time, err := time.Parse("2006-01-02T15:04:05Z", record[0])
			if err != nil {
				log.Panicf("Invalid time %s", record[0])
			}

			transaction := &Transaction{
				Timestamp: time,
				Action:    TransactionTypeToAction[record[1]],
				Asset:     record[2],
				Quantity:  decimal.RequireFromString(record[3]),
				Spot:      decimal.RequireFromString(record[4]),
				Currency:  "USD",
			}

			transactions = append(transactions, transaction)

		}

		headerRecordFound = true
	}

	return transactions
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
