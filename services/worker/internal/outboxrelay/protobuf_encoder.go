package outboxrelay

import (
	"context"
	"unicode/utf8"

	"github.com/monkeylabx/threadline/services/worker/internal/outboxdb"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ProtobufEncoder is the eight-field wire adapter for domain_event.proto.
// The compiled-schema Golden binds these tags; it does not install SDK output.
type ProtobufEncoder struct{}

func (ProtobufEncoder) Encode(ctx context.Context, facts outboxdb.PublishFacts) ([]byte, error) {
	if ctx == nil || !facts.Valid() {
		return nil, encodeError(EncoderFailureInvalidInput)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	timestamp := timestamppb.New(facts.OccurredAt)
	if timestamp.CheckValid() != nil || !utf8.ValidString(facts.TenantID) || !utf8.ValidString(facts.EventID) {
		return nil, encodeError(EncoderFailureEventPermanent)
	}
	stamp := wireInteger(nil, 1, uint64(timestamp.Seconds))
	if timestamp.Nanos != 0 {
		stamp = wireInteger(stamp, 2, uint64(timestamp.Nanos))
	}
	result := wireInteger(nil, 1, 1)
	result = wireBytes(result, 2, []byte(facts.TenantID))
	result = wireBytes(result, 3, []byte(facts.EventID))
	result = wireBytes(result, 4, []byte(facts.EventType))
	result = wireInteger(result, 5, uint64(facts.SchemaVersion))
	result = wireBytes(result, 6, stamp)
	if len(facts.Payload) > 0 {
		result = wireBytes(result, 7, facts.Payload)
	}
	result = wireInteger(result, 8, uint64(facts.OutboxEntryID))
	return result, nil
}

func wireInteger(target []byte, tag protowire.Number, value uint64) []byte {
	return protowire.AppendVarint(protowire.AppendTag(target, tag, protowire.VarintType), value)
}

func wireBytes(target []byte, tag protowire.Number, value []byte) []byte {
	return protowire.AppendBytes(protowire.AppendTag(target, tag, protowire.BytesType), value)
}
