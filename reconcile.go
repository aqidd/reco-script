package main

import (
	"cmp"
	"fmt"
	"io"
	"math"
	"sort"
	"slices"
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

// Reconcile compares system transactions against bank rows within [start, end].
func Reconcile(sys []Transaction, bank []BankRow, start, end time.Time, greedy bool, maxDiff int) Summary {
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

	// Build a per-day index of bank rows, each day's rows sorted by amount.
	// Both phases run off this single structure: Phase 1 binary-searches it
	// for exact matches, Phase 2 reuses it (and the same used array) to find
	// the nearest unused row. Unmatched bank = every row still !used at the end.
	bankByDay := map[string]*BankDay{} // keyed by DateStr()
	// Build the bankByDay index of bank rows by day, and sort each day's rows by amount.
	for _, bankRow := range filteredBank {
		day := bankRow.DateStr()
		if _, ok := bankByDay[day]; !ok {
			bankByDay[day] = &BankDay{}
		}
		bankByDay[day].rows = append(bankByDay[day].rows, bankRow) // append to the day's rows
	}

	for day := range bankByDay {
		bankByDay[day].used = make([]bool, len(bankByDay[day].rows)) // initialize used slice
		slices.SortFunc(bankByDay[day].rows, func(a, b BankRow) int { return cmp.Compare(a.Amount, b.Amount) }) // sort the day's rows by amount
	}

	var sysLeft []Transaction // system rows left after Phase 1
	for _, systemRow := range filteredSys {
		day := systemRow.Date()
		bankDay, ok := bankByDay[day]
		if !ok {
			sysLeft = append(sysLeft, systemRow)
			continue
		}
		// Binary search for the signed amount in this day's bank rows.
		i, found := slices.BinarySearchFunc(bankDay.rows, systemRow.Signed(), func(b BankRow, amt float64) int {
			return cmp.Compare(b.Amount, amt)
		})
		
		var unusedRowFound bool;
		if found {
			// Find unused rows.
			for j := i; j < len(bankDay.rows) && bankDay.rows[j].Amount == systemRow.Signed(); j++ {
				if !bankDay.used[j] {
					bankDay.used[j] = true
					sum.MatchedPairs = append(sum.MatchedPairs, Pair{Sys: systemRow, Bank: bankDay.rows[j], Diff: 0})
					unusedRowFound = true
					break
				}
			}
		}

		if !found || !unusedRowFound {
			sysLeft = append(sysLeft, systemRow)
		}
	}

	// Phase 2 — greedy nearest-amount within the same day -> discrepancies.
	if greedy {
		for _, sysRow := range sysLeft {
			day := sysRow.Date()
			bankDay, ok := bankByDay[day]
			if !ok {
				sum.UnmatchedSystem = append(sum.UnmatchedSystem, sysRow)
				continue
			}

			// Find the nearest unused bank row by amount.
			part, _ := slices.BinarySearchFunc(bankDay.rows, sysRow.Signed(), func(b BankRow, amt float64) int {
				return cmp.Compare(b.Amount, amt)
			})

			//find left part unused
			left := part - 1
			for left >= 0 {
				if !bankDay.used[left] {
					break
				}
				left--
			}

			//find right part unused
			right := part
			for right < len(bankDay.rows) {
				if !bankDay.used[right] {
					break
				}
				right++
			}

			// compare left and right to see the closest value
			var closestIndex int
			if left >= 0 && right < len(bankDay.rows) {
				if math.Abs(bankDay.rows[left].Amount-sysRow.Signed()) <= math.Abs(bankDay.rows[right].Amount-sysRow.Signed()) {
					closestIndex = left
				} else {
					closestIndex = right
				}
			} else if left >= 0 {
				closestIndex = left
			} else if right < len(bankDay.rows) {
				closestIndex = right
			} else {
				sum.UnmatchedSystem = append(sum.UnmatchedSystem, sysRow)
				continue // no unused bank rows available
			}
			// Check if the difference is within the maxDiff threshold
			if math.Abs(bankDay.rows[closestIndex].Amount-sysRow.Signed()) > float64(maxDiff) {
				sum.UnmatchedSystem = append(sum.UnmatchedSystem, sysRow)
				continue // difference exceeds maxDiff
			}
			
			sum.RecommendedPairs = append(sum.RecommendedPairs, Pair{
				Sys:  sysRow,
				Bank: bankDay.rows[closestIndex],
				Diff: math.Abs(bankDay.rows[closestIndex].Amount - sysRow.Signed()),
			})
			sum.TotalDiscrepancy += math.Abs(bankDay.rows[closestIndex].Amount - sysRow.Signed())
			bankDay.used[closestIndex] = true
		}
	} else {
		// If not greedy, all remaining system rows are unmatched.
		sum.UnmatchedSystem = append(sum.UnmatchedSystem, sysLeft...)
	}

	// Collect unmatched bank rows.
	for _, bankDay := range bankByDay {
		for j, b := range bankDay.rows {
			if !bankDay.used[j] {
				sum.UnmatchedBank[b.Bank] = append(sum.UnmatchedBank[b.Bank], b)
			}
		}
	}

	return sum
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
