// Package outboxpublish contains the Worker's private, single-attempt
// JetStream publish seam. It accepts already encoded Event-envelope bytes and
// never interprets them or permits a per-message broker target.
package outboxpublish
