package axfill

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/signin"
)

// The AT-SPI roles and states the Linux driver reads (atspi-constants.h).
const (
	roleFrame        = 23
	rolePasswordText = 40
	rolePushButton   = 43
	roleEntry        = 79
	roleDocumentWeb  = 95

	stateActive    = 1
	stateEditable  = 7
	stateEnabled   = 8
	stateFocused   = 12
	stateIconified = 15
	stateSensitive = 24
	stateShowing   = 25
	stateVisible   = 30

	// browserWindowClass is the Views class of a Chromium browser window's root.
	browserWindowClass = "BrowserRootView"

	// minimumVisibleSize is the smallest field or button the page shows, as the Mac driver counts it.
	minimumVisibleSize = 8
	// maximumControls bounds a page: a page with more is unreadable, as one the Mac driver cannot read in full.
	maximumControls = 2000
	// maximumDocuments bounds the frames of a page.
	maximumDocuments = 64
)

var errUnknownNode = errors.New("axfill: the accessible is gone")

// nodeRef names an accessible on the accessibility bus: the application's bus name and the object's path.
type nodeRef struct {
	bus  string
	path string
}

// node is one accessible as AT-SPI describes it. Attributes are Chromium's object attributes: tag, id, placeholder,
// text-input-type, and the Views class of the browser's own controls.
type node struct {
	ref        nodeRef
	role       uint32
	name       string
	states     uint64
	attributes map[string]string
	extents    rect
}

func (node node) has(state uint) bool { return node.states&(1<<state) != 0 }

// accessibilityTree reads accessibles; every call ends with ctx.
type accessibilityTree interface {
	read(ctx context.Context, ref nodeRef) (node, error)
	// find lists the descendants of ref with one of roles, in document order.
	find(ctx context.Context, ref nodeRef, roles ...uint32) ([]nodeRef, error)
	parent(ctx context.Context, ref nodeRef) (nodeRef, error)
	documentURL(ctx context.Context, document nodeRef) (string, error)
	characterCount(ctx context.Context, ref nodeRef) (int, error)
}

// idTable gives every accessible one ID while the driver keeps seeing it, so a field reads as the same field in every
// snapshot and a new document as a new one. An accessible unseen for forgetIDAfter is forgotten, as the Mac driver
// forgets its elements, and gets a new ID if it shows again.
type idTable struct {
	mutex  sync.Mutex
	byRef  map[nodeRef]seenID
	lastID uint64
}

type seenID struct {
	id   uint64
	seen time.Time
}

const (
	forgetIDAfter = 10 * time.Minute
	// pruneIDsAbove is how many IDs the table holds before it looks for ones to forget.
	pruneIDsAbove = 4096
)

func (table *idTable) idOf(ref nodeRef) uint64 {
	table.mutex.Lock()
	defer table.mutex.Unlock()
	now := time.Now()
	if table.byRef == nil {
		table.byRef = map[nodeRef]seenID{}
	}
	if known, isKnown := table.byRef[ref]; isKnown {
		table.byRef[ref] = seenID{id: known.id, seen: now}
		return known.id
	}
	if len(table.byRef) > pruneIDsAbove {
		for key, known := range table.byRef {
			if now.Sub(known.seen) > forgetIDAfter {
				delete(table.byRef, key)
			}
		}
	}
	table.lastID++
	table.byRef[ref] = seenID{id: table.lastID, seen: now}
	return table.lastID
}

// pageElement is what an action rechecks about an element: the documents from its own up to the top, with the
// origins they had, and whether it is a secure field.
type pageElement struct {
	ref       nodeRef
	documents []nodeRef
	origins   []string
	isSecure  bool
}

type pageReading struct {
	frames   []signin.Frame
	fields   []signin.Field
	buttons  []signin.Button
	elements map[signin.ElementID]pageElement
}

// readPage reads the web page of a browser window: the top document is the one showing document of the window that
// no other document holds, its frames are the documents inside it, and the fields and buttons are the inputs, password
// fields and buttons of those documents. Another showing top document, as a docked DevTools has, makes the page
// unreadable, since Hisar could not tell which one the user sees.
func readPage(ctx context.Context, tree accessibilityTree, window nodeRef, ids *idTable) (pageReading, error) {
	documentRefs, err := tree.find(ctx, window, roleDocumentWeb)
	if err != nil {
		return pageReading{}, err
	}
	if len(documentRefs) > maximumDocuments {
		return pageReading{}, signin.ErrUnreadable
	}
	isDocument := map[nodeRef]bool{}
	for _, ref := range documentRefs {
		isDocument[ref] = true
	}
	parentOf := map[nodeRef]nodeRef{}
	var top []nodeRef
	for _, ref := range documentRefs {
		document, err := tree.read(ctx, ref)
		if err != nil {
			return pageReading{}, err
		}
		if !document.has(stateShowing) {
			continue
		}
		parentDocument, err := findEnclosingDocument(ctx, tree, ref, window, isDocument)
		if err != nil {
			return pageReading{}, err
		}
		if parentDocument == (nodeRef{}) {
			top = append(top, ref)
		} else {
			parentOf[ref] = parentDocument
		}
	}
	webTops := top[:0]
	for _, ref := range top {
		if url, err := tree.documentURL(ctx, ref); err == nil && url != "" {
			webTops = append(webTops, ref)
		}
	}
	if len(webTops) == 0 {
		return pageReading{}, nil
	}
	if len(webTops) > 1 {
		return pageReading{}, signin.ErrUnreadable
	}
	reading := pageReading{elements: map[signin.ElementID]pageElement{}}
	frameIndex := map[nodeRef]int{}
	origins := map[nodeRef]string{}
	var addFrame func(ref nodeRef, parent int) error
	addFrame = func(ref nodeRef, parent int) error {
		url, err := tree.documentURL(ctx, ref)
		if err != nil {
			return err
		}
		frameIndex[ref] = len(reading.frames)
		origins[ref] = describeOrigin(url)
		reading.frames = append(reading.frames, signin.Frame{Parent: parent, Document: signin.DocumentID(ids.idOf(ref)), URL: url})
		for _, child := range documentRefs {
			if parentOf[child] == ref {
				if err := addFrame(child, frameIndex[ref]); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := addFrame(webTops[0], -1); err != nil {
		return pageReading{}, err
	}
	topExtents := rect{}
	if top, err := tree.read(ctx, webTops[0]); err == nil {
		topExtents = top.extents
	}
	controls, err := tree.find(ctx, webTops[0], roleEntry, rolePasswordText, rolePushButton)
	if err != nil {
		return pageReading{}, err
	}
	if len(controls) > maximumControls {
		return pageReading{}, signin.ErrUnreadable
	}
	for _, ref := range controls {
		control, err := tree.read(ctx, ref)
		if errors.Is(err, errUnknownNode) {
			continue
		}
		if err != nil {
			return pageReading{}, err
		}
		tag := control.attributes["tag"]
		if tag == "" {
			continue
		}
		document, err := findEnclosingDocument(ctx, tree, ref, window, isDocument)
		if err != nil {
			return pageReading{}, err
		}
		frame, isFrame := frameIndex[document]
		if !isFrame {
			continue
		}
		chain, chainOrigins := []nodeRef{}, []string{}
		for current := document; current != (nodeRef{}); current = parentOf[current] {
			chain, chainOrigins = append(chain, current), append(chainOrigins, origins[current])
		}
		id := signin.ElementID(ids.idOf(ref))
		isVisible := control.has(stateShowing) && control.has(stateVisible) && control.extents.Width >= minimumVisibleSize &&
			control.extents.Height >= minimumVisibleSize && isInside(control.extents, topExtents)
		if control.role == rolePushButton {
			reading.buttons = append(reading.buttons, signin.Button{ID: id, Frame: frame, Title: control.name, IsVisible: isVisible})
			reading.elements[id] = pageElement{ref: ref, documents: chain, origins: chainOrigins}
			continue
		}
		length, err := tree.characterCount(ctx, ref)
		if err != nil {
			return pageReading{}, err
		}
		isSecure := control.role == rolePasswordText
		reading.fields = append(reading.fields, signin.Field{
			ID: id, Frame: frame, IsSecure: isSecure, InputType: control.attributes["text-input-type"], Label: control.name,
			Identifier: control.attributes["id"], Placeholder: control.attributes["placeholder"],
			IsEditable: control.has(stateEditable) && control.has(stateEnabled) && control.has(stateSensitive),
			IsVisible:  isVisible, Length: length,
		})
		reading.elements[id] = pageElement{ref: ref, documents: chain, origins: chainOrigins, isSecure: isSecure}
	}
	return reading, nil
}

// findEnclosingDocument walks up from ref to the nearest document of isDocument, stopping at window; the zero nodeRef
// when there is none.
func findEnclosingDocument(ctx context.Context, tree accessibilityTree, ref, window nodeRef, isDocument map[nodeRef]bool) (nodeRef, error) {
	current := ref
	for range 256 {
		parent, err := tree.parent(ctx, current)
		if err != nil {
			return nodeRef{}, err
		}
		if parent == (nodeRef{}) || parent == window {
			return nodeRef{}, nil
		}
		if isDocument[parent] {
			return parent, nil
		}
		current = parent
	}
	return nodeRef{}, signin.ErrUnreadable
}

// isInside reports whether inner's midpoint lies in outer, as the Mac driver counts a field visible in the top web area.
// An outer without extents, as a browser that does not report them gives, holds everything.
func isInside(inner, outer rect) bool {
	if outer.Width <= 0 || outer.Height <= 0 {
		return true
	}
	x, y := inner.X+inner.Width/2, inner.Y+inner.Height/2
	return x >= outer.X && x < outer.X+outer.Width && y >= outer.Y && y < outer.Y+outer.Height
}

// describeOrigin is the origin an action compares before it acts: scheme and host of the URL, the whole URL for one
// that is not a web address.
func describeOrigin(url string) string {
	if origin, err := signin.ParseOrigin(url); err == nil {
		return origin.String()
	}
	return strings.TrimSpace(url)
}

// checkElement rereads an element before an action: it must still be there with the same role, a secure field still a
// password field, and every document from its own up to the top at the origin it had.
func checkElement(ctx context.Context, tree accessibilityTree, element pageElement) (node, error) {
	current, err := tree.read(ctx, element.ref)
	if err != nil {
		return node{}, signin.ErrChanged
	}
	if element.isSecure && current.role != rolePasswordText {
		return node{}, signin.ErrChanged
	}
	for index, document := range element.documents {
		url, err := tree.documentURL(ctx, document)
		if err != nil || describeOrigin(url) != element.origins[index] {
			return node{}, signin.ErrChanged
		}
	}
	return current, nil
}
