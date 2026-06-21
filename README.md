# reco-script

A small CLI that reconciles a system's internal transactions against one or more
bank statement CSVs over a date range. It reports exact matches, likely matches
with amount discrepancies, and unmatched rows on either side.

## Build

```bash
go build -o reco .
```

Requires Go 1.25+ (see `go.mod`). No external dependencies — standard library only.

## Usage

```bash
reco -system system.csv -bank bca.csv,bni.csv -start 2024-01-01 -end 2024-01-31
```

| Flag      | Description                                       |
|-----------|---------------------------------------------------|
| `-system` | Path to the internal system transactions CSV.     |
| `-bank`   | Comma-separated list of bank statement CSV paths. |
| `-start`  | Start date, inclusive (`YYYY-MM-DD`).             |
| `-end`    | End date, inclusive (`YYYY-MM-DD`).               |

All four flags are required. The tool prints a summary to stdout and writes a
`reconciliation_<unix>.csv` file in the working directory.

## Input formats

**System CSV** — columns: `trxID, amount, type, transactionTime`

```csv
trxID,amount,type,transactionTime
S1,100.00,CREDIT,2024-01-02T09:00:00Z
```

- `amount` is always positive; direction comes from `type` (`DEBIT` or `CREDIT`).
- `transactionTime` accepts RFC3339, `YYYY-MM-DD HH:MM:SS`, or `YYYY-MM-DD`.

**Bank CSV** — columns: `unique_identifier, amount, date`

```csv
unique_identifier,amount,date
B100,100.00,2024-01-02
B101,-50.00,2024-01-03
```

- `amount` is signed (negative for debits).
- The bank name is derived from the file name (`bca.csv` → `bca`).

Sample files live in `testdata/`.

## How it works

Both sides are first filtered to the `[start, end]` window (calendar day,
inclusive). System amounts are signed via the `type` column so they compare
directly against the already-signed bank amounts.

1. **Phase 1 — exact match.** Bank rows are indexed by `(day, signed amount)`.
   Amounts are compared as integer cents to avoid float equality bugs. Each
   system transaction claims at most one matching bank row → **Matched** pairs.
2. **Phase 2 — discrepancy match.** Remaining system transactions are greedily
   paired with the leftover bank row on the *same day* whose amount is closest.
   These become **Recommended** pairs (likely matches that need an adjustment),
   each carrying the absolute amount difference.
3. **Leftovers.** Anything still unpaired is reported as unmatched — system
   transactions missing from the banks, and bank rows missing from the system
   (grouped by bank).

## Output

The stdout summary reports counts of processed, matched, recommended, and
unmatched transactions plus the total discrepancy, followed by the unmatched
rows on each side.

A CSV (`reconciliation_<unix>.csv`) is also written with one row per matched and
recommended pair: `Type, TrxID/BankID, Date, Amount, Difference`.

## Project layout

| File            | Responsibility                                  |
|-----------------|-------------------------------------------------|
| `main.go`       | CLI flag parsing and orchestration.             |
| `model.go`      | `Transaction`, `BankRow`, `Pair` types.         |
| `parse.go`      | CSV reading and parsing.                         |
| `reconcile.go`  | Matching logic, summary, and CSV export.         |
