package signin

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/tryhisar/hisar/core/agentharness"
)

const (
	fixtureMain  = "http://127.0.0.1:47811"
	fixtureOther = "http://localhost:47812"
)

// recordedBrowsers are the folders of testdata/pages: the Mac's browsers by ID, and Linux's as linux-<ID>.
var recordedBrowsers = []string{"brave", "chrome", "edge", "linux-chromium"}

var testTiming = timing{
	poll: time.Millisecond, await: 150 * time.Millisecond, settle: 50 * time.Millisecond,
	promptWatch: 20 * time.Millisecond, promptGone: 20 * time.Millisecond, cleanup: time.Second,
}

func readRecordedPage(t *testing.T, browser, name string) Page {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join("testdata", "pages", browser, name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var page Page
	if err := json.Unmarshal(contents, &page); err != nil {
		t.Fatal(err)
	}
	return page
}

type fakeBrowser struct {
	t             *testing.T
	name          string
	mutex         sync.Mutex
	process       agentharness.Process
	page          Page
	values        map[ElementID]string
	prompts       []Prompt
	actions       []string
	nextID        uint64
	submits       []map[string]string
	onSubmit      func(browser *fakeBrowser)
	observeErrors []error
	// isHung makes Observe wait as a browser that stopped answering does, until the driver's bound or the context ends.
	isHung            bool
	onEnter           func(identifier string)
	enterError        error
	failClear         string
	readBack          func(field Field, value string) int
	isPromptPermanent bool
	onListPrompts     func() error
	// isLinux is a browser recorded on Linux, which the Linux platform signs in with.
	isLinux bool
}

const hungObserveBound = 400 * time.Millisecond

func newFakeBrowser(t *testing.T, browser string) *fakeBrowser {
	if id, isLinux := strings.CutPrefix(browser, "linux-"); isLinux {
		path := linuxExecutables[id][0]
		return &fakeBrowser{
			t: t, name: browser, values: map[ElementID]string{}, nextID: 1000, isLinux: true,
			process: agentharness.Process{PID: 4242, StartTime: 1_700_000_000, Path: path, Arguments: []string{path + " --force-renderer-accessibility"}},
		}
	}
	signatures := map[string]agentharness.Signature{}
	for _, row := range macBrowsers {
		signatures[row.ID] = row.Signature
	}
	return &fakeBrowser{
		t: t, name: browser, values: map[ElementID]string{}, nextID: 1000,
		process: agentharness.Process{
			PID: 4242, StartTime: 1_700_000_000, Path: "/Applications/Browser.app/Contents/MacOS/Browser",
			Arguments: []string{"/Applications/Browser.app/Contents/MacOS/Browser"}, Signature: signatures[browser],
		},
	}
}

func (browser *fakeBrowser) show(name, origin string) {
	browser.t.Helper()
	page := readRecordedPage(browser.t, browser.name, name)
	renumbered := map[uint64]uint64{}
	renumber := func(id uint64) uint64 {
		if _, isKnown := renumbered[id]; !isKnown {
			browser.nextID++
			renumbered[id] = browser.nextID
		}
		return renumbered[id]
	}
	for index := range page.Frames {
		page.Frames[index].Document = DocumentID(renumber(uint64(page.Frames[index].Document)))
		page.Frames[index].URL = replaceOrigin(page.Frames[index].URL, origin)
	}
	for index := range page.Fields {
		page.Fields[index].ID = ElementID(renumber(uint64(page.Fields[index].ID)))
	}
	for index := range page.Buttons {
		page.Buttons[index].ID = ElementID(renumber(uint64(page.Buttons[index].ID)))
	}
	browser.page = page
}

func replaceOrigin(rawURL, origin string) string {
	if origin == "" || len(rawURL) < len(fixtureMain) || rawURL[:len(fixtureMain)] != fixtureMain {
		return rawURL
	}
	return origin + rawURL[len(fixtureMain):]
}

func (browser *fakeBrowser) lock() func() {
	browser.mutex.Lock()
	return browser.mutex.Unlock
}

func (browser *fakeBrowser) log() []string {
	defer browser.lock()()
	return slices.Clone(browser.actions)
}

func (browser *fakeBrowser) findField(id ElementID) (Field, bool) {
	index := slices.IndexFunc(browser.page.Fields, func(field Field) bool { return field.ID == id })
	if index < 0 {
		return Field{}, false
	}
	return browser.page.Fields[index], true
}

func (browser *fakeBrowser) fieldValue(identifier string) string {
	defer browser.lock()()
	for _, field := range browser.page.Fields {
		if field.Identifier == identifier {
			return browser.values[field.ID]
		}
	}
	return ""
}

func (browser *fakeBrowser) submit() {
	posted := map[string]string{}
	for _, field := range browser.page.Fields {
		if value := browser.values[field.ID]; value != "" {
			posted[field.Identifier] = value
		}
	}
	browser.submits = append(browser.submits, posted)
	if browser.onSubmit != nil {
		browser.onSubmit(browser)
	}
}

type fakeDriver struct {
	browser *fakeBrowser
}

func (fakeDriver) HasAccess() bool { return true }

func (fakeDriver) RequestAccess() {}

func (driver fakeDriver) System() System {
	if driver.browser.isLinux {
		return SystemLinux
	}
	return SystemMac
}

func (driver fakeDriver) Observe(ctx context.Context, target Target) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	browser := driver.browser
	browser.mutex.Lock()
	isHung := browser.isHung
	browser.mutex.Unlock()
	if isHung {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(hungObserveBound):
			return nil, ErrUnreadable
		}
	}
	defer browser.lock()()
	if len(browser.observeErrors) > 0 {
		err := browser.observeErrors[0]
		browser.observeErrors = browser.observeErrors[1:]
		return nil, err
	}
	if target.PID != browser.process.PID || target.WindowID != 7 {
		return nil, ErrWindowNotFound
	}
	page := browser.page
	page.Process = browser.process
	page.Fields = slices.Clone(page.Fields)
	for index, field := range page.Fields {
		page.Fields[index].Length = utf8.RuneCountInString(browser.values[field.ID])
	}
	urls := map[ElementID]string{}
	for _, field := range page.Fields {
		urls[field.ID] = page.Frames[field.Frame].URL
	}
	for _, button := range page.Buttons {
		urls[button.ID] = page.Frames[button.Frame].URL
	}
	return &fakeSnapshot{browser: browser, page: page, urls: urls}, nil
}

type fakeSnapshot struct {
	browser *fakeBrowser
	page    Page
	urls    map[ElementID]string
}

func (snapshot *fakeSnapshot) Page() Page { return snapshot.page }

func (snapshot *fakeSnapshot) isCurrent(id ElementID) bool {
	browser := snapshot.browser
	if browser.process.StartTime != snapshot.page.Process.StartTime {
		return false
	}
	for _, field := range browser.page.Fields {
		if field.ID == id {
			return browser.page.Frames[field.Frame].URL == snapshot.urls[id]
		}
	}
	for _, button := range browser.page.Buttons {
		if button.ID == id {
			return browser.page.Frames[button.Frame].URL == snapshot.urls[id]
		}
	}
	return false
}

func (snapshot *fakeSnapshot) Enter(id ElementID, value Secret) error {
	browser := snapshot.browser
	browser.mutex.Lock()
	if !snapshot.isCurrent(id) {
		browser.mutex.Unlock()
		return ErrChanged
	}
	field, _ := browser.findField(id)
	if browser.enterError != nil {
		browser.mutex.Unlock()
		return browser.enterError
	}
	browser.values[id] = value.Reveal()
	browser.actions = append(browser.actions, "enter "+field.Identifier)
	onEnter := browser.onEnter
	browser.mutex.Unlock()
	if onEnter != nil {
		onEnter(field.Identifier)
	}
	return nil
}

func (snapshot *fakeSnapshot) Clear(id ElementID) error {
	browser := snapshot.browser
	defer browser.lock()()
	if !snapshot.isCurrent(id) {
		return ErrChanged
	}
	field, _ := browser.findField(id)
	if field.Identifier == browser.failClear {
		return errors.New("refused")
	}
	browser.values[id] = ""
	browser.actions = append(browser.actions, "clear "+field.Identifier)
	return nil
}

func (snapshot *fakeSnapshot) Press(id ElementID) error {
	browser := snapshot.browser
	defer browser.lock()()
	if !snapshot.isCurrent(id) {
		return ErrChanged
	}
	index := slices.IndexFunc(browser.page.Buttons, func(button Button) bool { return button.ID == id })
	browser.actions = append(browser.actions, "press "+browser.page.Buttons[index].Title)
	browser.submit()
	return nil
}

func (snapshot *fakeSnapshot) PressReturn(id ElementID) error {
	browser := snapshot.browser
	defer browser.lock()()
	if !snapshot.isCurrent(id) {
		return ErrChanged
	}
	field, _ := browser.findField(id)
	browser.actions = append(browser.actions, "return "+field.Identifier)
	browser.submit()
	return nil
}

func (snapshot *fakeSnapshot) ReadBack(id ElementID) (int, error) {
	browser := snapshot.browser
	defer browser.lock()()
	if !snapshot.isCurrent(id) {
		return 0, ErrChanged
	}
	field, _ := browser.findField(id)
	if browser.readBack != nil {
		return browser.readBack(field, browser.values[id]), nil
	}
	return utf8.RuneCountInString(browser.values[id]), nil
}

func (snapshot *fakeSnapshot) Close() {}

func (driver fakeDriver) ListPrompts(ctx context.Context, process ProcessInstance) ([]Prompt, error) {
	browser := driver.browser
	browser.mutex.Lock()
	onListPrompts := browser.onListPrompts
	browser.mutex.Unlock()
	if onListPrompts != nil {
		if err := onListPrompts(); err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	defer browser.lock()()
	if browser.process.StartTime != process.StartTime {
		return nil, ErrChanged
	}
	return slices.Clone(browser.prompts), nil
}

func (driver fakeDriver) DismissPrompt(ctx context.Context, process ProcessInstance, id ElementID) error {
	browser := driver.browser
	defer browser.lock()()
	index := slices.IndexFunc(browser.prompts, func(prompt Prompt) bool { return prompt.ID == id })
	switch {
	case ctx.Err() != nil:
		return ctx.Err()
	case browser.process.StartTime != process.StartTime || index < 0:
		return ErrChanged
	case !browser.prompts[index].HasClose:
		return ErrNoCloseControl
	}
	browser.actions = append(browser.actions, "dismiss prompt")
	if !browser.isPromptPermanent {
		browser.prompts = slices.Delete(browser.prompts, index, index+1)
	}
	return nil
}

// savePasswordPrompt is the prompt Brave showed after the fixture's plain form was submitted.
var savePasswordPrompt = Prompt{ID: 9000, Title: "Save password?", ButtonTitles: []string{"Close", "Never", "No Thanks", "Save"}, SecureFieldCount: 1, HasClose: true}

func newTestSigner(browser *fakeBrowser, now time.Time) *Signer {
	signer := NewSigner(fakeDriver{browser: browser}, browser.t.TempDir(), func() time.Time { return now })
	// Each recorded browser signs in on its own platform, whatever system runs the tests.
	signer.platform, signer.timing = macPlatform, testTiming
	if browser.isLinux {
		signer.platform = testLinuxPlatform(browser.process.Path)
	}
	return signer
}

var testTarget = Target{PID: 4242, WindowID: 7}

func cloneSubmits(browser *fakeBrowser) []map[string]string {
	defer browser.lock()()
	submits := make([]map[string]string, len(browser.submits))
	for index, posted := range browser.submits {
		submits[index] = maps.Clone(posted)
	}
	return submits
}
