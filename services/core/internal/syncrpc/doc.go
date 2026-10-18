// Package syncrpc serves threadline.sync.v1.SyncService for Slice A. Cursors
// belong to the authenticated Device: GetCursors lists that Device's applied
// positions and UpdateCursors persists monotonic, contiguous positions at or
// below committed history, rejecting each invalid cursor individually with the
// safe server position instead of failing the whole request.
//
// Direct messages, read cursors, gap repair, streaming and checkpoints are not
// implemented in this slice and fail closed with stable reasons.
package syncrpc
