# D-0026 — The HotAdd qualification catalogue is binding

- Date: 2026-10-05
- Contract: §128 (new); §73–§78, §103–§105 mapped to catalogue rows
- Status: accepted (2026-10-05).

## Context

Until now the list of tests that close HotAdd lived only in
`docs/HOTADD-QUALIFICATION.md`. The negative cases (A2–A16) and the
end-to-end cases (B2–B8) were added there without a contract section naming
them, and a review of the contract on 2026-10-04 found sections whose tests
were not in the catalogue at all: storage failures (§73), parts of the
stress tests (§74), most fuzz targets (§75), random reads and resource
measurements (§78), the recovery utility (§104) and the diagnostic bundle
(§105).

## Decision

1. `docs/HOTADD-QUALIFICATION.md` is the binding list of tests that close
   the HotAdd transport (§128). Every row names the contract sections it
   covers.
2. The catalogue gains rows for every HotAdd-relevant requirement of §73–
   §78 and §103 that it lacked; requirements that cannot apply to a
   read-only HotAdd library (for example a short write) are listed as not
   applicable, with the reason.
3. The recovery utility (§104) and the diagnostic bundle (§105) are built
   before HotAdd is closed, as one administrative program that uses the
   library's own recovery code.
4. Rules from now on:
   - a new kind of test enters the catalogue in the same commit as its
     first record;
   - a test that changes behaviour gets a decision record and a new
     contract version;
   - a new kind of test that no contract section covers is proposed before
     it is run.
5. HotAdd is closed when every row is done, or deferred with an accepted
   reason (for example a test that needs equipment the labs do not have).

## Consequences

- The catalogue, not a status page, answers "what is left".
