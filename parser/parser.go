package parser

import (
	"bufio"
	"encoding/csv"
	"io"
	"os"
	"time"

	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
	a "github.com/sklarsa/crypto-taxes/accounting"
)

// ReadStandardFile reads a transaction history csv file exported from Coinbase for a standard account,
// returning a slice of Transactions to be processed by an Account struct
func ReadStandardFile(filename string) []*a.Transaction {
	transactions := make([]*a.Transaction, 0)

	file, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	// Skip the first 7 lines before parsing the csv data
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

			transaction := &a.Transaction{
				Timestamp: time,
				Action:    a.TransactionTypeToAction[record[1]],
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
