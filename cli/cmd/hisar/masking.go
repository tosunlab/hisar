package main

import (
	"bytes"
	"io"
	"slices"
	"time"
)

const (
	concealedText       = "<concealed by hisar>"
	minimumMaskedLength = 4
	maskingFlushDelay   = 100 * time.Millisecond
)

type byteRange struct {
	start int
	end   int
}

// secretMasker replaces every occurrence of a secret with concealedText. It holds back only a tail that could
// still become a secret, so a value split across writes is still found.
type secretMasker struct {
	allSecret     [][]byte
	maximumLength int
	held          []byte
	output        io.Writer
}

func createSecretMasker(output io.Writer, allValue []string) *secretMasker {
	masker := &secretMasker{output: output}
	for _, value := range allValue {
		if len(value) < minimumMaskedLength || slices.ContainsFunc(masker.allSecret, func(secret []byte) bool { return string(secret) == value }) {
			continue
		}
		masker.allSecret = append(masker.allSecret, []byte(value))
		masker.maximumLength = max(masker.maximumLength, len(value))
	}
	return masker
}

func (masker *secretMasker) Write(chunk []byte) (int, error) {
	masker.held = append(masker.held, chunk...)
	return len(chunk), masker.emit(false)
}

func (masker *secretMasker) Flush() error {
	return masker.emit(true)
}

func (masker *secretMasker) hasHeld() bool {
	return len(masker.held) > 0
}

func (masker *secretMasker) emit(isFinal bool) error {
	data := masker.held
	holdFrom := len(data)
	if !isFinal {
		holdFrom = masker.findPartialStart(data)
	}
	allRun := masker.findAllRun(data)
	for _, run := range allRun {
		if run.start < holdFrom && run.end > holdFrom {
			holdFrom = run.start
		}
	}
	output := make([]byte, 0, holdFrom)
	position := 0
	for _, run := range allRun {
		if run.end > holdFrom {
			break
		}
		output = append(output, data[position:run.start]...)
		output = append(output, concealedText...)
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
func (masker *secretMasker) findPartialStart(data []byte) int {
	for start := max(0, len(data)-masker.maximumLength+1); start < len(data); start++ {
		tail := data[start:]
		for _, secret := range masker.allSecret {
			if len(secret) > len(tail) && bytes.HasPrefix(secret, tail) {
				return start
			}
		}
	}
	return len(data)
}

// findAllRun finds every occurrence of every secret and merges the ones that overlap into one run.
func (masker *secretMasker) findAllRun(data []byte) []byteRange {
	var allMatch []byteRange
	for _, secret := range masker.allSecret {
		for start := 0; start <= len(data)-len(secret); {
			index := bytes.Index(data[start:], secret)
			if index < 0 {
				break
			}
			allMatch = append(allMatch, byteRange{start: start + index, end: start + index + len(secret)})
			start += index + 1
		}
	}
	slices.SortFunc(allMatch, func(first, second byteRange) int { return first.start - second.start })
	var allRun []byteRange
	for _, match := range allMatch {
		if last := len(allRun) - 1; last >= 0 && match.start < allRun[last].end {
			allRun[last].end = max(allRun[last].end, match.end)
			continue
		}
		allRun = append(allRun, match)
	}
	return allRun
}

// copyMasked copies source to the masker until source ends or stop closes; a held tail is written as it is
// once nothing completes it within flushDelay.
func copyMasked(masker *secretMasker, source io.Reader, flushDelay time.Duration, stop <-chan struct{}) error {
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
