package axfill

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"
)

const (
	atspiRegistry     = "org.a11y.atspi.Registry"
	atspiRootPath     = "/org/a11y/atspi/accessible/root"
	atspiNullPath     = "/org/a11y/atspi/null"
	atspiAccessible   = "org.a11y.atspi.Accessible"
	atspiAction       = "org.a11y.atspi.Action"
	atspiCollection   = "org.a11y.atspi.Collection"
	atspiComponent    = "org.a11y.atspi.Component"
	atspiDocument     = "org.a11y.atspi.Document"
	atspiText         = "org.a11y.atspi.Text"
	coordinatesScreen = 0
	// matchAny and sortCanonical are AT-SPI's Collection constants: a match of any listed role, in document order.
	matchAny      = 2
	matchAll      = 1
	sortCanonical = 1
)

// atspiBus is the session's accessibility bus, the one AT-SPI applications register on.
type atspiBus struct {
	mutex      sync.Mutex
	connection *dbus.Conn
}

// connect reaches the accessibility bus through the session bus's org.a11y.Bus, once, and again after it closed.
func (bus *atspiBus) connect(ctx context.Context) (*dbus.Conn, error) {
	bus.mutex.Lock()
	defer bus.mutex.Unlock()
	if bus.connection != nil && bus.connection.Connected() {
		return bus.connection, nil
	}
	session, err := dbus.SessionBusPrivate()
	if err != nil {
		return nil, err
	}
	defer session.Close()
	if err := session.Auth(nil); err != nil {
		return nil, err
	}
	if err := session.Hello(); err != nil {
		return nil, err
	}
	var address string
	if err := session.Object("org.a11y.Bus", "/org/a11y/bus").CallWithContext(ctx, "org.a11y.Bus.GetAddress", 0).Store(&address); err != nil {
		return nil, err
	}
	connection, err := dbus.Connect(address)
	if err != nil {
		return nil, err
	}
	bus.connection = connection
	return connection, nil
}

// tree is the accessibility tree on the bus, as accessibilityTree reads it.
type atspiTree struct {
	connection *dbus.Conn
}

type atspiReference struct {
	Bus  string
	Path dbus.ObjectPath
}

func (tree atspiTree) object(ref nodeRef) dbus.BusObject {
	return tree.connection.Object(ref.bus, dbus.ObjectPath(ref.path))
}

// property reads one property within ctx; BusObject.GetProperty has no context, and a browser that stopped answering
// would hold it.
func (tree atspiTree) property(ctx context.Context, ref nodeRef, name string) (dbus.Variant, error) {
	var value dbus.Variant
	dot := strings.LastIndex(name, ".")
	err := tree.object(ref).CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, name[:dot], name[dot+1:]).Store(&value)
	return value, convertCallError(err)
}

// convertCallError is errUnknownNode for an accessible the application no longer has.
func convertCallError(err error) error {
	if err == nil {
		return nil
	}
	var dbusError dbus.Error
	if errors.As(err, &dbusError) && (strings.HasSuffix(dbusError.Name, ".UnknownObject") || strings.HasSuffix(dbusError.Name, ".UnknownMethod") ||
		strings.HasSuffix(dbusError.Name, ".ServiceUnknown")) {
		return errUnknownNode
	}
	return err
}

func (tree atspiTree) read(ctx context.Context, ref nodeRef) (node, error) {
	object := tree.object(ref)
	read := node{ref: ref}
	if err := object.CallWithContext(ctx, atspiAccessible+".GetRole", 0).Store(&read.role); err != nil {
		return node{}, convertCallError(err)
	}
	var states []uint32
	if err := object.CallWithContext(ctx, atspiAccessible+".GetState", 0).Store(&states); err != nil {
		return node{}, convertCallError(err)
	}
	for index, word := range states {
		if index < 2 {
			read.states |= uint64(word) << (32 * index)
		}
	}
	if err := object.CallWithContext(ctx, atspiAccessible+".GetAttributes", 0).Store(&read.attributes); err != nil {
		return node{}, convertCallError(err)
	}
	name, err := tree.property(ctx, ref, atspiAccessible+".Name")
	if err != nil {
		return node{}, err
	}
	read.name, _ = name.Value().(string)
	var extents struct{ X, Y, Width, Height int32 }
	if err := object.CallWithContext(ctx, atspiComponent+".GetExtents", 0, uint32(coordinatesScreen)).Store(&extents); err == nil {
		read.extents = rect{X: float64(extents.X), Y: float64(extents.Y), Width: float64(extents.Width), Height: float64(extents.Height)}
	}
	return read, nil
}

// matchRule is AT-SPI's Collection match rule, (aiia{ss}iaiiasib): states, attributes, roles and interfaces, each with
// how it matches, and whether the rule is inverted.
type matchRule struct {
	States             []int32
	StateMatchType     int32
	Attributes         map[string]string
	AttributeMatchType int32
	Roles              []int32
	RoleMatchType      int32
	Interfaces         []string
	InterfaceMatchType int32
	Invert             bool
}

// find asks the application for the descendants with one of roles through Collection.GetMatches: one call, the
// application walks its own tree.
func (tree atspiTree) find(ctx context.Context, ref nodeRef, roles ...uint32) ([]nodeRef, error) {
	roleBits := make([]int32, 4)
	for _, role := range roles {
		roleBits[role/32] |= int32(uint32(1) << (role % 32))
	}
	rule := matchRule{
		States: []int32{0, 0}, StateMatchType: matchAll, Attributes: map[string]string{}, AttributeMatchType: matchAll,
		Roles: roleBits, RoleMatchType: matchAny, Interfaces: []string{}, InterfaceMatchType: matchAll,
	}
	var matches []atspiReference
	if err := tree.object(ref).CallWithContext(ctx, atspiCollection+".GetMatches", 0, rule, uint32(sortCanonical), int32(0), true).Store(&matches); err != nil {
		return nil, convertCallError(err)
	}
	refs := make([]nodeRef, 0, len(matches))
	for _, match := range matches {
		refs = append(refs, nodeRef{bus: match.Bus, path: string(match.Path)})
	}
	return refs, nil
}

func (tree atspiTree) parent(ctx context.Context, ref nodeRef) (nodeRef, error) {
	value, err := tree.property(ctx, ref, atspiAccessible+".Parent")
	if err != nil {
		return nodeRef{}, err
	}
	var parent atspiReference
	if err := dbus.Store([]any{value.Value()}, &parent); err != nil {
		return nodeRef{}, err
	}
	if parent.Path == atspiNullPath || parent.Path == "" {
		return nodeRef{}, nil
	}
	return nodeRef{bus: parent.Bus, path: string(parent.Path)}, nil
}

func (tree atspiTree) documentURL(ctx context.Context, document nodeRef) (string, error) {
	var attributes map[string]string
	if err := tree.object(document).CallWithContext(ctx, atspiDocument+".GetAttributes", 0).Store(&attributes); err != nil {
		return "", convertCallError(err)
	}
	return attributes["URI"], nil
}

func (tree atspiTree) characterCount(ctx context.Context, ref nodeRef) (int, error) {
	value, err := tree.property(ctx, ref, atspiText+".CharacterCount")
	if err != nil {
		return 0, err
	}
	count, isCount := value.Value().(int32)
	if !isCount {
		return 0, fmt.Errorf("axfill: CharacterCount is %T", value.Value())
	}
	return int(count), nil
}

func (tree atspiTree) children(ctx context.Context, ref nodeRef) ([]nodeRef, error) {
	var children []atspiReference
	if err := tree.object(ref).CallWithContext(ctx, atspiAccessible+".GetChildren", 0).Store(&children); err != nil {
		return nil, convertCallError(err)
	}
	refs := make([]nodeRef, 0, len(children))
	for _, child := range children {
		refs = append(refs, nodeRef{bus: child.Bus, path: string(child.Path)})
	}
	return refs, nil
}

// processID is the process that owns an application's connection to the bus, as the bus daemon verified it.
func (tree atspiTree) processID(ctx context.Context, bus string) (int, error) {
	var pid uint32
	if err := tree.connection.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.GetConnectionUnixProcessID", 0, bus).Store(&pid); err != nil {
		return 0, err
	}
	return int(pid), nil
}

// applications are the registry's children: one per application on the bus.
func (tree atspiTree) applications(ctx context.Context) ([]nodeRef, error) {
	return tree.children(ctx, nodeRef{bus: atspiRegistry, path: atspiRootPath})
}

// doAction runs the action named one of names, the first the accessible has; it fails when it has none of them.
// Names come from GetName: Chromium leaves the names GetActions lists empty.
func (tree atspiTree) doAction(ctx context.Context, ref nodeRef, names ...string) error {
	count, err := tree.property(ctx, ref, atspiAction+".NActions")
	if err != nil {
		return err
	}
	actionCount, _ := count.Value().(int32)
	actionNames := make([]string, actionCount)
	for index := range actionNames {
		if err := tree.object(ref).CallWithContext(ctx, atspiAction+".GetName", 0, int32(index)).Store(&actionNames[index]); err != nil {
			return convertCallError(err)
		}
	}
	for _, name := range names {
		index := slices.Index(actionNames, name)
		if index < 0 {
			continue
		}
		var isDone bool
		if err := tree.object(ref).CallWithContext(ctx, atspiAction+".DoAction", 0, int32(index)).Store(&isDone); err != nil {
			return convertCallError(err)
		}
		if !isDone {
			return errRefused
		}
		return nil
	}
	return errRefused
}

func (tree atspiTree) grabFocus(ctx context.Context, ref nodeRef) error {
	var isFocused bool
	if err := tree.object(ref).CallWithContext(ctx, atspiComponent+".GrabFocus", 0).Store(&isFocused); err != nil {
		return convertCallError(err)
	}
	if !isFocused {
		return errRefused
	}
	return nil
}
