# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added
- `-greedy` flag to enable Phase 2 nearest-amount matching (off by default;
  exact-only otherwise).
- `-maxDiff` flag: a strict cap on the amount difference for a greedy match.
  Default `0` means zero tolerance (greedy suggests nothing until set); a match
  is recorded only when the difference is `<= maxDiff`, otherwise the
  transaction stays unmatched and the bank row is left free. Prevents large
  transactions from being force-matched to far-off bank rows.
- `BankDay` type: a per-day index of bank rows sorted by amount, shared by both
  matching phases.
- `testdata/gen` generator for large synthetic datasets, plus LFS-tracked
  fixtures under `testdata/large/`.
- Expanded `testdata/` fixtures (multiple banks, IDR/comma formats, malformed
  rows) with `testdata/README.md` describing each.

### Changed
- Rewrote matching to run both phases off one day-bucketed, amount-sorted index.
  Phase 1 (exact) and Phase 2 (nearest) now use binary search instead of linear
  scans, reducing the greedy path from `O(S×B)` to `O(S·log B)` — a 3M-row set
  now reconciles in seconds instead of effectively never finishing.
- Phase 2 (discrepancy matching) is now opt-in via `-greedy`.
- Unmatched bank rows are derived from the shared `used` flags after both
  phases, removing a redundant leftover copy.

### Fixed
- Parser no longer panics on ragged rows; short rows and empty dates are handled
  gracefully.
- Eliminated double-counting of unmatched system rows across the two phases.

### Removed
- Obsolete `exactKey`/`cents` hash-key helpers (matching now compares amounts
  directly).

## [0.1.0]

### Added
- Initial reconciliation CLI: exact + greedy nearest-amount matching over a date
  range, stdout summary, and `reconciliation_<unix>.csv` export.
- README and initial `testdata/` fixtures.
