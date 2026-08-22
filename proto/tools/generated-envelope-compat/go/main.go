package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	cryptov1 "threadline.compat/generated/threadline/crypto/v1"
	messagev1 "threadline.compat/generated/threadline/message/v1"

	"google.golang.org/protobuf/proto"
)

func fail(format string, values ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", values...)
	os.Exit(1)
}

func hexFile(path string) []byte {
	raw, err := os.ReadFile(path)
	if err != nil {
		fail("read %s: %v", path, err)
	}
	decoded, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		fail("decode %s: %v", path, err)
	}
	return decoded
}

func wireFile(path string) []byte {
	raw, err := os.ReadFile(path)
	if err != nil {
		fail("read %s: %v", path, err)
	}
	return raw
}

func main() {
	if len(os.Args) < 5 {
		fail("usage: compat produce|relay|consume GOLDEN_ROOT INPUT_ROOT OUTPUT_ROOT [EXPECTED_LABEL OUTPUT_LABEL]")
	}
	mode, goldenRoot, inputRoot, outputRoot := os.Args[1], os.Args[2], os.Args[3], os.Args[4]
	if mode != "produce" && mode != "relay" && mode != "consume" {
		fail("unsupported mode: %s", mode)
	}
	var expectedLabel, outputLabel string
	if len(os.Args) > 5 {
		expectedLabel = os.Args[5]
	}
	if len(os.Args) > 6 {
		outputLabel = os.Args[6]
	}
	sourceIsGolden := mode == "produce"
	var channelInput, recoveryInput []byte
	if sourceIsGolden {
		channelInput = hexFile(filepath.Join(inputRoot, "channel-event-envelope.golden.hex"))
		recoveryInput = hexFile(filepath.Join(inputRoot, "recovery-envelope.golden.hex"))
	} else {
		channelInput = wireFile(filepath.Join(inputRoot, "channel.bin"))
		recoveryInput = wireFile(filepath.Join(inputRoot, "recovery.bin"))
	}
	channelCanary := hexFile(filepath.Join(goldenRoot, "ciphertext-envelope.canary.hex"))
	recoveryCanary := hexFile(filepath.Join(goldenRoot, "crypto-envelope.canary.hex"))

	channel := &messagev1.ChannelEventEnvelope{}
	if err := proto.Unmarshal(channelInput, channel); err != nil {
		fail("decode channel: %v", err)
	}
	if !bytes.Contains(channelInput, channelCanary) {
		fail("channel input dropped the exact field-50000 canary")
	}
	expectedChannel := expectedLabel
	if sourceIsGolden {
		expectedChannel = "evt-golden-0001"
	}
	if channel.EventId != expectedChannel {
		fail("channel expected %s; got %s", expectedChannel, channel.EventId)
	}

	recovery := &cryptov1.RecoveryEnvelope{}
	if err := proto.Unmarshal(recoveryInput, recovery); err != nil {
		fail("decode recovery: %v", err)
	}
	if !bytes.Contains(recoveryInput, recoveryCanary) {
		fail("recovery input dropped the exact field-50000 canary")
	}
	expectedRecovery := expectedLabel
	if sourceIsGolden {
		expectedRecovery = "recovery-key-golden-v7"
	}
	if recovery.RecoveryKeyId != expectedRecovery {
		fail("recovery expected %s; got %s", expectedRecovery, recovery.RecoveryKeyId)
	}

	if mode != "consume" {
		if outputLabel == "" {
			fail("%s requires an output label", mode)
		}
		channel.EventId = outputLabel
		channelOutput, err := proto.Marshal(channel)
		if err != nil || !bytes.Contains(channelOutput, channelCanary) {
			fail("channel generated adapter dropped the exact field-50000 canary: %v", err)
		}
		recovery.RecoveryKeyId = outputLabel
		recoveryOutput, err := proto.Marshal(recovery)
		if err != nil || !bytes.Contains(recoveryOutput, recoveryCanary) {
			fail("recovery generated adapter dropped the exact field-50000 canary: %v", err)
		}
		if err := os.MkdirAll(outputRoot, 0o700); err != nil {
			fail("create output: %v", err)
		}
		if err := os.WriteFile(filepath.Join(outputRoot, "channel.bin"), channelOutput, 0o600); err != nil {
			fail("write channel: %v", err)
		}
		if err := os.WriteFile(filepath.Join(outputRoot, "recovery.bin"), recoveryOutput, 0o600); err != nil {
			fail("write recovery: %v", err)
		}
	}

	displayedInput := expectedLabel
	if sourceIsGolden {
		displayedInput = "Golden Frames"
	}
	suffix := ""
	if outputLabel != "" {
		suffix = " -> " + outputLabel
	}
	fmt.Printf("Go %s passed for %s%s.\n", mode, displayedInput, suffix)
}
