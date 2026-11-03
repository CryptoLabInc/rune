---
description: Capture a decision, insight, or context into RUNE organizational memory
argument-hint: <what to remember>
allowed-tools: mcp__plugin_rune_rune__capture
---

# /rune:capture — Save to Organizational Memory

Compose the memory yourself from the current work and call
`mcp__plugin_rune_rune__capture`. The runtime timestamps the record, embeds the
insight, encrypts its stored vector, and seals the memory payload. Any `author`
field is client-supplied metadata, not verified human attribution.

Pass two fields:

- `insight` — the concise, self-contained essence someone should find when
  searching later: what was decided or learned, and why, in a few sentences.
  This is what gets embedded and searched, so write it to stand alone (no "as
  discussed above").
- `context` — the fuller background: the problem, alternatives, trade-offs, and
  specifics. Stored and returned on recall but not searched. Optional, but
  usually worth including.

Use `$ARGUMENTS` as the subject when given; otherwise capture the most
significant decision or insight from the current conversation.

Capture may not check for duplicates. Submit each decision once; separate
capture calls can store duplicate records. Use successful capture results in
the current conversation to keep track of what has already been saved.

Do not call recall with the insight to check whether it was already captured:
recall sends its query vector in plaintext to Console and RuneSpace over TLS.
If prior capture is uncertain, skip it or ask the user. When a new decision
supersedes an earlier one, state what changed and cite the earlier `record_id`
if known; a new capture does not replace the old record.

Result handling:

- `captured: true` → confirm briefly, citing `record_id`.
- `captured: false` → older runtimes may reject near-duplicates. Relay the
  returned reason without assuming every runtime performs a similarity check.
  Do not retry automatically.
- `ok: false` → relay the error and its hint, applying these rules:
  - `PIPELINE_NOT_READY` → follow the hint; retry only after the pipeline is
    ready and the hint calls for it. Do not poll or loop.
  - `CONSOLE_UPGRADE_REQUIRED`, or an error mentioning `metadata_envelope_v2`
    → the Console administrator must upgrade Console first. After that upgrade
    is confirmed, direct the user to refresh the manifest once with
    `/rune:deactivate` then `/rune:activate`, or reconnect RUNE through `/mcp`
    or a new session. Activating an already active runtime does not refresh
    it. Do not repeatedly reactivate or resubmit capture.
  - `CONSOLE_DECRYPTION_ERROR` / `DataLoss` → stored metadata could not be
    validated. Do not retry; ask the Console administrator to investigate
    integrity and legacy migration status. Do not suggest enabling legacy
    reads, disabling validation, or downgrading.
  - An insert failure, timeout, or lost response can leave a record already
    stored. Do not claim it was not saved or submit a fresh capture without
    asking the user; a new call can create a duplicate. Do not use recall as
    a duplicate check.
