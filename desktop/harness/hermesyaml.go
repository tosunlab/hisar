package harness

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Hermes' config.yaml is the user's file, with their comments and layout, and Hermes rewrites it in place too. Hisar
// finds where things are with a YAML parser, changes only the lines of its own entry, and then parses the result again:
// an edit that would change anything but Hisar's entry is refused, never written.

var errHermesLayout = errors.New("Hermes' config.yaml lists its MCP servers in a form Hisar cannot change. Add Hisar's server there by hand")

var errHermesDuplicateKey = errors.New("Hermes' config.yaml has a key twice in one place, so Hisar cannot tell which one Hermes reads. Remove one and try again")

var errHermesAliasKey = errors.New("Hermes' config.yaml uses an alias as a key, so Hisar cannot tell which keys Hermes reads. Write the key out and try again")

// hermesTimeoutSeconds lets a tool call outlast run_with_secrets' one hour and its approval, as Codex's
// tool_timeout_sec does; Hermes cancels a call at its timeout, and 0 would cancel every call at once.
const (
	hermesTimeoutSeconds        = 4200
	hermesConnectTimeoutSeconds = 60
)

// hermesServer is the entry Hisar writes as mcp_servers.hisar. socketPath is the app's HISAR_CLI_SOCKET when it is not
// the default: Hermes starts MCP servers with only a few of its own variables, so the entry has to carry it.
type hermesServer struct {
	hisarPath  string
	socketPath string
}

func (server hermesServer) value() map[string]any {
	arguments := make([]any, len(mcpServerArguments))
	for i, argument := range mcpServerArguments {
		arguments[i] = argument
	}
	value := map[string]any{
		"command": server.hisarPath, "args": arguments, "timeout": hermesTimeoutSeconds, "connect_timeout": hermesConnectTimeoutSeconds,
	}
	if server.socketPath != "" {
		value["env"] = map[string]any{"HISAR_CLI_SOCKET": server.socketPath}
	}
	return value
}

func (server hermesServer) text(indent, step, newline string) string {
	quotedArguments := make([]string, len(mcpServerArguments))
	for i, argument := range mcpServerArguments {
		quotedArguments[i] = quoteYAML(argument)
	}
	inner := indent + step
	lines := []string{
		indent + mcpServerName + ":",
		inner + "command: " + quoteYAML(server.hisarPath),
		inner + "args: [" + strings.Join(quotedArguments, ", ") + "]",
		inner + "timeout: " + strconv.Itoa(hermesTimeoutSeconds),
		inner + "connect_timeout: " + strconv.Itoa(hermesConnectTimeoutSeconds),
	}
	if server.socketPath != "" {
		lines = append(lines, inner+"env:", inner+step+"HISAR_CLI_SOCKET: "+quoteYAML(server.socketPath))
	}
	return strings.Join(lines, newline) + newline
}

func quoteYAML(value string) string {
	var quoted bytes.Buffer
	encoder := json.NewEncoder(&quoted)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
	return strings.TrimSuffix(quoted.String(), "\n")
}

type hermesDocument struct {
	content []byte
	lines   [][]byte
	root    *yaml.Node
	config  map[string]any
	newline string
}

func parseHermesDocument(content []byte) (hermesDocument, error) {
	document := hermesDocument{content: content, lines: bytes.SplitAfter(content, []byte("\n")), newline: "\n"}
	if last := len(document.lines) - 1; len(document.lines[last]) == 0 {
		document.lines = document.lines[:last]
	}
	if index := bytes.IndexByte(content, '\n'); index > 0 && content[index-1] == '\r' {
		document.newline = "\r\n"
	}
	decoder := yaml.NewDecoder(bytes.NewReader(content))
	var node yaml.Node
	if err := decoder.Decode(&node); err != nil && !errors.Is(err, io.EOF) {
		return hermesDocument{}, errors.New("Hermes' config.yaml is not valid YAML")
	}
	var next yaml.Node
	if err := decoder.Decode(&next); !errors.Is(err, io.EOF) {
		return hermesDocument{}, errHermesLayout
	}
	if err := checkKeys(&node); err != nil {
		return hermesDocument{}, err
	}
	if node.Kind == yaml.DocumentNode && len(node.Content) == 1 {
		switch root := node.Content[0]; {
		case root.Kind == yaml.MappingNode && root.Style&yaml.FlowStyle == 0 && root.Column == 1:
			document.root = root
			if err := root.Decode(&document.config); err != nil {
				return hermesDocument{}, errors.New("Hermes' config.yaml is not valid YAML")
			}
		case root.Kind != yaml.ScalarNode || root.Tag != "!!null":
			return hermesDocument{}, errHermesLayout
		}
	}
	return document, nil
}

// checkKeys refuses a mapping anywhere in node with a key twice, since Hermes reads the last one, which no search for
// the first one sees, and an alias used as a key (`*name: value`), whose key only Hermes' parse resolves.
func checkKeys(node *yaml.Node) error {
	if node.Kind == yaml.MappingNode {
		keys := map[string]bool{}
		for i := 0; i < len(node.Content); i += 2 {
			switch key := node.Content[i]; key.Kind {
			case yaml.AliasNode:
				return errHermesAliasKey
			case yaml.ScalarNode:
				if keys[key.Value] {
					return errHermesDuplicateKey
				}
				keys[key.Value] = true
			}
		}
	}
	for _, child := range node.Content {
		if err := checkKeys(child); err != nil {
			return err
		}
	}
	return nil
}

func (document hermesDocument) lineStart(line int) int {
	offset := 0
	for _, text := range document.lines[:line-1] {
		offset += len(text)
	}
	return offset
}

// endLine is the last line of a block mapping entry: the last line of any node in it, then any line after it that is
// indented deeper than its key, as a block scalar's text and comments inside the entry are.
func (document hermesDocument) endLine(key, value *yaml.Node) int {
	end := max(key.Line, getLastLine(value))
	for line := end + 1; line <= len(document.lines); line++ {
		text := document.lines[line-1]
		trimmed := bytes.TrimLeft(text, " ")
		if len(bytes.TrimSpace(trimmed)) == 0 {
			continue
		}
		if len(text)-len(trimmed) < key.Column {
			break
		}
		end = line
	}
	return end
}

func getLastLine(node *yaml.Node) int {
	last := node.Line
	for _, child := range node.Content {
		last = max(last, getLastLine(child))
	}
	return last
}

func findEntry(mapping *yaml.Node, name string) (*yaml.Node, *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if key := mapping.Content[i]; key.Kind == yaml.ScalarNode && key.Value == name {
			return key, mapping.Content[i+1]
		}
	}
	return nil, nil
}

func isMerge(key *yaml.Node) bool {
	return key.Kind == yaml.ScalarNode && key.ShortTag() == "!!merge"
}

type hermesEntry struct {
	serversKey, servers *yaml.Node
	hisarKey, hisar     *yaml.Node
	isNamed, isOurs     bool
}

// findServer reads mcp_servers as Hermes does. A top level that merges another mapping may bring its own mcp_servers,
// and an anchored mcp_servers may be copied elsewhere, so Hisar does not tell their state; a hisar server merged into
// mcp_servers is the user's.
func (document hermesDocument) findServer(server hermesServer) (hermesEntry, error) {
	entry := hermesEntry{}
	if document.root == nil {
		return entry, nil
	}
	if slices.ContainsFunc(document.root.Content, isMerge) {
		return hermesEntry{}, errHermesLayout
	}
	entry.serversKey, entry.servers = findEntry(document.root, "mcp_servers")
	if entry.servers == nil {
		return entry, nil
	}
	switch servers := entry.servers; {
	case servers.Kind == yaml.ScalarNode && servers.Tag == "!!null" && servers.Value == "":
		return entry, nil
	case servers.Kind != yaml.MappingNode || servers.Anchor != "":
		return hermesEntry{}, errHermesLayout
	}
	servers, _ := document.config["mcp_servers"].(map[string]any)
	var value any
	value, entry.isNamed = servers[mcpServerName]
	entry.hisarKey, entry.hisar = findEntry(entry.servers, mcpServerName)
	isFlow := entry.servers.Style&yaml.FlowStyle != 0
	switch {
	case !entry.isNamed && isFlow:
		return hermesEntry{}, errHermesLayout
	case entry.isNamed:
		entry.isOurs = entry.hisar != nil && !isFlow && reflect.DeepEqual(value, server.value())
	}
	return entry, nil
}

// setHermesServer returns content with Hisar's mcp_servers.hisar added after the other servers, or after the end of the
// file when it has no mcp_servers; every other byte stays. Hisar's own entry, however Hermes reformatted it, stays as it
// is. A server named hisar that Hisar did not write is errConflict.
func setHermesServer(content []byte, server hermesServer) ([]byte, error) {
	document, err := parseHermesDocument(content)
	if err != nil {
		return nil, err
	}
	entry, err := document.findServer(server)
	if err != nil {
		return nil, err
	}
	switch {
	case entry.isOurs:
		return content, nil
	case entry.isNamed:
		return nil, errConflict
	}
	var edited []byte
	if entry.servers == nil {
		edited = slices.Clone(content)
		if len(edited) > 0 {
			if !bytes.HasSuffix(edited, []byte("\n")) {
				edited = append(edited, document.newline...)
			}
			edited = append(edited, document.newline...)
		}
		edited = append(edited, "mcp_servers:"+document.newline+server.text("  ", "  ", document.newline)...)
	} else {
		parentIndent := entry.serversKey.Column - 1
		indent, afterLine := parentIndent+2, document.endLine(entry.serversKey, entry.servers)
		if children := entry.servers.Content; len(children) > 0 {
			lastKey := children[len(children)-2]
			indent, afterLine = lastKey.Column-1, document.endLine(lastKey, children[len(children)-1])
		}
		step := strings.Repeat(" ", indent-parentIndent)
		at := document.lineStart(afterLine + 1)
		before := content[:at]
		if len(before) > 0 && !bytes.HasSuffix(before, []byte("\n")) {
			before = append(slices.Clone(before), document.newline...)
		}
		edited = slices.Concat(before, []byte(server.text(strings.Repeat(" ", indent), step, document.newline)), content[at:])
	}
	if err := checkHermesEdit(content, edited, func(config map[string]any) {
		servers, _ := config["mcp_servers"].(map[string]any)
		if servers == nil {
			servers = map[string]any{}
		}
		servers[mcpServerName] = server.value()
		config["mcp_servers"] = servers
	}); err != nil {
		return nil, err
	}
	return edited, nil
}

// removeHermesServer returns content without Hisar's entry, and without mcp_servers when Hisar's entry was all it held
// and nothing but Hisar's lines are under it, a comment of the user's included. Content without Hisar's entry comes
// back as it is.
func removeHermesServer(content []byte, server hermesServer) ([]byte, error) {
	document, err := parseHermesDocument(content)
	if err != nil {
		return nil, err
	}
	entry, err := document.findServer(server)
	if err != nil || !entry.isOurs {
		return content, err
	}
	start, end := entry.hisarKey.Line, document.endLine(entry.hisarKey, entry.hisar)
	isAlone := len(entry.servers.Content) == 2 && entry.serversKey.Line+1 == start &&
		document.endLine(entry.serversKey, entry.servers) == end &&
		strings.TrimRight(string(document.lines[entry.serversKey.Line-1]), "\r\n") == "mcp_servers:"
	if isAlone {
		start = entry.serversKey.Line
		if end == len(document.lines) && start > 1 && len(bytes.TrimSpace(document.lines[start-2])) == 0 {
			start--
		}
	}
	edited := slices.Concat(content[:document.lineStart(start)], content[document.lineStart(end+1):])
	if err := checkHermesEdit(content, edited, func(config map[string]any) {
		servers, _ := config["mcp_servers"].(map[string]any)
		delete(servers, mcpServerName)
		switch {
		case isAlone:
			delete(config, "mcp_servers")
		case len(servers) == 0:
			config["mcp_servers"] = nil
		}
	}); err != nil {
		return nil, err
	}
	return edited, nil
}

func checkHermesEdit(original, edited []byte, change func(config map[string]any)) error {
	var want, got map[string]any
	if yaml.Unmarshal(original, &want) != nil || yaml.Unmarshal(edited, &got) != nil {
		return errHermesLayout
	}
	if want == nil {
		want = map[string]any{}
	}
	change(want)
	if got == nil {
		got = map[string]any{}
	}
	if !reflect.DeepEqual(want, got) {
		return errHermesLayout
	}
	return nil
}
