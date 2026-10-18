// Package channelrpc serves threadline.channel.v1.ChannelService from Core
// facts. Each implemented procedure authorizes through current PostgreSQL
// facts inside one read-committed transaction and reads inside that same
// transaction, so the row it returns is the row it authorized.
//
// Only GetChannel is implemented. Mutations stay Unimplemented until the
// visible Approval and durable Audit boundary that channelcommand waits for
// exists; this package must not bypass that boundary.
package channelrpc
