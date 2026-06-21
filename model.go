package main

import "time"

// TxType is the system transaction type enum.
type TxType string

const (
	Debit  TxType = "DEBIT"
	Credit TxType = "CREDIT"
)

// Transaction is one row from the internal system CSV.
type Transaction struct {
	TrxID           string
	Amount          float64 // always positive; direction comes from Type
	Type            TxType
	TransactionTime time.Time
}

// Signed returns the amount with direction applied: debit negative, credit
// positive. This lets us compare against bank rows, whose amount is already
// signed.
func (t Transaction) Signed() float64 {
	if t.Type == Debit {
		return -t.Amount
	}
	return t.Amount
}

// Date returns the calendar day (no time) as YYYY-MM-DD.
func (t Transaction) Date() string { return t.TransactionTime.Format("2006-01-02") }

// BankRow is one row from a bank statement CSV. Amount is signed (negative for
// debits). Bank holds the source bank name, derived from the file name.
type BankRow struct {
	Bank   string
	ID     string
	Amount float64
	Date   time.Time
}

// DateStr returns the calendar day as YYYY-MM-DD.
func (b BankRow) DateStr() string { return b.Date.Format("2006-01-02") }

// Pair is a matched system+bank transaction. Diff is the absolute amount
// difference (0 for an exact match, >0 for a discrepancy).
type Pair struct {
	Sys  Transaction
	Bank BankRow
	Diff float64
}
