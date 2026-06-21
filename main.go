package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	sysPath := flag.String("system", "", "system transactions CSV path")
	bankList := flag.String("bank", "", "comma-separated bank statement CSV paths")
	startStr := flag.String("start", "", "start date (YYYY-MM-DD)")
	endStr := flag.String("end", "", "end date (YYYY-MM-DD)")
	flag.Parse()

	if *sysPath == "" || *bankList == "" || *startStr == "" || *endStr == "" {
		fmt.Fprintln(os.Stderr, "usage: amartha -system sys.csv -bank a.csv,b.csv -start 2024-01-01 -end 2024-01-31")
		os.Exit(2)
	}

	start, err := time.Parse("2006-01-02", *startStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bad -start: %v\n", err)
		os.Exit(1)
	}
	end, err := time.Parse("2006-01-02", *endStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bad -end: %v\n", err)
		os.Exit(1)
	}

	sys, err := parseSystem(*sysPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse system: %v\n", err)
		os.Exit(1)
	}

	var bankPaths []string
	for _, bankPath := range strings.Split(*bankList, ",") {
		if bankPath = strings.TrimSpace(bankPath); bankPath != "" {
			bankPaths = append(bankPaths, bankPath)
		}
	}
	bank, err := parseBanks(bankPaths)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse banks: %v\n", err)
		os.Exit(1)
	}

	sum := Reconcile(sys, bank, start, end)
	sum.Print(os.Stdout)
	sum.WriteToCSV()
}
