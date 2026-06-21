package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// parseTime accepts the few layouts that show up in date/time columns and
// returns the parsed time. Go uses a reference date (Mon Jan 2 15:04:05 2006)
// as the format string instead of yyyy-mm-dd tokens.
func parseTime(dateText string) (time.Time, error) {
	dateText = strings.TrimSpace(dateText)
	layouts := []string{
		time.RFC3339,          // 2024-01-15T08:30:00Z
		"2006-01-02 15:04:05", // 2024-01-15 08:30:00
		"2006-01-02",          // 2024-01-15
	}
	for _, format := range layouts {
		if parsedDate, err := time.Parse(format, dateText); err == nil {
			return parsedDate, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized time %q", dateText)
}

// bankName derives a bank label from a file path: testdata/bca.csv -> bca.
func bankName(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// readCSV opens path and returns all rows WITHOUT the header row.
func readCSV(path string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	reader := csv.NewReader(f)
	// TODO: should we allow ragged rows? discuss later
	reader.FieldsPerRecord = -1 // allow ragged rows
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) <= 1 {
		return nil, nil // empty or header-only file
	}
	return rows[1:], nil // drop header row
}

// parseSystem reads the internal system transactions CSV.
// Columns: trxID, amount, type, transactionTime
func parseSystem(path string) ([]Transaction, error) {
	rows, err := readCSV(path)
	if err != nil {
		return nil, err
	}
	var out []Transaction
	for i, record := range rows {

		// TODO: Brittle detection, can improve later
		// record[0]=trxID  record[1]=amount  record[2]=type  record[3]=transactionTime
		// Handle ragged rows from system.
		if len(record) < 4 {
			// assume transaction time is empty if missing, but still parse the row.
			if len(record) == 3 {
				record = append(record, "")
			} else {
				// TODO count skipped rows.
				continue // skip rows with less than 3 columns
			}	
		}
		// (i+2 because i is 0-based AND we dropped the header.)
		amt, err := strconv.ParseFloat(strings.TrimSpace(record[1]), 64)
		if err != nil {
			return nil, fmt.Errorf("system row %d amount: %w", i+2, err)
		}

		var ts time.Time
		if record[3] == "" {
			ts = time.Time{} // zero value for empty time
		} else {
			ts, err = parseTime(record[3])
			if err != nil {
				return nil, fmt.Errorf("system row %d time %w", i+2, err)
			}
		}
		
		out = append(out, Transaction{TrxID: record[0], Amount: amt, Type: TxType(strings.ToUpper(strings.TrimSpace(record[2]))), TransactionTime: ts})
	}
	return out, nil
}

// parseBanks reads every bank CSV and returns all rows flattened, each tagged
// with its source bank. Columns: unique_identifier, amount, date
// This is your worked example — parseSystem mirrors it.
func parseBanks(paths []string) ([]BankRow, error) {
	var bankRows []BankRow
	for _, p := range paths {
		name := bankName(p)
		rows, err := readCSV(p)
		if err != nil {
			return nil, err
		}
		for i, record := range rows {
			// Handle ragged rows from banks.
			if len(record) < 3 {
				// assume transaction time is empty if missing, but still parse the row.
				if len(record) == 2 {
					record = append(record, "")
				} else {
					// TODO count skipped rows.
					continue // skip rows with less than 2 columns
				}	
			}

			amt, err := strconv.ParseFloat(strings.TrimSpace(record[1]), 64)
			if err != nil {
				return nil, fmt.Errorf("bank %s row %d amount: %w", name, i+2, err)
			}

			var date time.Time
			if record[2] == "" {
				date = time.Time{} // zero value for empty time
			} else {
				date, err = parseTime(record[2])
				if err != nil {
					return nil, fmt.Errorf("bank %s row %d date: %w", name, i+2, err)
				}
			}
			
			bankRows = append(bankRows, BankRow{
				Bank:   name,
				ID:     record[0],
				Amount: amt,
				Date:   date,
			})
		}
	}
	return bankRows, nil
}
