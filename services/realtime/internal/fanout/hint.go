package fanout

import (
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// messageHint validates the transport before treating it as a wake-up only.
// Unknown fields are skipped without interpreting payload or creating authority.
func messageHint(body []byte) bool {
	if len(body) == 0 || len(body) > 327680 {
		return false
	}
	var state hintState
	for len(body) > 0 {
		number, kind, n := protowire.ConsumeTag(body)
		if n < 0 || number <= 0 {
			return false
		}
		body = body[n:]
		size := protowire.ConsumeFieldValue(number, kind, body)
		if size < 0 {
			return false
		}
		if !state.accept(number, kind, body[:size]) {
			return false
		}
		body = body[size:]
	}
	return state.recognized && state.seen&0x17e == 0x17e
}

type hintState struct {
	seen       uint16
	recognized bool
}

func (state *hintState) accept(number protowire.Number, kind protowire.Type, wire []byte) bool {
	if number < 1 || number > 8 {
		return true
	}
	bit := uint16(1) << number
	if state.seen&bit != 0 {
		return false
	}
	state.seen |= bit
	if !validHintField(number, kind, wire) {
		return false
	}
	if number == 4 {
		value, _ := protowire.ConsumeBytes(wire)
		state.recognized = string(value) == "message.committed"
	}
	return true
}

func integerField(number protowire.Number) bool {
	switch number {
	case 1, 5, 8:
		return true
	default:
		return false
	}
}

func validHintInteger(number protowire.Number, kind protowire.Type, wire []byte) bool {
	if kind != protowire.VarintType {
		return false
	}
	value, n := protowire.ConsumeVarint(wire)
	if n < 0 || value == 0 {
		return false
	}
	if number == 8 {
		return value <= math.MaxInt64
	}
	return value == 1
}

func validHintField(number protowire.Number, kind protowire.Type, wire []byte) bool {
	if integerField(number) {
		return validHintInteger(number, kind, wire)
	}
	if kind != protowire.BytesType {
		return false
	}
	value, n := protowire.ConsumeBytes(wire)
	if n < 0 {
		return false
	}
	switch number {
	case 6:
		var stamp timestamppb.Timestamp
		return len(value) <= 64 && proto.Unmarshal(value, &stamp) == nil && stamp.CheckValid() == nil
	case 7:
		return len(value) <= 262144
	default:
		text := string(value)
		return text != "" && utf8.Valid(value) && strings.TrimSpace(text) == text && !strings.ContainsFunc(text, unicode.IsControl)
	}
}
