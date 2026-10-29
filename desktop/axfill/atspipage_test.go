package axfill

import (
	"context"
	"errors"
	"testing"

	"github.com/tryhisar/hisar/core/signin"
)

// fakeTree is an accessibility tree in memory: each node with its parent, in document order.
type fakeTree struct {
	nodes   map[nodeRef]node
	parents map[nodeRef]nodeRef
	order   []nodeRef
	urls    map[nodeRef]string
	counts  map[nodeRef]int
}

func newFakeTree() *fakeTree {
	return &fakeTree{nodes: map[nodeRef]node{}, parents: map[nodeRef]nodeRef{}, urls: map[nodeRef]string{}, counts: map[nodeRef]int{}}
}

var shown = uint64(1)<<stateShowing | 1<<stateVisible

func (tree *fakeTree) add(path string, parent nodeRef, role uint32, name string, states uint64, attributes map[string]string) nodeRef {
	ref := nodeRef{bus: ":1.1", path: path}
	tree.nodes[ref] = node{ref: ref, role: role, name: name, states: states, attributes: attributes, extents: rect{X: 10, Y: 10, Width: 200, Height: 30}}
	tree.parents[ref] = parent
	tree.order = append(tree.order, ref)
	return ref
}

// remove takes an accessible and everything below it out of the tree, as a browser does with a page it left.
func (tree *fakeTree) remove(ref nodeRef) {
	for _, candidate := range tree.order {
		if tree.isBelow(candidate, ref) {
			delete(tree.nodes, candidate)
		}
	}
	delete(tree.nodes, ref)
	kept := tree.order[:0]
	for _, candidate := range tree.order {
		if _, isFound := tree.nodes[candidate]; isFound {
			kept = append(kept, candidate)
		}
	}
	tree.order = kept
}

func (tree *fakeTree) read(_ context.Context, ref nodeRef) (node, error) {
	found, isFound := tree.nodes[ref]
	if !isFound {
		return node{}, errUnknownNode
	}
	return found, nil
}

func (tree *fakeTree) isBelow(ref, ancestor nodeRef) bool {
	for current := tree.parents[ref]; current != (nodeRef{}); current = tree.parents[current] {
		if current == ancestor {
			return true
		}
	}
	return false
}

func (tree *fakeTree) find(_ context.Context, ref nodeRef, roles ...uint32) ([]nodeRef, error) {
	found := []nodeRef{}
	for _, candidate := range tree.order {
		for _, role := range roles {
			if tree.nodes[candidate].role == role && tree.isBelow(candidate, ref) {
				found = append(found, candidate)
			}
		}
	}
	return found, nil
}

func (tree *fakeTree) parent(_ context.Context, ref nodeRef) (nodeRef, error) {
	if _, isFound := tree.nodes[ref]; !isFound {
		return nodeRef{}, errUnknownNode
	}
	return tree.parents[ref], nil
}

func (tree *fakeTree) documentURL(_ context.Context, document nodeRef) (string, error) {
	if _, isFound := tree.nodes[document]; !isFound {
		return "", errUnknownNode
	}
	return tree.urls[document], nil
}

func (tree *fakeTree) characterCount(_ context.Context, ref nodeRef) (int, error) {
	return tree.counts[ref], nil
}

const editable = uint64(1)<<stateEditable | 1<<stateEnabled | 1<<stateSensitive

// signInWindow is a browser window with a sign-in page that holds a cross-origin frame with a password field.
func signInWindow() (*fakeTree, nodeRef, map[string]nodeRef) {
	tree := newFakeTree()
	refs := map[string]nodeRef{}
	window := tree.add("/frame", nodeRef{}, roleFrame, "Sign in - Chromium", shown, map[string]string{"class": browserWindowClass})
	toolbar := tree.add("/toolbar", window, 63, "", shown, nil)
	tree.add("/omnibox", toolbar, roleEntry, "Address and search bar", shown|editable, map[string]string{"class": "OmniboxViewViews"})
	refs["top"] = tree.add("/document", window, roleDocumentWeb, "Sign in", shown, nil)
	tree.urls[refs["top"]] = "https://example.com/login"
	form := tree.add("/form", refs["top"], 87, "", shown, nil)
	refs["username"] = tree.add("/username", form, roleEntry, "Email", shown|editable, map[string]string{"tag": "input", "id": "email", "text-input-type": "email", "placeholder": "you@example.com"})
	tree.counts[refs["username"]] = 3
	refs["frame"] = tree.add("/iframe", form, roleDocumentWeb, "", shown, nil)
	tree.urls[refs["frame"]] = "https://login.example.net/frame"
	refs["password"] = tree.add("/password", refs["frame"], rolePasswordText, "Password", shown|editable, map[string]string{"tag": "input", "text-input-type": "password"})
	refs["button"] = tree.add("/button", form, rolePushButton, "Sign in", shown, map[string]string{"tag": "button"})
	refs["background"] = tree.add("/background", window, roleDocumentWeb, "Other tab", 0, nil)
	tree.urls[refs["background"]] = "https://other.example.org/"
	tree.add("/background-field", refs["background"], rolePasswordText, "Password", editable, map[string]string{"tag": "input"})
	return tree, window, refs
}

// readPage reads the showing tab's page: its frames with their parents, and only the web page's fields and buttons,
// each in the frame that holds it; the browser's own controls and a background tab are not the page.
func TestReadPageReadsTheShowingPageAndItsFrames(t *testing.T) {
	tree, window, refs := signInWindow()
	reading, err := readPage(context.Background(), tree, window, &idTable{})
	if err != nil {
		t.Fatal(err)
	}
	if len(reading.frames) != 2 || reading.frames[0].URL != "https://example.com/login" || reading.frames[0].Parent != -1 ||
		reading.frames[1].URL != "https://login.example.net/frame" || reading.frames[1].Parent != 0 {
		t.Fatalf("frames %+v", reading.frames)
	}
	if len(reading.fields) != 2 || len(reading.buttons) != 1 {
		t.Fatalf("fields %+v, buttons %+v", reading.fields, reading.buttons)
	}
	username, password := reading.fields[0], reading.fields[1]
	if username.Frame != 0 || username.IsSecure || username.InputType != "email" || username.Label != "Email" || username.Identifier != "email" ||
		username.Placeholder != "you@example.com" || !username.IsEditable || !username.IsVisible || username.Length != 3 {
		t.Errorf("username %+v", username)
	}
	if password.Frame != 1 || !password.IsSecure || !password.IsEditable {
		t.Errorf("password %+v", password)
	}
	if element := reading.elements[password.ID]; element.ref != refs["password"] || !element.isSecure ||
		len(element.origins) != 2 || element.origins[0] != "https://login.example.net" || element.origins[1] != "https://example.com" {
		t.Errorf("password element %+v", element)
	}
	if reading.buttons[0].Title != "Sign in" || reading.buttons[0].Frame != 0 {
		t.Errorf("button %+v", reading.buttons[0])
	}
}

// An accessible keeps its ID across readings, so core/signin sees the same field, and a new document gets a new one.
func TestReadPageKeepsIDsAcrossReadings(t *testing.T) {
	tree, window, _ := signInWindow()
	ids := &idTable{}
	first, _ := readPage(context.Background(), tree, window, ids)
	second, _ := readPage(context.Background(), tree, window, ids)
	if first.fields[0].ID != second.fields[0].ID || first.frames[0].Document != second.frames[0].Document {
		t.Errorf("IDs changed: %+v, %+v", first, second)
	}
	replaced := tree.add("/document-2", window, roleDocumentWeb, "Signed in", shown, nil)
	tree.urls[replaced] = "https://example.com/home"
	tree.remove(nodeRef{bus: ":1.1", path: "/document"})
	third, err := readPage(context.Background(), tree, window, ids)
	if err != nil || len(third.frames) != 1 || third.frames[0].Document == first.frames[0].Document {
		t.Errorf("after navigating: %+v, %v", third, err)
	}
}

// Two showing pages, as a docked DevTools beside the page shows, leave Hisar unable to tell which one the user sees.
func TestReadPageRefusesTwoShowingPages(t *testing.T) {
	tree, window, _ := signInWindow()
	devTools := tree.add("/devtools", window, roleDocumentWeb, "DevTools", shown, nil)
	tree.urls[devTools] = "devtools://devtools/bundled/devtools_app.html"
	if _, err := readPage(context.Background(), tree, window, &idTable{}); !errors.Is(err, signin.ErrUnreadable) {
		t.Errorf("%v, want ErrUnreadable", err)
	}
}

// A field the page hides, scrolls out of the page or makes tiny reads as not visible.
func TestReadPageMarksHiddenFields(t *testing.T) {
	tree, window, refs := signInWindow()
	hidden := tree.nodes[refs["username"]]
	hidden.states &^= 1 << stateShowing
	tree.nodes[refs["username"]] = hidden
	tiny := tree.nodes[refs["password"]]
	tiny.extents = rect{X: 10, Y: 10, Width: 1, Height: 1}
	tree.nodes[refs["password"]] = tiny
	reading, err := readPage(context.Background(), tree, window, &idTable{})
	if err != nil || reading.fields[0].IsVisible || reading.fields[1].IsVisible {
		t.Errorf("%+v, %v", reading.fields, err)
	}
}

// Before every action the element must still be in the documents it was read in, at their origins, and a password
// field still one.
func TestCheckElementCatchesAChangedFrameOrField(t *testing.T) {
	tree, window, refs := signInWindow()
	reading, _ := readPage(context.Background(), tree, window, &idTable{})
	password := reading.elements[reading.fields[1].ID]
	if _, err := checkElement(context.Background(), tree, password); err != nil {
		t.Fatalf("unchanged: %v", err)
	}
	tree.urls[refs["frame"]] = "https://attacker.example/frame"
	if _, err := checkElement(context.Background(), tree, password); !errors.Is(err, signin.ErrChanged) {
		t.Errorf("frame navigated: %v", err)
	}
	tree.urls[refs["frame"]] = "https://login.example.net/other"
	changed := tree.nodes[refs["password"]]
	changed.role = roleEntry
	tree.nodes[refs["password"]] = changed
	if _, err := checkElement(context.Background(), tree, password); !errors.Is(err, signin.ErrChanged) {
		t.Errorf("no longer a password field: %v", err)
	}
	delete(tree.nodes, refs["password"])
	if _, err := checkElement(context.Background(), tree, password); !errors.Is(err, signin.ErrChanged) {
		t.Errorf("field gone: %v", err)
	}
}
