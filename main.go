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

type Action int

const (
	BUY  Action = iota
	SELL Action = iota
)

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
			err := account.processTransaction(t, sales)
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
		fmt.Printf("%s: Sold %s of %s with P&L of $%s, Purchase date %s\n", s.SaleDate.Format("2006-01-02"), s.Quantity, s.Asset, s.Proceeds.Sub(cost), s.PurchaseDate.Format("2006-01-02"))
	}

	fmt.Println("\n" + account.Report())

}

type Transaction struct {
	Timestamp time.Time
	Action    Action
	Asset     string
	Quantity  decimal.Decimal
	Spot      decimal.Decimal
	Currency  string
}

func (t Transaction) ToLot() *Lot {
	return &Lot{
		PurchaseDate: t.Timestamp,
		Quantity:     t.Quantity,
		Spot:         t.Spot,
	}
}

type Lot struct {
	PurchaseDate time.Time
	Quantity     decimal.Decimal
	Spot         decimal.Decimal
}

func (l Lot) TotalCost() decimal.Decimal {
	return l.Quantity.Mul(l.Spot)
}

type LotHistory struct {
	Lots []*Lot
}

func (h *LotHistory) append(l *Lot) {
	h.Lots = append(h.Lots, l)
}

func (h *LotHistory) pop() (*Lot, error) {
	if len(h.Lots) == 0 {
		return nil, fmt.Errorf("%s len is 0, cannot pop element off empty slice", h)
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

func (h *LotHistory) sell(t *Transaction, sales chan<- *Sale) error {
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

func NewLotHistory() *LotHistory {
	return &LotHistory{
		Lots: make([]*Lot, 0),
	}
}

func (h *LotHistory) Quantity() decimal.Decimal {
	quantity := decimal.Zero
	for _, l := range h.Lots {
		quantity = quantity.Add(l.Quantity)
	}
	return quantity
}

func (h *LotHistory) TotalCost() decimal.Decimal {
	totalCost := decimal.Zero
	for _, l := range h.Lots {
		totalCost = totalCost.Add(l.Spot)
	}
	return totalCost
}

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

type Sale struct {
	Asset        string
	SaleDate     time.Time
	PurchaseDate time.Time
	Quantity     decimal.Decimal
	AvgCost      decimal.Decimal
	FifoCost     decimal.Decimal
	Proceeds     decimal.Decimal
}

type Account struct {
	Holdings map[string]*LotHistory
}

func NewAccount() *Account {
	return &Account{
		Holdings: make(map[string]*LotHistory),
	}
}

func (a *Account) processTransaction(t *Transaction, sales chan<- *Sale) error {

	asset := t.Asset
	holding, ok := a.Holdings[asset]
	if !ok {
		holding = NewLotHistory()
		a.Holdings[asset] = holding
	}

	switch t.Action {
	case BUY:
		lot := t.ToLot()
		holding.append(lot)

	case SELL:
		err := holding.sell(t, sales)
		if err != nil {
			return err
		}
	}
	return nil
}

type TransactionHistory struct {
	Sales []Sale
}

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

func (a *Account) Report() string {
	header := "Account Summary"
	report := strings.Repeat("-", len(header)) + "\n"
	report += header + "\n" + strings.Repeat("-", len(header)) + "\n"
	for asset, holding := range a.Holdings {
		report += fmt.Sprintf("%s: %s\n", asset, holding.Quantity())
	}
	return report
}
