package fanout

import (
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func hintBytes(fields map[protowire.Number][]byte) []byte {
	var body []byte
	for _, tag := range []protowire.Number{1, 2, 3, 4, 5, 6, 7, 8} {
		value, ok := fields[tag]
		if !ok {
			continue
		}
		kind := protowire.BytesType
		if tag == 1 || tag == 5 || tag == 8 {
			kind = protowire.VarintType
		}
		body = protowire.AppendTag(body, tag, kind)
		body = append(body, value...)
	}
	return body
}
func hintFields() map[protowire.Number][]byte {
	return map[protowire.Number][]byte{1: {1}, 2: protowire.AppendBytes(nil, []byte("t")), 3: protowire.AppendBytes(nil, []byte("e")),
		4: protowire.AppendBytes(nil, []byte("message.committed")), 5: {1}, 6: protowire.AppendBytes(nil, []byte{8, 1}), 8: {1}}
}

func TestHintAcceptsOnlyRegisteredBoundedEnvelope(t *testing.T) {
	body := hintBytes(hintFields())
	if !messageHint(body) {
		t.Fatal("valid hint rejected")
	}
	canary := protowire.AppendBytes(protowire.AppendTag(body, 50000, protowire.BytesType), []byte("unknown"))
	if !messageHint(canary) {
		t.Fatal("unknown field prevented hint")
	}
	for _, tag := range []protowire.Number{1, 2, 3, 4, 5, 6, 8} {
		fields := hintFields()
		delete(fields, tag)
		if messageHint(hintBytes(fields)) {
			t.Fatalf("missing field %d accepted", tag)
		}
	}
	for tag, value := range map[protowire.Number][]byte{1: {0}, 2: protowire.AppendBytes(nil, []byte(" bad ")), 3: protowire.AppendBytes(nil, []byte{255}),
		4: protowire.AppendBytes(nil, []byte("unknown.event")), 5: {2}, 6: protowire.AppendBytes(nil, []byte{255}), 7: protowire.AppendBytes(nil, make([]byte, 262145)), 8: {0}} {
		fields := hintFields()
		fields[tag] = value
		if messageHint(hintBytes(fields)) {
			t.Fatalf("bad field %d accepted", tag)
		}
	}
	for _, bad := range [][]byte{nil, {0}, make([]byte, 327681), append(body, 8, 1), append(body, 10, 0), append(body, 255)} {
		if messageHint(bad) {
			t.Fatal("invalid framing accepted")
		}
	}
}
