# testdata fixtures

Reconciliation window for every scenario below: `-start 2024-01-01 -end 2024-01-31`.

## Flat — clean dataset (runs with the current parser)

Plain floats, ISO/space/date-only timestamps. Four banks. Exercises every
output bucket.

```bash
go run . -system testdata/system.csv \
  -bank testdata/bca.csv,testdata/bni.csv,testdata/mandiri.csv,testdata/bri.csv \
  -start 2024-01-01 -end 2024-01-31
```

Expected outcome:

Amounts are realistic IDR rupiah (whole rupiah, no cents): min ~Rp10rb,
typical Rp150rb–2jt, large transfers Rp25jt.

| System | Bank        | Result                                              |
|--------|-------------|-----------------------------------------------------|
| S1     | bca B100    | Matched (exact, +150,000)                            |
| S2     | bca B101    | Matched (exact, -50,000 debit)                       |
| S6     | bca B103    | Matched (exact, 1,500,000)                           |
| S7     | mandiri M300| Matched (exact, -25,000,000 debit — large transfer) |
| S9     | bca B104    | Matched (exact, -1,000,000 debit)                    |
| S11    | bri R400    | Matched (exact, lowercase `credit` → ToUpper)        |
| S3     | bni N200    | Recommended (2,000,000 vs 1,997,500 → fee 2,500)     |
| S8     | bni N201    | Recommended (500,000 vs 493,500 → fee 6,500)         |
| S12    | bri R401    | Recommended (-12,500,000 vs -12,450,000 → 50,000)    |
| S4     | —           | Unmatched system (no bank row that day)              |
| S5     | —           | Unmatched system (no bank row that day)              |
| S10    | —           | Out of range (2023-12-31 → filtered out)             |
| —      | bca B102    | Unmatched bank (-999,000, no system match)           |
| —      | mandiri M301| Unmatched bank (12,000,000, no system match)         |

Total discrepancy: 2,500 + 6,500 + 50,000 = 59,000.

Variations packed in: multiple banks, realistic IDR magnitudes (Rp75rb up to
Rp25jt) as plain floats, discrepancies that read like real admin/transfer fees,
three timestamp layouts (RFC3339, `space`, date-only), lowercase type,
exact / discrepancy / unmatched-both-sides / out-of-range cases.

## `idr/` — Indonesian localized format (needs parser change)

`Rp` prefix, `.` thousands separator, `,` decimal — e.g. `"Rp1.500.000,00"`.
Amounts are `"`-quoted because they contain the CSV separator comma.

To parse: strip `Rp` and spaces, remove `.`, swap `,` → `.`, then `ParseFloat`.
A negative is written `"-Rp250.000,50"`.

## `comma/` — en-US thousands separator (needs parser change)

`"1,000.00"`, `"1,234,567.89"`, `"-2,500.75"`. Quoted for the same reason.

To parse: strip `,` then `ParseFloat`. (Conflicts with the IDR rule above —
the parser needs a locale flag or a heuristic to tell them apart.)

## `malformed/` — robustness / failure-path fixtures

| File               | What it stresses                                                        |
|--------------------|------------------------------------------------------------------------|
| `system_padded.csv`| Padded fields (` 100.00 `, ` CREDIT `) + mixed-case `Credit`. Amount/type already trimmed & upper-cased — these pass today. Row S3 amount `abc` hits the `ParseFloat` error path. |
| `dates_mixed.csv`  | Row S2 uses `02/01/2024` (dd/mm/yyyy) — unsupported by `parseTime`, an error today; target for a new layout. |
| `short_row.csv`    | Missing `transactionTime` column → `record[3]` index **panic**. Parser should bounds-check the row length. |
| `header_only.csv`  | Header with no data rows → `readCSV` returns nil (handled today).        |
| `empty.csv`        | Zero-byte file → handled today (nil rows).                              |

Note: `parse.go:74` has a pre-existing format-string bug (`%2` should be `%w`)
that surfaces when the system-row time error path fires (e.g. `dates_mixed.csv`).
