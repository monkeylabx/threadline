// Package messagerpc serves threadline.message.v1.MessageService for Slice A:
// human APPLICATION events in Channels, committed in one read-committed
// transaction that authorizes, observes the send-command binding, allocates
// the conversation sequence, stores the opaque envelope and appends the
// message.committed Domain Event, then answers with the persisted receipt.
//
// Everything outside that slice (direct messages, redaction, MLS and
// membership categories, attachments, agent attribution, epoch and rekey
// validation, hash-chain fields, checkpoint signing) fails closed with a
// stable reason instead of being approximated.
package messagerpc
