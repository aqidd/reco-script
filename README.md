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
reco -greedy -system system.csv -bank bca.csv,bni.csv -start 2024-01-01 -end 2024-01-31
```

| Flag      | Required | Description                                              |
|-----------|----------|---------------------------------------------------------|
| `-system` | yes      | Path to the internal system transactions CSV.           |
| `-bank`   | yes      | Comma-separated list of bank statement CSV paths.       |
| `-start`  | yes      | Start date, inclusive (`YYYY-MM-DD`).                   |
| `-end`    | yes      | End date, inclusive (`YYYY-MM-DD`).                     |
| `-greedy` | no       | Enable Phase 2 nearest-amount matching (off by default).|

The tool prints a summary to stdout and writes a `reconciliation_<unix>.csv`
file in the working directory. Without `-greedy` it runs exact-match only; any
non-exact transaction is reported as unmatched.

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
directly against the already-signed bank amounts. Both phases run off one
shared per-day index — bank rows bucketed by day, each day's rows sorted by
amount — so lookups are `O(log n)`, not a full scan.

1. **Phase 1 — exact match.** For each system transaction, binary-search its
   day's bank rows for the same signed amount and claim the first unused match
   → **Matched** pairs.
2. **Phase 2 — discrepancy match** *(only with `-greedy`)*. Remaining system
   transactions are paired with the nearest-amount unused bank row on the *same
   day*, found by binary search over the sorted rows. These become
   **Recommended** pairs (likely matches needing an adjustment), each carrying
   the absolute amount difference.
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

| File            | Responsibility                                          |
|-----------------|---------------------------------------------------------|
| `main.go`       | CLI flag parsing and orchestration.                     |
| `model.go`      | `Transaction`, `BankRow`, `Pair`, `BankDay` types.      |
| `parse.go`      | CSV reading and parsing.                                 |
| `reconcile.go`  | Matching logic, summary, and CSV export.                |

## Scale & test data

`testdata/` holds small hand-written fixtures (see `testdata/README.md`).
`testdata/gen` generates large synthetic datasets for performance testing:

```bash
go run ./testdata/gen -n 1000000 -days 3 -out testdata/large
```

This produces ~3M system + ~3M bank rows. The matching is `O((S+B)·log B)`, so
a 3M-row set reconciles in seconds. The generated CSVs are tracked with Git LFS
(see `.gitattributes`).
