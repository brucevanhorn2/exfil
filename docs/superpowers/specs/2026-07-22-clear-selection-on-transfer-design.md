# Clear selection on successful transfer

## Goal

After a marked (multi-selected) file finishes transferring successfully, its
checkmark in the source pane should clear automatically — no extra keypress,
no lingering marks on files that already moved.

## Behavior

- Applies only to files that were explicitly marked (space-bar multi-select),
  not the implicit single-file cursor case (which has no `Selected` entry to
  clear anyway).
- Clearing is per-file and automatic: the instant a given file's transfer
  reports success (`TransferDoneMsg`), that file's mark is removed from the
  *source* pane's selection — independent of any other files in the same
  batch.
- Failed transfers (`TransferErrorMsg`) leave the mark untouched, so failed
  files stay visibly selected and can be retried without re-marking.
- No status message and no new keybinding — this is a quiet visual update
  (the checkbox disappears) consistent with how progress/queue updates
  already render live.

## Design

`Model.transferDest` (`internal/ui/app.go`) currently maps an in-flight
transfer ID to the destination pane name only:

```go
transferDest map[int]string
```

Extend this to a small struct that also carries the source pane name and the
filename — both already known to `enqueueCopyDirection` at the point it
queues each job:

```go
type transferInfo struct {
    destPane string // "local" or "remote"
    srcPane  string // "local" or "remote"
    filename string
}

transferDest map[int]transferInfo
```

- `setTransferDest(id, destName, srcName, filename)` — updated signature.
- `popTransferDest(id) (transferInfo, bool)` — updated return type.
- `enqueueCopyDirection` passes the source pane name (`"local"`/`"remote"`,
  same convention as `dstName`) and `filename` when it calls
  `setTransferDest`.

In `Update()`:

- `TransferDoneMsg`: after popping `transferInfo`, resolve the *source* pane
  (`m.localPane`/`m.remotePane` by `srcPane`) and clear its selection for
  `filename`, in addition to the existing dest-pane refresh.
- `TransferErrorMsg`: unchanged — pop and discard, no selection change.

New method on `BrowserPane` (`internal/ui/browser.go`):

```go
// ClearSelected removes name from the pane's selection, if present.
func (b *BrowserPane) ClearSelected(name string) {
    delete(b.Selected, name)
}
```

## Testing

- Unit test on `BrowserPane.ClearSelected`: clears an existing mark, no-op on
  an unmarked/absent name.
- Unit/integration test around the `Update()` transfer-message handling
  (mirroring existing `TransferDoneMsg`/`TransferErrorMsg` tests if present):
  mark 2+ files, simulate one `TransferDoneMsg` and one `TransferErrorMsg`,
  assert only the succeeded file's mark is cleared.
- Manual verification with `-t` test mode: mark multiple files, push across,
  watch marks clear as each completes.

## Out of scope

- No change to directory transfers (still unsupported).
- No change to cursor-only (unmarked) transfers.
- No new status message or keybinding.
