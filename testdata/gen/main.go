// Command gen produces large synthetic system + bank CSVs for scale testing.
//
//	go run ./testdata/gen -n 1000000 -days 3 -out testdata/large
//
// Output is deterministic (index-driven, no RNG) so runs are reproducible.
// Matching mix per day:
//   - ~96%  exact     (bank carries the signed system amount)
//   - ~3%   discrepancy(bank amount off by a 2,500 admin fee)
//   - ~2%   unmatched system (no bank row emitted)
//   - ~2.5% bank-only rows (no system counterpart)
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func main() {
	n := flag.Int("n", 1_000_000, "system rows per day")
	days := flag.Int("days", 3, "number of consecutive days")
	outDir := flag.String("out", "testdata/large", "output directory")
	startStr := flag.String("start", "2024-01-01", "first day (YYYY-MM-DD)")
	flag.Parse()

	start, err := time.Parse("2006-01-02", *startStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bad -start: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		os.Exit(1)
	}

	sysF, err := os.Create(filepath.Join(*outDir, "system.csv"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "create system: %v\n", err)
		os.Exit(1)
	}
	defer sysF.Close()
	bankF, err := os.Create(filepath.Join(*outDir, "bank.csv"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "create bank: %v\n", err)
		os.Exit(1)
	}
	defer bankF.Close()

	sw := bufio.NewWriterSize(sysF, 1<<20)
	bw := bufio.NewWriterSize(bankF, 1<<20)
	defer sw.Flush()
	defer bw.Flush()

	sw.WriteString("trxID,amount,type,transactionTime\n")
	bw.WriteString("unique_identifier,amount,date\n")

	var sysRows, bankRows, matched, disc, unsys, bankOnly int
	for d := 0; d < *days; d++ {
		day := start.AddDate(0, 0, d).Format("2006-01-02")
		for i := 0; i < *n; i++ {
			// Whole-rupiah amount spread across 10,000 .. ~30,000,000,
			// with 300k distinct values so (day,amount) buckets stay small.
			amount := int64(10000 + (i%300000)*100)
			typ := "DEBIT"
			signed := -amount
			if i%2 == 0 {
				typ = "CREDIT"
				signed = amount
			}
			fmt.Fprintf(sw, "S%d-%d,%d.00,%s,%s\n", d, i, amount, typ, day)
			sysRows++

			switch {
			case i%50 == 0:
				unsys++ // no bank row → unmatched system
			case i%33 == 0:
				fmt.Fprintf(bw, "B%d-%d,%d.00,%s\n", d, i, signed-2500, day)
				bankRows++
				disc++ // amount off by a fee → discrepancy
			default:
				fmt.Fprintf(bw, "B%d-%d,%d.00,%s\n", d, i, signed, day)
				bankRows++
				matched++
			}

			if i%40 == 0 {
				// Bank-only row with no system counterpart.
				fmt.Fprintf(bw, "X%d-%d,%d.00,%s\n", d, i, signed+777, day)
				bankRows++
				bankOnly++
			}
		}
	}

	fmt.Printf("days=%d rows/day=%d\n", *days, *n)
	fmt.Printf("system rows : %d\n", sysRows)
	fmt.Printf("bank rows   : %d\n", bankRows)
	fmt.Printf("  exact     : %d\n", matched)
	fmt.Printf("  discrepancy: %d\n", disc)
	fmt.Printf("  unmatched sys (no bank): %d\n", unsys)
	fmt.Printf("  bank-only : %d\n", bankOnly)
}
