package main

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"time"
	"os"
	"encoding/csv"
)

// Summary is the reconciliation result.
type Summary struct {
	TotalProcessed   int
	MatchedPairs     []Pair               // matched exact pairs
	RecommendedPairs   []Pair             // matched pairs with discrepancies, but only those that are recommended for adjustment
	UnmatchedSystem  []Transaction        // in system, missing from banks
	UnmatchedBank    map[string][]BankRow // in banks, missing from system, by bank
	TotalDiscrepancy float64              // sum of |amount differences| of matched pairs
}

// dayOnly strips the time-of-day so we compare calendar days only.
func dayOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// inRange reports whether t's calendar day is within [start, end] inclusive.
func inRange(t, start, end time.Time) bool {
	d := dayOnly(t)
	return !d.Before(dayOnly(start)) && !d.After(dayOnly(end))
}

// cents converts a money float to integer cents so equal amounts hash the same
// in a map key (float keys are fragile: 10.10 may not equal 10.10 exactly).
func cents(f float64) int64 { return int64(math.Round(f * 100)) }

// exactKey identifies an exact match: same day AND same signed amount.
func exactKey(day string, signed float64) string {
	return day + "|" + strconv.FormatInt(cents(signed), 10)
}

// Reconcile compares system transactions against bank rows within [start, end].
func Reconcile(sys []Transaction, bank []BankRow, start, end time.Time) Summary {
	// Filter both sides to the reconciliation window.
	var filteredSys []Transaction
	for _, s := range sys {
		if inRange(s.TransactionTime, start, end) {
			filteredSys = append(filteredSys, s)
		}
	}
	var filteredBank []BankRow
	for _, b := range bank {
		if inRange(b.Date, start, end) {
			filteredBank = append(filteredBank, b)
		}
	}

	sum := Summary{
		TotalProcessed: len(filteredSys) + len(filteredBank),
		UnmatchedBank:  map[string][]BankRow{},
	}

	// Phase 1 — exact match on (day, signed amount).
	bankByKey := map[string][]int{}
	for j, b := range filteredBank {
		k := exactKey(b.DateStr(), b.Amount)
		bankByKey[k] = append(bankByKey[k], j)
	}
	usedBank := make([]bool, len(filteredBank))
	var sysLeft []Transaction
	for _, s := range filteredSys {
		k := exactKey(s.Date(), s.Signed())
		matched := false
		for _, j := range bankByKey[k] {
			if !usedBank[j] {
				usedBank[j] = true
				matched = true
				sum.MatchedPairs = append(sum.MatchedPairs, Pair{Sys: s, Bank: filteredBank[j], Diff: 0})
				break
			}
		}
		if !matched {
			sysLeft = append(sysLeft, s)
		}
	}
	var bankLeft []BankRow
	for j, b := range filteredBank {
		if !usedBank[j] {
			bankLeft = append(bankLeft, b)
		}
	}

	// Phase 2 — greedy nearest-amount within the same day -> discrepancies.
	// Don't save in matched pairs. save to recomended pairs. These are the ones that are likely to be adjusted.
	pairs, sysUnmatched, bankUnmatched := greedyMatch(sysLeft, bankLeft)
	for _, p := range pairs {
		sum.RecommendedPairs = append(sum.RecommendedPairs, p)
		sum.TotalDiscrepancy += p.Diff
	}

	// Leftovers are truly unmatched.
	sum.UnmatchedSystem = sysUnmatched
	for _, b := range bankUnmatched {
		sum.UnmatchedBank[b.Bank] = append(sum.UnmatchedBank[b.Bank], b)
	}
	return sum
}

// greedyMatch pairs each leftover system transaction with the bank row on the
// SAME DAY whose signed amount is closest. One-to-one: a bank row is taken at
// most once. Each returned Pair is treated as a discrepancy.
func greedyMatch(sys []Transaction, bank []BankRow) (pairs []Pair, sysLeft []Transaction, bankLeft []BankRow) {
	used := make([]bool, len(bank))
	for _, system := range sys {
		best := -1

		for j := range bank {
			if used[j] || bank[j].DateStr() != system.Date() {
				continue
			}
			if best == -1 || math.Abs(bank[j].Amount-system.Signed()) < math.Abs(bank[best].Amount-system.Signed()) {
				best = j
			}
		}

		if best == -1 {
			sysLeft = append(sysLeft, system)
			continue
		}
		used[best] = true
		bestBankAmount := bank[best]
		pairs = append(pairs, Pair{Sys: system, Bank: bestBankAmount, Diff: math.Abs(system.Signed() - bestBankAmount.Amount)})
	}
	for j, b := range bank {
		if !used[j] {
			bankLeft = append(bankLeft, b)
		}
	}
	return
}

// Print writes a human-readable reconciliation summary to w.
func (summary Summary) Print(w io.Writer) {
	unmatched := len(summary.UnmatchedSystem)
	for _, rows := range summary.UnmatchedBank {
		unmatched += len(rows)
	}

	fmt.Fprintf(w, "Reconciliation Summary\n")
	fmt.Fprintf(w, "  Transactions processed : %d\n", summary.TotalProcessed)
	fmt.Fprintf(w, "  MatchedAmount          : %d\n", len(summary.MatchedPairs))
	fmt.Fprintf(w, "  RecommendedPairs       : %d\n", len(summary.RecommendedPairs))
	fmt.Fprintf(w, "  Unmatched              : %d\n", unmatched)
	fmt.Fprintf(w, "  Total discrepancy      : %.2f\n", summary.TotalDiscrepancy)

	if len(summary.UnmatchedSystem) > 0 {
		fmt.Fprintf(w, "\n  System transactions missing in bank statements:\n")
		for _, txn := range summary.UnmatchedSystem {
			fmt.Fprintf(w, "    %s  %-6s  %.2f  %s\n", txn.TrxID, txn.Type, txn.Amount, txn.Date())
		}
	}
	if len(summary.UnmatchedBank) > 0 {
		fmt.Fprintf(w, "\n  Bank rows missing in system (grouped by bank):\n")
		names := make([]string, 0, len(summary.UnmatchedBank))
		for n := range summary.UnmatchedBank {
			names = append(names, n)
		}
		sort.Strings(names) // stable, deterministic output
		for _, n := range names {
			fmt.Fprintf(w, "    [%s]\n", n)
			for _, b := range summary.UnmatchedBank[n] {
				fmt.Fprintf(w, "      %s  %.2f  %s\n", b.ID, b.Amount, b.DateStr())
			}
		}
	}
}

func (summary Summary) WriteToCSV() {
	path := fmt.Sprintf("reconciliation_%d.csv", time.Now().Unix())
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create CSV file: %v\n", err)
		return
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	defer writer.Flush()

	// Write header
	writer.Write([]string{"Type", "TrxID/BankID", "Date", "Amount", "Difference"})

	// Write matched pairs
	for _, pair := range summary.MatchedPairs {
		writer.Write([]string{
			"Matched",
			pair.Sys.TrxID,
			pair.Sys.Date(),
			fmt.Sprintf("%.2f", pair.Sys.Signed()),
			fmt.Sprintf("%.2f", pair.Diff),
		})
	}

	// Write recommended pairs
	for _, pair := range summary.RecommendedPairs {
		writer.Write([]string{
			"Recommended",
			pair.Sys.TrxID,
			pair.Sys.Date(),
			fmt.Sprintf("%.2f", pair.Sys.Signed()),
			fmt.Sprintf("%.2f", pair.Diff),
		})
	}

	return
}
