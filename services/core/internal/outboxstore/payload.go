package outboxstore

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
)

const (
	payloadKindChannel = 0x01
	domainEventIDBytes = 16
)

// encodeChannelEventPayload produces payload layout v1: one kind byte, the
// big-endian uint32 identifier length, the UTF-8 identifier and the big-endian
// uint64 sequence. It carries no message content so a permanent Domain Event
// row never outlives retention or redaction of the message itself.
// Golden vector: channel "c", sequence 1 → 0100000001630000000000000001.
func encodeChannelEventPayload(channelID string, channelSeq uint64) []byte {
	payload := make([]byte, 0, 1+4+len(channelID)+8)
	payload = append(payload, payloadKindChannel)
	payload = binary.BigEndian.AppendUint32(payload, uint32(len(channelID)))
	payload = append(payload, channelID...)
	return binary.BigEndian.AppendUint64(payload, channelSeq)
}

// newDomainEventID returns 128 CSPRNG bits as 32 lowercase hexadecimal
// characters. The ID is server-generated because Outbox identity is tenant-wide
// across producers; a client-chosen value could pre-empt another producer's ID.
func newDomainEventID() (string, error) {
	var raw [domainEventIDBytes]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
