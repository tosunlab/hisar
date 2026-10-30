//go:build darwin && !ios && cgo

package axfill

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -mmacosx-version-min=12.0
#cgo LDFLAGS: -framework AppKit -framework ApplicationServices -framework Foundation
#include <stdlib.h>
#include <string.h>
#include "axfill_darwin.h"
*/
import "C"

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/tryhisar/hisar/core/agentharness"
	"github.com/tryhisar/hisar/core/signin"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

// readyTimeout covers the first AXEnhancedUserInterface of a Chromium process, which builds its web tree in about 2 s.
const readyTimeout = 3000

// operationTimeout limits every call into the browser however long its context lasts: after it, the call sends no
// more messages, and each message asks the accessibility API to wait at most what is left. The API keeps that timeout
// itself, so a message to a browser that stops answering can still outlast it.
const operationTimeout = 5 * time.Second

// beginLimit limits one native call by ctx and operationTimeout. The call itself returns only once it sends nothing
// more, so end, which must follow it, frees the cancellation flag no message still reads.
func beginLimit(ctx context.Context) (limit C.AXFillLimit, end func()) {
	bound := operationTimeout
	if deadline, hasDeadline := ctx.Deadline(); hasDeadline {
		bound = min(bound, time.Until(deadline))
	}
	cancelled := (*C.int32_t)(C.calloc(1, C.size_t(unsafe.Sizeof(C.int32_t(0)))))
	stopped := make(chan struct{})
	stopAfter := context.AfterFunc(ctx, func() {
		atomic.StoreInt32((*int32)(unsafe.Pointer(cancelled)), 1)
		close(stopped)
	})
	limit = C.AXFillLimit{milliseconds: C.int(max(bound.Milliseconds(), 0)), cancelled: cancelled}
	return limit, func() {
		if !stopAfter() {
			<-stopped
		}
		C.free(unsafe.Pointer(cancelled))
	}
}

func convertUnreadable(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return signin.ErrUnreadable
}

type Driver struct {
	readProcess func(pid int) (agentharness.Process, bool)
}

// New returns the driver; resolver reads the browser process's start time, arguments and code signature.
func New(resolver *appinfo.Resolver) signin.Driver {
	return &Driver{readProcess: func(pid int) (agentharness.Process, bool) {
		chain := resolver.ReadChain(pid)
		if len(chain.Processes) == 0 || chain.Processes[0].PID != pid {
			return agentharness.Process{}, false
		}
		return chain.Processes[0], true
	}}
}

func (*Driver) HasAccess() bool {
	return C.axfillHasAccess() != 0
}

func (*Driver) System() signin.System { return signin.SystemMac }

func (*Driver) RequestAccess() {
	C.axfillRequestAccess()
}

type observedFrame struct {
	Parent   int    `json:"parent"`
	Document uint64 `json:"document"`
	URL      string `json:"url"`
	Origin   string `json:"origin"`
}

type observedField struct {
	ID          uint64 `json:"id"`
	Frame       int    `json:"frame"`
	IsSecure    bool   `json:"isSecure"`
	InputType   string `json:"inputType"`
	Label       string `json:"label"`
	Identifier  string `json:"identifier"`
	Placeholder string `json:"placeholder"`
	IsEditable  bool   `json:"isEditable"`
	IsVisible   bool   `json:"isVisible"`
	Length      int    `json:"length"`
}

type observedButton struct {
	ID        uint64 `json:"id"`
	Frame     int    `json:"frame"`
	Title     string `json:"title"`
	IsVisible bool   `json:"isVisible"`
}

type observedWindow struct {
	Window  uint64           `json:"window"`
	Frames  []observedFrame  `json:"frames"`
	Fields  []observedField  `json:"fields"`
	Buttons []observedButton `json:"buttons"`
}

func (driver *Driver) Observe(ctx context.Context, target signin.Target) (signin.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if target.PID <= 0 || target.WindowID == 0 {
		return nil, signin.ErrWindowNotFound
	}
	process, isFound := driver.readProcess(target.PID)
	if !isFound {
		return nil, convertWindowState(C.axfillReadWindowState(C.int(target.PID), C.uint32_t(target.WindowID)))
	}
	limit, end := beginLimit(ctx)
	var status C.int
	raw := C.axfillObserve(C.int(target.PID), C.uint32_t(target.WindowID), readyTimeout, limit, &status)
	end()
	if status == C.axfillUnreadable {
		return nil, convertUnreadable(ctx)
	}
	if status != C.axfillOK {
		return nil, convertWindowState(status)
	}
	defer C.free(unsafe.Pointer(raw))
	if !isSameInstance(process.PID, process.StartTime) {
		return nil, signin.ErrWindowNotFound
	}
	var window observedWindow
	if err := json.Unmarshal([]byte(C.GoString(raw)), &window); err != nil {
		return nil, err
	}
	return newSnapshot(ctx, target, process, window), nil
}

// convertWindowState is what a window the driver could not read says: only a window the window server no longer
// lists has closed.
func convertWindowState(status C.int) error {
	switch status {
	case C.axfillNoAccess:
		return signin.ErrNoAccess
	case C.axfillNotVisible:
		return signin.ErrNotVisible
	case C.axfillWindowClosed:
		return signin.ErrWindowClosed
	default:
		return signin.ErrWindowNotFound
	}
}

func isSameInstance(pid int, startTime int64) bool {
	instance, err := agentharness.ReadInstance(pid)
	return err == nil && !instance.IsExited && instance.StartTime == startTime
}

type element struct {
	// origins are the origin keys of the element's frames, nearest first, as the Objective-C side compares them.
	origins  string
	isSecure bool
}

type snapshot struct {
	ctx      context.Context
	target   signin.Target
	page     signin.Page
	window   uint64
	elements map[signin.ElementID]element
}

func newSnapshot(ctx context.Context, target signin.Target, process agentharness.Process, window observedWindow) *snapshot {
	page := signin.Page{Process: process}
	frameOrigins := make([]string, len(window.Frames))
	for index, frame := range window.Frames {
		page.Frames = append(page.Frames, signin.Frame{Parent: frame.Parent, Document: signin.DocumentID(frame.Document), URL: frame.URL})
		frameOrigins[index] = frame.Origin
		if frame.Parent >= 0 {
			frameOrigins[index] += "\n" + frameOrigins[frame.Parent]
		}
	}
	elements := map[signin.ElementID]element{}
	for _, field := range window.Fields {
		id := signin.ElementID(field.ID)
		page.Fields = append(page.Fields, signin.Field{
			ID: id, Frame: field.Frame, IsSecure: field.IsSecure, InputType: field.InputType, Label: field.Label,
			Identifier: field.Identifier, Placeholder: field.Placeholder, IsEditable: field.IsEditable,
			IsVisible: field.IsVisible, Length: field.Length,
		})
		elements[id] = element{origins: frameOrigins[field.Frame], isSecure: field.IsSecure}
	}
	for _, button := range window.Buttons {
		id := signin.ElementID(button.ID)
		page.Buttons = append(page.Buttons, signin.Button{ID: id, Frame: button.Frame, Title: button.Title, IsVisible: button.IsVisible})
		elements[id] = element{origins: frameOrigins[button.Frame]}
	}
	return &snapshot{ctx: ctx, target: target, page: page, window: window.Window, elements: elements}
}

func (snapshot *snapshot) Page() signin.Page {
	return snapshot.page
}

func (snapshot *snapshot) act(id signin.ElementID, action func(C.AXFillGuard) C.int) error {
	element, isKnown := snapshot.elements[id]
	if !isKnown || !isSameInstance(snapshot.page.Process.PID, snapshot.page.Process.StartTime) {
		return signin.ErrChanged
	}
	origins := C.CString(element.origins)
	defer C.free(unsafe.Pointer(origins))
	isSecure := C.int(0)
	if element.isSecure {
		isSecure = 1
	}
	limit, end := beginLimit(snapshot.ctx)
	defer end()
	guard := C.AXFillGuard{
		pid: C.int(snapshot.target.PID), windowID: C.uint32_t(snapshot.target.WindowID), window: C.uint64_t(snapshot.window),
		element: C.uint64_t(id), origins: origins, isSecure: isSecure, limit: limit,
	}
	return convertStatus(snapshot.ctx, action(guard))
}

func convertStatus(ctx context.Context, status C.int) error {
	switch status {
	case C.axfillOK:
		return nil
	case C.axfillChanged:
		return signin.ErrChanged
	case C.axfillUnreadable:
		return convertUnreadable(ctx)
	default:
		return errRefused
	}
}

func (snapshot *snapshot) Enter(field signin.ElementID, value signin.Secret) error {
	return snapshot.act(field, func(guard C.AXFillGuard) C.int {
		text := value.Reveal()
		buffer := C.CString(text)
		defer func() {
			C.memset(unsafe.Pointer(buffer), 0, C.size_t(len(text)))
			C.free(unsafe.Pointer(buffer))
		}()
		return C.axfillSetValue(guard, buffer)
	})
}

func (snapshot *snapshot) Clear(field signin.ElementID) error {
	return snapshot.act(field, func(guard C.AXFillGuard) C.int {
		empty := C.CString("")
		defer C.free(unsafe.Pointer(empty))
		return C.axfillSetValue(guard, empty)
	})
}

func (snapshot *snapshot) Press(button signin.ElementID) error {
	return snapshot.act(button, func(guard C.AXFillGuard) C.int { return C.axfillPress(guard) })
}

func (snapshot *snapshot) PressReturn(field signin.ElementID) error {
	return snapshot.act(field, func(guard C.AXFillGuard) C.int { return C.axfillPressReturn(guard) })
}

func (snapshot *snapshot) ReadBack(field signin.ElementID) (int, error) {
	var count C.int
	err := snapshot.act(field, func(guard C.AXFillGuard) C.int { return C.axfillCountCharacters(guard, &count) })
	return int(count), err
}

type observedPrompt struct {
	ID               uint64          `json:"id"`
	Title            string          `json:"title"`
	ButtonTitles     []string        `json:"buttonTitles"`
	SecureFieldCount int             `json:"secureFieldCount"`
	Frame            rect            `json:"frame"`
	Controls         []promptControl `json:"controls"`
}

func (*Driver) ListPrompts(ctx context.Context, process signin.ProcessInstance) ([]signin.Prompt, error) {
	observed, err := listPrompts(ctx, process)
	if err != nil {
		return nil, err
	}
	prompts := []signin.Prompt{}
	for _, prompt := range observed {
		_, hasClose := findCloseControl(prompt.Frame, prompt.Controls)
		prompts = append(prompts, signin.Prompt{
			ID: signin.ElementID(prompt.ID), Title: prompt.Title, ButtonTitles: prompt.ButtonTitles,
			SecureFieldCount: prompt.SecureFieldCount, HasClose: hasClose,
		})
	}
	return prompts, nil
}

func listPrompts(ctx context.Context, process signin.ProcessInstance) ([]observedPrompt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !isSameInstance(process.PID, process.StartTime) {
		return nil, signin.ErrChanged
	}
	limit, end := beginLimit(ctx)
	var status C.int
	raw := C.axfillListPrompts(C.int(process.PID), limit, &status)
	end()
	switch {
	case status == C.axfillUnreadable:
		return nil, convertUnreadable(ctx)
	case status != C.axfillOK:
		return nil, signin.ErrNoAccess
	}
	defer C.free(unsafe.Pointer(raw))
	var prompts []observedPrompt
	if err := json.Unmarshal([]byte(C.GoString(raw)), &prompts); err != nil {
		return nil, err
	}
	if !isSameInstance(process.PID, process.StartTime) {
		return nil, signin.ErrChanged
	}
	return prompts, nil
}

// DismissPrompt reads the prompt again, finds its close control, and has the Objective-C side press it only if it is
// still that control.
func (*Driver) DismissPrompt(ctx context.Context, process signin.ProcessInstance, prompt signin.ElementID) error {
	prompts, err := listPrompts(ctx, process)
	if err != nil {
		return err
	}
	index := slices.IndexFunc(prompts, func(observed observedPrompt) bool { return signin.ElementID(observed.ID) == prompt })
	if index < 0 {
		return signin.ErrChanged
	}
	control, isFound := findCloseControl(prompts[index].Frame, prompts[index].Controls)
	if !isFound {
		return signin.ErrNoCloseControl
	}
	classes := C.CString(strings.Join(control.Classes, "\n"))
	defer C.free(unsafe.Pointer(classes))
	ancestorClasses := C.CString(strings.Join(control.AncestorClasses, "\n"))
	defer C.free(unsafe.Pointer(ancestorClasses))
	if !isSameInstance(process.PID, process.StartTime) {
		return signin.ErrChanged
	}
	limit, end := beginLimit(ctx)
	defer end()
	return convertStatus(ctx, C.axfillPressPromptControl(C.int(process.PID), C.uint64_t(prompt), C.uint64_t(control.ID), classes, ancestorClasses, limit))
}

func (*snapshot) Close() {}

func listWindowIDs(pid int) ([]uint32, error) {
	raw := C.axfillListWindows(C.int(pid))
	if raw == nil {
		return nil, errRefused
	}
	defer C.free(unsafe.Pointer(raw))
	var windows []struct {
		ID         uint32 `json:"id"`
		IsOnScreen bool   `json:"isOnScreen"`
	}
	if err := json.Unmarshal([]byte(C.GoString(raw)), &windows); err != nil {
		return nil, err
	}
	ids := []uint32{}
	for _, window := range windows {
		if window.IsOnScreen {
			ids = append(ids, window.ID)
		}
	}
	return ids, nil
}
