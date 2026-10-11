package masking

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func maskAll(t *testing.T, allSecret []string, allChunk ...string) string {
	t.Helper()
	var output bytes.Buffer
	masker := CreateSecretMasker(&output, allSecret)
	for _, chunk := range allChunk {
		masker.Write([]byte(chunk))
	}
	if err := masker.Flush(); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func TestMaskReplacesEveryOccurrence(t *testing.T) {
	allCase := []struct {
		allSecret []string
		input     string
		expected  string
	}{
		{[]string{"hunter22"}, "password=hunter22\nagain hunter22!", "password=<concealed by hisar>\nagain <concealed by hisar>!"},
		{[]string{"abcdef", "cdefgh"}, "xabcdefghy", "x<concealed by hisar>y"},
		{[]string{"abcd", "abcdefgh"}, "abcdefgh abcd", "<concealed by hisar> <concealed by hisar>"},
		{[]string{"aaaa"}, "aaaaaa", "<concealed by hisar>"},
		{[]string{"abcd"}, "abcdabcd", "<concealed by hisar><concealed by hisar>"},
		{[]string{"abc", "xy"}, "abc xy", "abc xy"},
		{[]string{"line one\nline two"}, "key:\nline one\nline two\nend", "key:\n<concealed by hisar>\nend"},
		{[]string{"\x00\xff\x10binary"}, "head\x00\xff\x10binarytail\x00", "head<concealed by hisar>tail\x00"},
		{[]string{"şifre-değeri"}, "değer: şifre-değeri.", "değer: <concealed by hisar>."},
		{nil, "nothing to hide", "nothing to hide"},
	}
	for _, testCase := range allCase {
		if output := maskAll(t, testCase.allSecret, testCase.input); output != testCase.expected {
			t.Errorf("mask %q in %q = %q; want %q", testCase.allSecret, testCase.input, output, testCase.expected)
		}
	}
}

func TestMaskFindsSecretsAcrossEveryWriteBoundary(t *testing.T) {
	allSecret := []string{"s3cr3t-value", "value-2-xyz", "token"}
	input := "a s3cr3t-value-2-xyz b token\ntokens s3cr3t-valu s3cr3t-value"
	expected := maskAll(t, allSecret, input)
	if expected != "a <concealed by hisar> b <concealed by hisar>\n<concealed by hisar>s s3cr3t-valu <concealed by hisar>" {
		t.Fatalf("whole input: %q", expected)
	}
	for first := 0; first <= len(input); first++ {
		for second := first; second <= len(input); second++ {
			if output := maskAll(t, allSecret, input[:first], input[first:second], input[second:]); output != expected {
				t.Fatalf("split at %d and %d: %q", first, second, output)
			}
		}
	}
	allByte := make([]string, 0, len(input))
	for index := range input {
		allByte = append(allByte, input[index:index+1])
	}
	if output := maskAll(t, allSecret, allByte...); output != expected {
		t.Errorf("one byte at a time: %q", output)
	}
}

func TestMaskHoldsBackOnlyAPossibleSecret(t *testing.T) {
	var output bytes.Buffer
	masker := CreateSecretMasker(&output, []string{"hunter22"})
	masker.Write([]byte("Enter the code: hun"))
	if output.String() != "Enter the code: " || !masker.hasHeld() {
		t.Errorf("written %q while a secret may follow", output.String())
	}
	masker.Write([]byte("gry\n"))
	if output.String() != "Enter the code: hungry\n" || masker.hasHeld() {
		t.Errorf("written %q after the secret did not follow", output.String())
	}
	masker.Write([]byte("no secret here, prompt> "))
	if output.String() != "Enter the code: hungry\nno secret here, prompt> " {
		t.Errorf("a prompt was held back: %q", output.String())
	}
}

func TestMaskSkipsShortAndDuplicateValues(t *testing.T) {
	masker := CreateSecretMasker(io.Discard, []string{"abc", "", "abcd", "abcd"})
	if len(masker.allSecret) != 1 || string(masker.allSecret[0]) != "abcd" {
		t.Errorf("secrets %q", masker.allSecret)
	}
}

type lockedBuffer struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (buffer *lockedBuffer) Write(data []byte) (int, error) {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()
	return buffer.buffer.Write(data)
}

func (buffer *lockedBuffer) String() string {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()
	return buffer.buffer.String()
}

func waitForText(t *testing.T, buffer *lockedBuffer, expected string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for buffer.String() != expected {
		if time.Now().After(deadline) {
			t.Fatalf("output %q; want %q", buffer.String(), expected)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestCopyMaskedFlushesAHeldTailAfterTheDelay(t *testing.T) {
	reader, writer := io.Pipe()
	var output lockedBuffer
	done := make(chan error, 1)
	go func() {
		done <- CopyMasked(CreateSecretMasker(&output, []string{"hunter22"}), reader, 50*time.Millisecond, nil)
	}()
	writer.Write([]byte("Password: hunt"))
	waitForText(t, &output, "Password: ", time.Second)
	startedAt := time.Now()
	waitForText(t, &output, "Password: hunt", time.Second)
	if elapsed := time.Since(startedAt); elapsed > 500*time.Millisecond {
		t.Errorf("the held tail waited %v", elapsed)
	}
	writer.Write([]byte("er22 and hunter22"))
	writer.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if output.String() != "Password: hunter22 and <concealed by hisar>" {
		t.Errorf("output %q", output.String())
	}
}

func TestCopyMaskedJoinsAValueSplitAcrossQuickWrites(t *testing.T) {
	reader, writer := io.Pipe()
	var output lockedBuffer
	done := make(chan error, 1)
	go func() {
		done <- CopyMasked(CreateSecretMasker(&output, []string{"hunter22"}), reader, time.Second, nil)
	}()
	for _, chunk := range []string{"token=hu", "nte", "r22\n"} {
		writer.Write([]byte(chunk))
		time.Sleep(10 * time.Millisecond)
	}
	writer.Close()
	<-done
	if output.String() != "token=<concealed by hisar>\n" {
		t.Errorf("output %q", output.String())
	}
}

func TestCopyMaskedStopsWhenAsked(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	var output lockedBuffer
	stop := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- CopyMasked(CreateSecretMasker(&output, []string{"hunter22"}), reader, time.Hour, stop) }()
	writer.Write([]byte("left open by a background process: hunt"))
	waitForText(t, &output, "left open by a background process: ", time.Second)
	close(stop)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("CopyMasked did not stop")
	}
	if !strings.HasSuffix(output.String(), "hunt") {
		t.Errorf("the held tail was lost: %q", output.String())
	}
}
