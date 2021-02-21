package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"time"

	"github.com/shopspring/decimal"
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

func main() {
	transactions := ReadStandardFile("tmp-data/TransactionsHistoryReport-2021-02-20-19_51_13.csv")
	sort.Slice(transactions, func(i, j int) bool {
		return transactions[i].Timestamp.Unix() < transactions[j].Timestamp.Unix()
	})
	account := NewAccount()

	for _, t := range transactions {
		sale := account.processTransaction(t)
		if sale != nil {
			fmt.Printf("%s Sold %s %s -- Proceeds: $%s, Cost: $%s, P&L: $%s\n", t.Timestamp, t.Quantity, t.Asset, sale.Proceeds, sale.Cost, sale.Proceeds.Sub(sale.Cost))
		}
	}
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
		Quantity: t.Quantity,
		Spot:     t.Spot,
	}
}

type Lot struct {
	Quantity decimal.Decimal
	Spot     decimal.Decimal
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

func (h *LotHistory) pop() *Lot {
	if len(h.Lots) == 0 {
		log.Fatal("LotHistory.Lots len is 0.  Cannot pop element off empty slice.")
	}
	lot := h.Lots[0]
	h.Lots = h.Lots[1:]
	return lot
}

func (h *LotHistory) peek() *Lot {
	if len(h.Lots) == 0 {
		return nil
	}

	return h.Lots[0]
}

func (h *LotHistory) sell(quantity decimal.Decimal) decimal.Decimal {
	totalCost := decimal.Zero
	remaining := quantity
	for ok := true; ok; ok = remaining.GreaterThan(decimal.Zero) {
		lot := h.peek()
		if lot == nil {
			log.Fatalf("No more lots available. Sold more shares than bought. %s shares remaining", remaining)
		}

		switch remaining.Cmp(lot.Quantity) {
		case -1:
			totalCost = totalCost.Add(remaining.Mul(lot.Spot))
			lot.Quantity = lot.Quantity.Sub(remaining)
			remaining = decimal.Zero
		default:
			lot = h.pop()
			totalCost = totalCost.Add(lot.TotalCost())
			remaining = remaining.Sub((lot.TotalCost()))
		}

	}
	return totalCost
}

type AssetHolding struct {
	LotHistory LotHistory
}

func NewAssetHolding() *AssetHolding {
	lotHistory := &LotHistory{
		Lots: make([]*Lot, 0),
	}
	return &AssetHolding{
		LotHistory: *lotHistory,
	}
}

func (h *AssetHolding) Quantity() decimal.Decimal {
	quantity := decimal.Zero
	for _, l := range h.LotHistory.Lots {
		quantity = quantity.Add(l.Quantity)
	}
	return quantity
}

func (h *AssetHolding) TotalCost() decimal.Decimal {
	totalCost := decimal.Zero
	for _, l := range h.LotHistory.Lots {
		totalCost = totalCost.Add(l.Spot)
	}
	return totalCost
}

func (h *AssetHolding) AvgCost() decimal.Decimal {
	totalCost := decimal.Zero
	quantity := decimal.Zero
	for _, l := range h.LotHistory.Lots {
		totalCost = totalCost.Add(l.Spot)
		quantity = quantity.Add(l.Quantity)
	}
	return totalCost.Div(quantity)
}

type Sale struct {
	Cost     decimal.Decimal
	Proceeds decimal.Decimal
}

type Account struct {
	Holdings map[string]*AssetHolding
}

func NewAccount() *Account {
	return &Account{
		Holdings: make(map[string]*AssetHolding),
	}
}

func (a *Account) processTransaction(t *Transaction) *Sale {
	var sale *Sale
	asset := t.Asset
	holding, ok := a.Holdings[asset]
	if !ok {
		holding = NewAssetHolding()
		a.Holdings[asset] = holding
	}

	switch t.Action {
	case BUY:
		lot := t.ToLot()
		holding.LotHistory.append(lot)

	case SELL:
		cost := holding.LotHistory.sell(t.Quantity)
		sale = &Sale{
			Cost:     cost,
			Proceeds: t.Quantity.Mul(t.Spot),
		}
	}
	return sale
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

		fmt.Println(record)
		if headerRecordFound {

			time, err := time.Parse("2006-01-02T15:04:05Z", record[0])
			if err != nil {
				log.Fatalf("Invalid time %s", record[0])
			}

			// todo: Action

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
