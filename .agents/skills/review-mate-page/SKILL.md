---
name: review-mate-page
description: Resolve Mate pages quarantined as needs_review by inspecting the annotated page, correcting the persisted transcription, and updating only the affected Anki cards without asking Mate to regenerate the notebook.
---

# Review a Mate page

Use this workflow when Mate reports or notifies that a page is in `needs_review`. The review is completed in the conversation with the user; there is no `mate review` command.

## Invariants

- Treat the PDF and its annotated PNG as evidence. Never guess illegible content.
- Change only the selected note and page. Preserve unrelated SQLite rows, artifacts, Feynman prompts, Anki notes, and scheduling.
- A manually resolved page ends as `done`, with `processed_hash = observed_hash`. Do not leave it as `transcribed`, because that status asks Mate to generate material on its next run.
- Generate cards only for knowledge affected by the corrected page. Do not regenerate the notebook's full card set.
- Keep SQLite, `transcript.md`, `cards.json`, Feynman material, and Anki consistent before declaring the review complete.
- Never delete an Anki note unless the user has confirmed that it is obsolete. Prefer an in-place update when an old card has a clear replacement.

## Locate the review

Resolve configuration from the running process, environment, or Mate defaults instead of assuming user-specific paths. The defaults are `~/.mate/state.db` and `~/.mate/output`.

1. Query `pages` for `status = 'needs_review'`, ordered by `note_id, page_number`.
2. For the chosen row, inspect:
   - its `transcription`, `observed_hash`, and `processed_hash`;
   - `<output>/<note path without .pdf>/review/page-N.png`;
   - the corresponding PDF page when the review image lacks enough context;
   - the current `materials` row, `transcript.md`, `cards.json`, and affected Feynman prompt.
3. Show the annotated image to the user. Ask for input only when the source remains ambiguous.

If Mate is running, prevent a concurrent scan while persisting the resolution and restore its prior running state afterward. Back up the SQLite database with SQLite's backup mechanism before changing it; do not copy a live database while ignoring its WAL.

## Resolve the transcription

Produce the complete corrected Markdown for the page, not merely a replacement for `[?]`.

- For a readable content page, set `transcription` to the corrected Markdown, `processed_hash` to `observed_hash`, and `status` to `done`.
- For a confirmed cover or blank page, clear `transcription`, set `processed_hash` to `observed_hash`, and set `status` to `skipped`.
- If the evidence is still insufficient, leave the row in `needs_review` and stop without changing study material.

Perform related database changes in one transaction. Rebuild the note's `transcript.md` from all rows with a non-empty `processed_hash`, ordered by `page_number`, using the same headings as Mate.

## Repair study material

Compare the old and corrected page with the existing cards. Draft only additions, replacements, or removals directly attributable to that correction.

- `basic`: a focused question in `front` and its answer in `back`.
- `reversed`: a pair worth recalling in both directions.
- `cloze`: `front` must contain valid Anki syntax such as `{{c1::answer}}`; `back` may be empty.

Normalize whitespace and tags as Mate does. Preserve all unaffected entries in `materials.cards_json` and `cards.json`, then apply the reviewed subset. Patch an existing Feynman prompt only when the correction changes its substance; never regenerate unrelated prompts.

For an existing `materials` row, do not change `source_hash` merely because the transcription was manually corrected. If `synced_hash` already equaled `source_hash`, preserve both values. If they differed before the review, set them equal only after verifying that the entire stored card set—not merely the reviewed subset—is present in Anki. If the note has no material yet, construct a valid material record and compute `source_hash` with the current `materialSource` implementation; never invent or approximate it.

## Synchronize only affected Anki notes

Use AnkiConnect at the configured endpoint. Mate identifies the source note with the tag `mate_note_` plus the first 12 hexadecimal characters of `sha256(note_id)`. Its stable field is:

```text
MateID = "mate-" + hex(sha256(note_id + NUL + card_type + NUL + front))
```

For a one-to-one correction of the same card type, update the existing Anki note in place, including its new `MateID` when the front changed. This preserves its Anki note identity and scheduling. Use the Mate-owned note types and fields:

- `Mate Basic` and `Mate Reversed`: `Front`, `Back`, `MateID`;
- `Mate Cloze`: `Text`, `Extra`, `MateID`.

Create genuinely new cards with the correct Mate note type and source tag. When a stale card has no replacement, present it to the user before deleting it. Do not touch cards outside the affected concepts.

## Finish and verify

After SQLite, artifacts, and Anki all agree:

1. Remove only the resolved `review/page-N.png`; remove the review directory only if it becomes empty.
2. Restore the Mate process if it was running.
3. Verify the page is `done` or `skipped`, no affected card is duplicated, `cards.json` matches `materials.cards_json`, and `synced_hash = source_hash`.
4. Confirm from the next scan or logs that Mate ignored the unchanged page and made no model request for it.
5. Report the corrected page, cards added/updated/removed, any Feynman prompt changed, and the backup location.
