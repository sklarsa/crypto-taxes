package main

import (
	"flag"
	"fmt"
	"os"
	"sort"

	log "github.com/sirupsen/logrus"
	"github.com/sklarsa/crypto-taxes/accounting"
	"github.com/sklarsa/crypto-taxes/parser"
)

func usage() {
	fmt.Printf("Usage: %s [OPTIONS] filename.csv\n", os.Args[0])
	flag.PrintDefaults()
}

func main() {
	badTransactions := make(chan *accounting.Transaction)
	sales := make(chan *accounting.Sale)

	flag.Usage = usage

	var verbose bool
	flag.BoolVar(&verbose, "v", false, "Turns on debug logging")

	var avgCost bool
	flag.BoolVar(&avgCost, "avg", false, "Average cost basis (FIFO is default)")

	var csvOutput bool
	flag.BoolVar(&csvOutput, "csv", false, "Output results in turbotax csv format")

	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}

	if verbose {
		log.SetLevel(log.DebugLevel)
	}
	filename := flag.Arg(0)

	transactions, err := parser.ReadStandardFile(filename)
	if err != nil {
		log.Panic(err)
	}

	sort.Slice(transactions, func(i, j int) bool {
		return transactions[i].Timestamp.Unix() < transactions[j].Timestamp.Unix()
	})
	account := accounting.NewAccount()

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
			os.Stderr.WriteString(
				fmt.Sprintf("\033[0;31mError processing %s sale of %s %s\033[0m\n", t.Timestamp.Format("2006-01-02"), t.Quantity, t.Asset),
			)
		}
	}()

	if csvOutput {
		fmt.Println("\"Currency Name\",\"Purchase Date\",\"Cost Basis\",\"Date Sold\",\"Proceeds\"")
	}
	for s := range sales {
		cost := s.FifoCost
		if avgCost {
			cost = s.AvgCost
		}
		if csvOutput {
			fmt.Printf("\"%s\",%s,%s,%s,%s,\n", s.Asset, s.PurchaseDate.Format("2006-01-02"), cost, s.SaleDate.Format("2006-01-02"), s.Proceeds)
		} else {
			fmt.Printf("%s: Sold %s of %s with P&L of $%s purchased on %s\n", s.SaleDate.Format("2006-01-02"), s.Quantity, s.Asset, s.Proceeds.Sub(cost).Round(2), s.PurchaseDate.Format("2006-01-02"))
		}

	}
	if !csvOutput {
		fmt.Println("\n" + account.Report())
	}

}
