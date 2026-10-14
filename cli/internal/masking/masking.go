// Package masking replaces secret values in a command's output with a fixed text.
package masking

import (
	"bytes"
	"io"
	"slices"
	"time"
)

const (
	ConcealedText = "<concealed by hisar>"
	MinimumLength = 4
	FlushDelay    = 100 * time.Millisecond
)

type byteRange struct {
	start int
	end   int
}

// SecretMasker replaces every occurrence of a secret with ConcealedText. It holds back only a tail that could
// still become a secret, so a value split across writes is still found.
type SecretMasker struct {
	secrets       [][]byte
	maximumLength int
	held          []byte
	output        io.Writer
}

func NewSecretMasker(output io.Writer, values []string) *SecretMasker {
	masker := &SecretMasker{output: output}
	for _, value := range values {
		if len(value) < MinimumLength || slices.ContainsFunc(masker.secrets, func(secret []byte) bool { return string(secret) == value }) {
			continue
		}
		masker.secrets = append(masker.secrets, []byte(value))
		masker.maximumLength = max(masker.maximumLength, len(value))
	}
	return masker
}

func (masker *SecretMasker) Write(chunk []byte) (int, error) {
	masker.held = append(masker.held, chunk...)
	return len(chunk), masker.emit(false)
}

func (masker *SecretMasker) Flush() error {
	return masker.emit(true)
}

func (masker *SecretMasker) hasHeld() bool {
	return len(masker.held) > 0
}

func (masker *SecretMasker) emit(isFinal bool) error {
	data := masker.held
	holdFrom := len(data)
	if !isFinal {
		holdFrom = masker.findPartialStart(data)
	}
	runs := masker.findRuns(data)
	for _, run := range runs {
		if run.start < holdFrom && run.end > holdFrom {
			holdFrom = run.start
		}
	}
	output := make([]byte, 0, holdFrom)
	position := 0
	for _, run := range runs {
		if run.end > holdFrom {
			break
		}
		output = append(output, data[position:run.start]...)
		output = append(output, ConcealedText...)
		position = run.end
	}
	output = append(output, data[position:holdFrom]...)
	masker.held = append([]byte(nil), data[holdFrom:]...)
	if len(output) == 0 {
		return nil
	}
	_, err := masker.output.Write(output)
	return err
}

// findPartialStart returns where the earliest tail of data starts that is a proper prefix of a secret.
func (masker *SecretMasker) findPartialStart(data []byte) int {
	for start := max(0, len(data)-masker.maximumLength+1); start < len(data); start++ {
		tail := data[start:]
		for _, secret := range masker.secrets {
			if len(secret) > len(tail) && bytes.HasPrefix(secret, tail) {
				return start
			}
		}
	}
	return len(data)
}

// findRuns finds every occurrence of every secret and merges the ones that overlap into one run.
func (masker *SecretMasker) findRuns(data []byte) []byteRange {
	var matches []byteRange
	for _, secret := range masker.secrets {
		for start := 0; start <= len(data)-len(secret); {
			index := bytes.Index(data[start:], secret)
			if index < 0 {
				break
			}
			matches = append(matches, byteRange{start: start + index, end: start + index + len(secret)})
			start += index + 1
		}
	}
	slices.SortFunc(matches, func(first, second byteRange) int { return first.start - second.start })
	var runs []byteRange
	for _, match := range matches {
		if last := len(runs) - 1; last >= 0 && match.start < runs[last].end {
			runs[last].end = max(runs[last].end, match.end)
			continue
		}
		runs = append(runs, match)
	}
	return runs
}

// CopyMasked copies source to the masker until source ends or stop closes; a held tail is written as it is
// once nothing completes it within flushDelay.
func CopyMasked(masker *SecretMasker, source io.Reader, flushDelay time.Duration, stop <-chan struct{}) error {
	type readResult struct {
		chunk []byte
		err   error
	}
	readChannel := make(chan readResult, 1)
	go func() {
		buffer := make([]byte, 32*1024)
		for {
			count, err := source.Read(buffer)
			if count > 0 {
				readChannel <- readResult{chunk: append([]byte(nil), buffer[:count]...)}
			}
			if err != nil {
				readChannel <- readResult{err: err}
				return
			}
		}
	}()
	flushTimer := time.NewTimer(flushDelay)
	flushTimer.Stop()
	defer flushTimer.Stop()
	var writeError error
	for {
		select {
		case result := <-readChannel:
			if result.err != nil {
				if err := masker.Flush(); writeError == nil {
					writeError = err
				}
				return writeError
			}
			if _, err := masker.Write(result.chunk); err != nil && writeError == nil {
				writeError = err
			}
			flushTimer.Stop()
			if masker.hasHeld() {
				flushTimer.Reset(flushDelay)
			}
		case <-flushTimer.C:
			if err := masker.Flush(); err != nil && writeError == nil {
				writeError = err
			}
		case <-stop:
			if err := masker.Flush(); writeError == nil {
				writeError = err
			}
			return writeError
		}
	}
}
