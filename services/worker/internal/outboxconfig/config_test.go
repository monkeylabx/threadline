package outboxconfig

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

const defaultDocument = `{
  "policy_id": "threadline.outbox.policy/v1",
  "payload_hard_bytes": 262144,
  "wire_hard_bytes": 327680,
  "batch_size": 64,
  "lease_ms": 30000,
  "absolute_lifetime_ms": 300000,
  "event_retry_ceiling": 8,
  "transport_base_ms": 1000,
  "transport_cap_ms": 60000,
  "unknown_base_ms": 5000,
  "unknown_cap_ms": 300000,
  "event_base_ms": 5000,
  "event_cap_ms": 300000,
  "retention_days": 90,
  "duplicate_window_ms": 120000
}`

func TestParseDefaultSnapshotAndGoldenDigest(t *testing.T) {
	t.Parallel()

	snapshot := mustParse(t, defaultDocument)
	if !snapshot.Valid() || snapshot.PolicyID() != PolicyV1ID || snapshot.PayloadHardBytes() != 262_144 ||
		snapshot.WireHardBytes() != 327_680 || snapshot.BatchSize() != 64 ||
		snapshot.Lease() != 30*time.Second || snapshot.AbsoluteLifetime() != 5*time.Minute ||
		snapshot.EventRetryCeiling() != 8 || snapshot.RetentionDays() != 90 ||
		snapshot.DuplicateWindow() != 2*time.Minute {
		t.Fatalf("default snapshot has incorrect frozen values: %#v", snapshot)
	}
	if snapshot.TransportBackoff().Base() != time.Second || snapshot.TransportBackoff().Cap() != time.Minute ||
		snapshot.UnknownBackoff().Base() != 5*time.Second || snapshot.UnknownBackoff().Cap() != 5*time.Minute ||
		snapshot.EventBackoff().Base() != 5*time.Second || snapshot.EventBackoff().Cap() != 5*time.Minute {
		t.Fatal("default snapshot has incorrect backoff values")
	}

	wantPreimage := "7468726561646c696e652e6f7574626f782e706f6c6963792d736e617073686f742f76310000040000000500000000004000007530000493e000000008000003e80000ea6000001388000493e000001388000493e00000005a0001d4c0"
	wantDigest := "9c9d119ee28a1237b0c9b95cdf3a79dff57132dbb871d202cb937d5f5b72dec5"
	values := snapshot.values()
	if got := hex.EncodeToString(policyPreimage(values)); got != wantPreimage {
		t.Fatalf("preimage = %s, want golden %s", got, wantPreimage)
	}
	digest := snapshot.Digest()
	if got := hex.EncodeToString(digest[:]); got != wantDigest {
		t.Fatalf("digest = %s, want golden %s", got, wantDigest)
	}
}

func TestParseAcceptsEveryFrozenBoundary(t *testing.T) {
	t.Parallel()

	tests := []struct{ name, document string }{
		{name: "batch minimum", document: replaceSetting(t, defaultDocument, "batch_size", "64", "1")},
		{name: "batch maximum", document: replaceSetting(t, defaultDocument, "batch_size", "64", "256")},
		{name: "lease minimum", document: replaceSetting(t, defaultDocument, "lease_ms", "30000", "5000")},
		{name: "lease maximum", document: replaceSetting(t, defaultDocument, "lease_ms", "30000", "120000")},
		{name: "absolute minimum", document: replaceSetting(t, replaceSetting(t, defaultDocument, "lease_ms", "30000", "5000"), "absolute_lifetime_ms", "300000", "30000")},
		{name: "absolute maximum", document: replaceSetting(t, defaultDocument, "absolute_lifetime_ms", "300000", "900000")},
		{name: "absolute equals twice lease", document: replaceSetting(t, replaceSetting(t, defaultDocument, "lease_ms", "30000", "15000"), "absolute_lifetime_ms", "300000", "30000")},
		{name: "event ceiling minimum", document: replaceSetting(t, defaultDocument, "event_retry_ceiling", "8", "1")},
		{name: "event ceiling maximum", document: replaceSetting(t, defaultDocument, "event_retry_ceiling", "8", "20")},
		{name: "transport minima", document: replaceSetting(t, replaceSetting(t, defaultDocument, "transport_base_ms", "1000", "100"), "transport_cap_ms", "60000", "1000")},
		{name: "transport maxima", document: replaceSetting(t, replaceSetting(t, defaultDocument, "transport_base_ms", "1000", "10000"), "transport_cap_ms", "60000", "300000")},
		{name: "transport cap equals base", document: replaceSetting(t, defaultDocument, "transport_cap_ms", "60000", "1000")},
		{name: "unknown minima", document: replaceSetting(t, replaceSetting(t, defaultDocument, "unknown_base_ms", "5000", "500"), "unknown_cap_ms", "300000", "5000")},
		{name: "unknown maxima", document: replaceSetting(t, replaceSetting(t, defaultDocument, "unknown_base_ms", "5000", "30000"), "unknown_cap_ms", "300000", "900000")},
		{name: "unknown cap equals base", document: replaceSetting(t, defaultDocument, "unknown_cap_ms", "300000", "5000")},
		{name: "event minima", document: replaceSetting(t, replaceSetting(t, defaultDocument, "event_base_ms", "5000", "500"), "event_cap_ms", "300000", "5000")},
		{name: "event maxima", document: replaceSetting(t, replaceSetting(t, defaultDocument, "event_base_ms", "5000", "30000"), "event_cap_ms", "300000", "900000")},
		{name: "event cap equals base", document: replaceSetting(t, defaultDocument, "event_cap_ms", "300000", "5000")},
		{name: "retention minimum", document: replaceSetting(t, defaultDocument, "retention_days", "90", "30")},
		{name: "retention maximum", document: replaceSetting(t, defaultDocument, "retention_days", "90", "365")},
		{name: "surrounding whitespace", document: " \n\t" + defaultDocument + "\r\n "},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			mustParse(t, test.document)
		})
	}
}

func TestParseRejectsEveryFrozenInvalidSpellingAndShape(t *testing.T) {
	t.Parallel()

	tests := []struct{ name, document string }{
		{name: "empty"},
		{name: "BOM", document: "\ufeff" + defaultDocument},
		{name: "null root", document: "null"},
		{name: "boolean root", document: "true"},
		{name: "string root", document: `"config"`},
		{name: "array root", document: "[]"},
		{name: "second document", document: defaultDocument + "{}"},
		{name: "trailing junk", document: defaultDocument + "x"},
		{name: "trailing comma", document: strings.TrimSuffix(defaultDocument, "\n}") + ",\n}"},
		{name: "oversized", document: defaultDocument + strings.Repeat(" ", maximumDocumentBytes)},
		{name: "missing key", document: strings.Replace(defaultDocument, "  \"batch_size\": 64,\n", "", 1)},
		{name: "unknown key", document: strings.Replace(defaultDocument, "{", "{\n  \"canary-unknown\": 1,", 1)},
		{name: "duplicate key", document: strings.Replace(defaultDocument, "{", "{\n  \"batch_size\": 64,", 1)},
		{name: "unknown policy", document: strings.Replace(defaultDocument, PolicyV1ID, "threadline.outbox.policy/v2", 1)},
		{name: "policy id non-string", document: strings.Replace(defaultDocument, `"`+PolicyV1ID+`"`, "null", 1)},
		{name: "numeric null", document: replaceSetting(t, defaultDocument, "batch_size", "64", "null")},
		{name: "numeric boolean", document: replaceSetting(t, defaultDocument, "batch_size", "64", "true")},
		{name: "numeric string", document: replaceSetting(t, defaultDocument, "batch_size", "64", `"64"`)},
		{name: "numeric array", document: replaceSetting(t, defaultDocument, "batch_size", "64", "[]")},
		{name: "numeric object", document: replaceSetting(t, defaultDocument, "batch_size", "64", "{}")},
		{name: "negative", document: replaceSetting(t, defaultDocument, "batch_size", "64", "-1")},
		{name: "negative zero", document: replaceSetting(t, defaultDocument, "batch_size", "64", "-0")},
		{name: "leading zero", document: replaceSetting(t, defaultDocument, "batch_size", "64", "064")},
		{name: "exponent", document: replaceSetting(t, defaultDocument, "batch_size", "64", "1e2")},
		{name: "decimal", document: replaceSetting(t, defaultDocument, "batch_size", "64", "64.0")},
		{name: "fractional", document: replaceSetting(t, defaultDocument, "batch_size", "64", "0.5")},
		{name: "uint32 overflow", document: replaceSetting(t, defaultDocument, "batch_size", "64", "4294967296")},
		{name: "int64 overflow", document: replaceSetting(t, defaultDocument, "batch_size", "64", "9223372036854775808")},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertInvalid(t, []byte(test.document))
		})
	}
}

func TestParseRejectsEveryFrozenSemanticViolation(t *testing.T) {
	t.Parallel()

	tests := []struct{ name, document string }{
		{name: "payload lower", document: replaceSetting(t, defaultDocument, "payload_hard_bytes", "262144", "262143")},
		{name: "payload higher", document: replaceSetting(t, defaultDocument, "payload_hard_bytes", "262144", "262145")},
		{name: "wire lower", document: replaceSetting(t, defaultDocument, "wire_hard_bytes", "327680", "327679")},
		{name: "wire higher", document: replaceSetting(t, defaultDocument, "wire_hard_bytes", "327680", "327681")},
		{name: "batch zero", document: replaceSetting(t, defaultDocument, "batch_size", "64", "0")},
		{name: "batch high", document: replaceSetting(t, defaultDocument, "batch_size", "64", "257")},
		{name: "lease low", document: replaceSetting(t, defaultDocument, "lease_ms", "30000", "4999")},
		{name: "lease high", document: replaceSetting(t, defaultDocument, "lease_ms", "30000", "120001")},
		{name: "absolute low", document: replaceSetting(t, defaultDocument, "absolute_lifetime_ms", "300000", "29999")},
		{name: "absolute high", document: replaceSetting(t, defaultDocument, "absolute_lifetime_ms", "300000", "900001")},
		{name: "absolute below twice lease", document: replaceSetting(t, replaceSetting(t, defaultDocument, "lease_ms", "30000", "120000"), "absolute_lifetime_ms", "300000", "239999")},
		{name: "event ceiling zero", document: replaceSetting(t, defaultDocument, "event_retry_ceiling", "8", "0")},
		{name: "event ceiling high", document: replaceSetting(t, defaultDocument, "event_retry_ceiling", "8", "21")},
		{name: "transport base low", document: replaceSetting(t, defaultDocument, "transport_base_ms", "1000", "99")},
		{name: "transport base high", document: replaceSetting(t, defaultDocument, "transport_base_ms", "1000", "10001")},
		{name: "transport cap low", document: replaceSetting(t, defaultDocument, "transport_cap_ms", "60000", "999")},
		{name: "transport cap high", document: replaceSetting(t, defaultDocument, "transport_cap_ms", "60000", "300001")},
		{name: "transport cap below base", document: replaceSetting(t, replaceSetting(t, defaultDocument, "transport_base_ms", "1000", "10000"), "transport_cap_ms", "60000", "9999")},
		{name: "unknown base low", document: replaceSetting(t, defaultDocument, "unknown_base_ms", "5000", "499")},
		{name: "unknown base high", document: replaceSetting(t, defaultDocument, "unknown_base_ms", "5000", "30001")},
		{name: "unknown cap low", document: replaceSetting(t, defaultDocument, "unknown_cap_ms", "300000", "4999")},
		{name: "unknown cap high", document: replaceSetting(t, defaultDocument, "unknown_cap_ms", "300000", "900001")},
		{name: "unknown cap below base", document: replaceSetting(t, replaceSetting(t, defaultDocument, "unknown_base_ms", "5000", "30000"), "unknown_cap_ms", "300000", "29999")},
		{name: "event base low", document: replaceSetting(t, defaultDocument, "event_base_ms", "5000", "499")},
		{name: "event base high", document: replaceSetting(t, defaultDocument, "event_base_ms", "5000", "30001")},
		{name: "event cap low", document: replaceSetting(t, defaultDocument, "event_cap_ms", "300000", "4999")},
		{name: "event cap high", document: replaceSetting(t, defaultDocument, "event_cap_ms", "300000", "900001")},
		{name: "event cap below base", document: replaceSetting(t, replaceSetting(t, defaultDocument, "event_base_ms", "5000", "30000"), "event_cap_ms", "300000", "29999")},
		{name: "retention low", document: replaceSetting(t, defaultDocument, "retention_days", "90", "29")},
		{name: "retention high", document: replaceSetting(t, defaultDocument, "retention_days", "90", "366")},
		{name: "duplicate lower", document: replaceSetting(t, defaultDocument, "duplicate_window_ms", "120000", "119999")},
		{name: "duplicate higher", document: replaceSetting(t, defaultDocument, "duplicate_window_ms", "120000", "120001")},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertInvalid(t, []byte(test.document))
		})
	}
}

func TestEveryAcceptedSemanticChangeChangesDigest(t *testing.T) {
	t.Parallel()

	baseline := mustParse(t, defaultDocument).Digest()
	changes := []struct{ key, from, to string }{
		{key: "batch_size", from: "64", to: "63"},
		{key: "lease_ms", from: "30000", to: "29999"},
		{key: "absolute_lifetime_ms", from: "300000", to: "299999"},
		{key: "event_retry_ceiling", from: "8", to: "7"},
		{key: "transport_base_ms", from: "1000", to: "999"},
		{key: "transport_cap_ms", from: "60000", to: "59999"},
		{key: "unknown_base_ms", from: "5000", to: "4999"},
		{key: "unknown_cap_ms", from: "300000", to: "299999"},
		{key: "event_base_ms", from: "5000", to: "4999"},
		{key: "event_cap_ms", from: "300000", to: "299999"},
		{key: "retention_days", from: "90", to: "89"},
	}
	for _, change := range changes {
		change := change
		t.Run(change.key, func(t *testing.T) {
			t.Parallel()
			changed := mustParse(t, replaceSetting(t, defaultDocument, change.key, change.from, change.to)).Digest()
			if changed == baseline {
				t.Fatal("accepted semantic change did not change digest")
			}
		})
	}
}

func TestSnapshotIsDetachedRedactedAndRaceSafe(t *testing.T) {
	t.Parallel()

	document := []byte(defaultDocument)
	snapshot, err := Parse(document)
	if err != nil {
		t.Fatal(err)
	}
	before := snapshot.Digest()
	for index := range document {
		document[index] = 'X'
	}
	if snapshot.Digest() != before || snapshot.PolicyID() != PolicyV1ID || !snapshot.Valid() {
		t.Fatal("caller mutation changed snapshot")
	}
	if (Snapshot{}).Valid() {
		t.Fatal("zero Snapshot reported valid")
	}

	const readers = 128
	var group sync.WaitGroup
	group.Add(readers)
	for range readers {
		go func() {
			defer group.Done()
			if snapshot.Digest() != before || snapshot.BatchSize() != 64 || snapshot.Lease() != 30*time.Second {
				t.Errorf("concurrent snapshot read changed")
			}
		}()
	}
	group.Wait()

	for _, value := range []any{snapshot, snapshot.TransportBackoff(), ErrorInvalidInput} {
		for _, rendered := range []string{fmt.Sprint(value), fmt.Sprintf("%+v", value), fmt.Sprintf("%#v", value)} {
			if strings.Contains(rendered, PolicyV1ID) || strings.Contains(rendered, "300000") {
				t.Fatalf("formatting exposed snapshot internals: %q", rendered)
			}
		}
		encoded, marshalErr := json.Marshal(value)
		if marshalErr != nil || strings.Contains(string(encoded), PolicyV1ID) {
			t.Fatalf("JSON exposed snapshot internals: %q/%v", encoded, marshalErr)
		}
	}
}

func TestInvalidErrorsAreStableAndDoNotEchoInput(t *testing.T) {
	t.Parallel()

	const canary = "raw-config-secret-canary"
	_, err := Parse([]byte(`{"` + canary + `":1}`))
	code, ok := ErrorCodeOf(err)
	if !ok || code != ErrorInvalidInput || strings.Contains(fmt.Sprintf("%v %+v %#v", err, err, err), canary) {
		t.Fatalf("invalid error = %v/%v/%v", err, code, ok)
	}
	if _, ok := ErrorCodeOf(errors.New(canary)); ok {
		t.Fatal("foreign error was classified")
	}
	encoded, marshalErr := json.Marshal(err)
	if marshalErr != nil || strings.Contains(string(encoded), canary) {
		t.Fatalf("error JSON = %q/%v", encoded, marshalErr)
	}
}

func mustParse(t *testing.T, document string) Snapshot {
	t.Helper()
	snapshot, err := Parse([]byte(document))
	if err != nil {
		t.Fatalf("Parse = %v", err)
	}
	return snapshot
}

func assertInvalid(t *testing.T, document []byte) {
	t.Helper()
	snapshot, err := Parse(document)
	if snapshot != (Snapshot{}) {
		t.Fatalf("invalid Parse returned snapshot: %#v", snapshot)
	}
	if code, ok := ErrorCodeOf(err); !ok || code != ErrorInvalidInput {
		t.Fatalf("invalid Parse error = %v/%v/%v", err, code, ok)
	}
}

func replaceSetting(t *testing.T, document, key, from, to string) string {
	t.Helper()
	old := `"` + key + `": ` + from
	if strings.Count(document, old) != 1 {
		t.Fatalf("fixture occurrence %q = %d, want one", old, strings.Count(document, old))
	}
	return strings.Replace(document, old, `"`+key+`": `+to, 1)
}
