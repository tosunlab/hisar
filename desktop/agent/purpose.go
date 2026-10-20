package agent

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/tryhisar/hisar/core/sshagent"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

const (
	PurposePush       = "push"
	PurposePull       = "pull"
	PurposeFetch      = "fetch"
	PurposeClone      = "clone"
	PurposeSignIn     = "sign-in"
	PurposeSignCommit = "sign-commit"
	PurposeSignTag    = "sign-tag"
	PurposeUseKey     = "use-key"

	maximumCommandLineLength = 160
	maximumSubjectLength     = 100
	maximumSignedPayloadSize = 64 * 1024
	maximumGitHeadSize       = 4 * 1024
	defaultSSHPort           = 22
	redactedValue            = "•••"
	sshOptionLetterWithValue = "BbcDEeFIiJLlmOoPpQRSWw"
	gitGlobalOptionWithValue = "|-C|-c|--git-dir|--work-tree|--namespace|--super-prefix|--config-env|"
)

// RequestDescription is what the approval dialog says about a request: "<app> wants to <Action> <Target>".
type RequestDescription struct {
	Purpose           string
	Action            string
	Target            string
	CommandLine       string
	RepositoryPath    string
	HostAndPort       string
	Branch            string
	CommitSubject     string
	ForwardedFromHost string
}

// FileReader reads a small regular file, failing for anything larger than maximumSize.
type FileReader func(path string, maximumSize int64) ([]byte, error)

func ReadSmallRegularFile(path string, maximumSize int64) ([]byte, error) {
	// Non-blocking, so a named pipe in the arguments cannot hold up the request.
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !fileInfo.Mode().IsRegular() || fileInfo.Size() > maximumSize {
		return nil, errors.New("agent: not a small regular file")
	}
	return io.ReadAll(io.LimitReader(file, maximumSize))
}

type sshCommandLine struct {
	host            string
	port            int
	remoteArguments []string
}

func isGitProcess(process appinfo.Process) bool {
	return appinfo.GetProgramName(process) == "git"
}

// parseSSHCommandLine reads the destination and remote command of an ssh invocation.
func parseSSHCommandLine(arguments []string) sshCommandLine {
	parsed := sshCommandLine{}
	for index := 1; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--" {
			if index+1 < len(arguments) {
				parsed.setDestination(arguments[index+1])
				parsed.remoteArguments = arguments[index+2:]
			}
			return parsed
		}
		if !strings.HasPrefix(argument, "-") || argument == "-" {
			parsed.setDestination(argument)
			parsed.remoteArguments = arguments[index+1:]
			return parsed
		}
		for letterIndex := 1; letterIndex < len(argument); letterIndex++ {
			letter := argument[letterIndex]
			if !strings.ContainsRune(sshOptionLetterWithValue, rune(letter)) {
				continue
			}
			value := argument[letterIndex+1:]
			if value == "" && index+1 < len(arguments) {
				index++
				value = arguments[index]
			}
			parsed.applyOption(letter, value)
			break
		}
	}
	return parsed
}

func (parsed *sshCommandLine) applyOption(letter byte, value string) {
	switch letter {
	case 'p':
		if port, err := strconv.Atoi(value); err == nil && port > 0 && port <= 65535 {
			parsed.port = port
		}
	case 'o':
		name, optionValue, isFound := strings.Cut(value, "=")
		if !isFound {
			name, optionValue, _ = strings.Cut(value, " ")
		}
		if strings.EqualFold(strings.TrimSpace(name), "Port") {
			parsed.applyOption('p', strings.TrimSpace(optionValue))
		}
	}
}

func (parsed *sshCommandLine) setDestination(destination string) {
	if rest, isURL := strings.CutPrefix(destination, "ssh://"); isURL {
		rest, _, _ = strings.Cut(rest, "/")
		if at := strings.LastIndex(rest, "@"); at >= 0 {
			rest = rest[at+1:]
		}
		if host, portText, err := net.SplitHostPort(rest); err == nil {
			parsed.host = host
			parsed.applyOption('p', portText)
			return
		}
		parsed.host = strings.Trim(rest, "[]")
		return
	}
	if at := strings.LastIndex(destination, "@"); at >= 0 {
		destination = destination[at+1:]
	}
	parsed.host = destination
}

// findGitSubcommand skips git's global options, like -C <path>, to the subcommand.
func findGitSubcommand(arguments []string) string {
	for index := 1; index < len(arguments); index++ {
		argument := arguments[index]
		if !strings.HasPrefix(argument, "-") {
			return argument
		}
		if strings.Contains(gitGlobalOptionWithValue, "|"+argument+"|") {
			index++
		}
	}
	return ""
}

func describeRemoteGitCommand(remoteArguments []string) (string, bool) {
	words := strings.Fields(strings.Join(remoteArguments, " "))
	if len(words) == 0 {
		return "", false
	}
	command := filepath.Base(words[0])
	if command == "git" && len(words) > 1 {
		command = "git-" + words[1]
		words = words[1:]
	}
	switch command {
	case "git-receive-pack":
		return PurposePush, true
	case "git-upload-pack", "git-upload-archive":
		return PurposeFetch, true
	case "git-lfs-authenticate", "git-lfs-transfer":
		for _, word := range words[1:] {
			if strings.Trim(word, `'"`) == "upload" {
				return PurposePush, true
			}
		}
		return PurposeFetch, true
	}
	return "", false
}

var (
	// urlPasswordPattern finds the password of a URL, https://user:password@host, made only of characters that are never
	// shell syntax, so a mask never hides a command when git hands an argument to a shell (git bisect run, an alias).
	urlPasswordPattern = regexp.MustCompile(`(://[^/?#@\s:]*:)[^\s;&|()<>$\x60\\'"/?#@]+(@)`)
	// gitSecretConfigPattern matches the last part of a git config key that holds a credential, such as
	// http.extraHeader or github.token.
	gitSecretConfigPattern = regexp.MustCompile(`(?i)token|password|secret|header|auth`)
)

// redactGitArgument hides the credentials git takes in its own arguments: the value of a -c option named like a
// secret or a header, and the password of a URL. An alias runs, so its value stays whole, in -c alias.x=… and in
// git config alias.x ….
func redactGitArgument(previousArgument, argument string) string {
	if strings.HasPrefix(strings.ToLower(previousArgument), "alias.") {
		return argument
	}
	if previousArgument == "-c" {
		key, value, _ := strings.Cut(argument, "=")
		section, _, _ := strings.Cut(key, ".")
		if strings.EqualFold(section, "alias") {
			return argument
		}
		if value != "" && gitSecretConfigPattern.MatchString(key[strings.LastIndex(key, ".")+1:]) {
			return key + "=" + redactedValue
		}
	}
	return urlPasswordPattern.ReplaceAllString(argument, "${1}"+redactedValue+"${2}")
}

func quoteArgument(argument string) string {
	if argument == "" {
		return "''"
	}
	for _, character := range argument {
		isSafe := character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || strings.ContainsRune("_@%+=:,./-~•", character)
		if !isSafe {
			return "'" + strings.ReplaceAll(argument, "'", `'\''`) + "'"
		}
	}
	return argument
}

// formatCommandLine shows every argument of a command like a shell would. Only git's own credentials are hidden: the
// approval shows on the user's own Mac and phones, and any other literal in a command came from whoever wrote it, so
// masking it would protect nothing and could hide what the command does.
func formatCommandLine(arguments []string) string {
	if len(arguments) == 0 {
		return ""
	}
	program := filepath.Base(arguments[0])
	quoted := []string{quoteArgument(program)}
	for index := 1; index < len(arguments); index++ {
		argument := arguments[index]
		if program == "git" {
			argument = redactGitArgument(arguments[index-1], argument)
		}
		quoted = append(quoted, quoteArgument(argument))
	}
	return strings.ToValidUTF8(strings.Join(quoted, " "), "�")
}

// formatShortCommandLine is formatCommandLine cut to one line of the dialog, for a process that only explains a request.
func formatShortCommandLine(arguments []string) string {
	return truncateText(formatCommandLine(arguments), maximumCommandLineLength)
}

func truncateText(text string, maximumLength int) string {
	text = strings.ToValidUTF8(text, "�")
	if utf8.RuneCountInString(text) <= maximumLength {
		return text
	}
	return string([]rune(text)[:maximumLength-1]) + "…"
}

func shortenHomePath(path, homeDirectory string) string {
	if homeDirectory == "" || path == "" {
		return path
	}
	homeDirectory = strings.TrimSuffix(homeDirectory, "/")
	if path == homeDirectory {
		return "~"
	}
	if rest, isInHome := strings.CutPrefix(path, homeDirectory+"/"); isInHome {
		return "~/" + rest
	}
	return path
}

func formatHostAndPort(host string, port int) string {
	if host == "" {
		return ""
	}
	if port == 0 {
		port = defaultSSHPort
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// readBranch follows .git (a directory, or a file pointing at the git directory of a worktree) to HEAD.
func readBranch(repositoryPath string, readFile FileReader) string {
	gitPath := filepath.Join(repositoryPath, ".git")
	headContent, err := readFile(filepath.Join(gitPath, "HEAD"), maximumGitHeadSize)
	if err != nil {
		pointer, pointerError := readFile(gitPath, maximumGitHeadSize)
		gitDirectory, isPointer := strings.CutPrefix(strings.TrimSpace(string(pointer)), "gitdir: ")
		if pointerError != nil || !isPointer {
			return ""
		}
		if !filepath.IsAbs(gitDirectory) {
			gitDirectory = filepath.Join(repositoryPath, gitDirectory)
		}
		if headContent, err = readFile(filepath.Join(gitDirectory, "HEAD"), maximumGitHeadSize); err != nil {
			return ""
		}
	}
	branch, _ := strings.CutPrefix(strings.TrimSpace(string(headContent)), "ref: refs/heads/")
	if branch == strings.TrimSpace(string(headContent)) {
		return ""
	}
	return branch
}

// parseSignedPayload reads the commit or tag git hands to ssh-keygen: headers, a blank line, then the message.
func parseSignedPayload(payload []byte) (isTag bool, tagName string, subject string) {
	scanner := bufio.NewScanner(bytes.NewReader(payload))
	scanner.Buffer(make([]byte, 0, 4096), maximumSignedPayloadSize)
	isHeader := true
	for scanner.Scan() {
		line := scanner.Text()
		if isHeader {
			if line == "" {
				isHeader = false
				continue
			}
			if name, isTagLine := strings.CutPrefix(line, "tag "); isTagLine {
				isTag, tagName = true, name
			}
			continue
		}
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return isTag, tagName, truncateText(trimmed, maximumSubjectLength)
		}
	}
	return isTag, tagName, ""
}

func findSignedPayloadPath(process appinfo.Process) string {
	arguments := process.Arguments
	if appinfo.GetProgramName(process) != "ssh-keygen" || len(arguments) < 3 {
		return ""
	}
	isSign, isGitNamespace := false, false
	for index := 1; index+1 < len(arguments); index++ {
		isSign = isSign || (arguments[index] == "-Y" && arguments[index+1] == "sign")
		isGitNamespace = isGitNamespace || (arguments[index] == "-n" && arguments[index+1] == sshagent.GitSignatureNamespace)
	}
	path := arguments[len(arguments)-1]
	if !isSign || !isGitNamespace || strings.HasPrefix(path, "-") {
		return ""
	}
	if !filepath.IsAbs(path) {
		if process.WorkingDirectory == "" {
			return ""
		}
		path = filepath.Join(process.WorkingDirectory, path)
	}
	return path
}

type gitChain struct {
	nearest   *appinfo.Process
	outermost *appinfo.Process
	isPull    bool
	isClone   bool
}

func findGitChain(processes []appinfo.Process) gitChain {
	chain := gitChain{}
	for index := range processes {
		process := &processes[index]
		if !isGitProcess(*process) {
			continue
		}
		if chain.nearest == nil {
			chain.nearest = process
		}
		chain.outermost = process
		switch findGitSubcommand(process.Arguments) {
		case "pull":
			chain.isPull = true
		case "clone":
			chain.isClone = true
		}
	}
	return chain
}

// DescribeRequest says what a request is for from the requesting process chain (nearest first) and the agent request.
func DescribeRequest(processes []appinfo.Process, request sshagent.ApprovalRequest, homeDirectory string, readFile FileReader) RequestDescription {
	if readFile == nil {
		readFile = ReadSmallRegularFile
	}
	description := RequestDescription{Purpose: PurposeUseKey}
	var peer appinfo.Process
	if len(processes) > 0 {
		peer = processes[0]
	}
	chain := findGitChain(processes)
	if chain.nearest != nil && chain.nearest.WorkingDirectory != "" {
		description.RepositoryPath = shortenHomePath(chain.nearest.WorkingDirectory, homeDirectory)
	}
	host, port := request.DestinationHost, request.DestinationPort
	parsedSSH := sshCommandLine{}
	if appinfo.GetProgramName(peer) == "ssh" {
		parsedSSH = parseSSHCommandLine(peer.Arguments)
	}
	if request.IsForwarded {
		description.ForwardedFromHost = request.ForwardedFromHost
		if request.ForwardedFromPort != 0 {
			description.ForwardedFromHost = formatHostAndPort(request.ForwardedFromHost, request.ForwardedFromPort)
		}
		if description.ForwardedFromHost == "" {
			description.ForwardedFromHost = parsedSSH.host
		}
		if description.ForwardedFromHost == "" {
			description.ForwardedFromHost = request.ForwardedFromFingerprint
		}
		description.RepositoryPath = ""
		parsedSSH = sshCommandLine{}
	} else if parsedSSH.host != "" {
		host, port = parsedSSH.host, parsedSSH.port
	}
	description.HostAndPort = formatHostAndPort(host, port)
	if description.HostAndPort == "" {
		description.HostAndPort = request.DestinationFingerprint
	}

	switch {
	case request.SignatureNamespace == sshagent.GitSignatureNamespace || (request.IsUnlockRequired && findSignedPayloadPath(peer) != ""):
		description.Purpose, description.Action, description.HostAndPort = PurposeSignCommit, "sign a commit", ""
		if payloadPath := findSignedPayloadPath(peer); payloadPath != "" {
			if payload, err := readFile(payloadPath, maximumSignedPayloadSize); err == nil {
				isTag, tagName, subject := parseSignedPayload(payload)
				description.CommitSubject = subject
				if isTag && tagName != "" {
					description.Purpose, description.Action, description.Target = PurposeSignTag, "sign the tag", truncateText(tagName, maximumSubjectLength)
				}
			}
		}
		if chain.nearest != nil && chain.nearest.WorkingDirectory != "" {
			description.Branch = readBranch(chain.nearest.WorkingDirectory, readFile)
		}
	case len(parsedSSH.remoteArguments) > 0 && isGitRemoteCommand(parsedSSH.remoteArguments):
		purpose, _ := describeRemoteGitCommand(parsedSSH.remoteArguments)
		if purpose == PurposeFetch && chain.isPull {
			purpose = PurposePull
		} else if purpose == PurposeFetch && chain.isClone {
			purpose = PurposeClone
		}
		description.Purpose, description.Target = purpose, host
		description.Action = map[string]string{PurposePush: "push to", PurposePull: "pull from", PurposeClone: "clone from", PurposeFetch: "fetch from"}[purpose]
	case host != "":
		description.Purpose, description.Action, description.Target = PurposeSignIn, "sign in to", host
	default:
		description.Action = "use your SSH key"
	}

	commandProcess := chain.outermost
	if (commandProcess == nil || request.IsForwarded) && len(peer.Arguments) > 0 {
		commandProcess = &peer
	}
	if commandProcess != nil {
		description.CommandLine = formatShortCommandLine(commandProcess.Arguments)
	}
	return description
}

func isGitRemoteCommand(remoteArguments []string) bool {
	_, isGit := describeRemoteGitCommand(remoteArguments)
	return isGit
}

// formatApprovalReason is the Touch ID reason, e.g. allow "Terminal" to push to git.tryhisar.com.
func formatApprovalReason(appName string, description RequestDescription) string {
	action := strings.TrimSpace(description.Action + " " + description.Target)
	if description.Purpose == PurposeUseKey && description.HostAndPort != "" && !strings.HasPrefix(description.HostAndPort, "SHA256:") {
		action += " for " + description.HostAndPort
	}
	reason := fmt.Sprintf("allow %q to %s", appName, action)
	if description.ForwardedFromHost != "" {
		reason += " from " + description.ForwardedFromHost + " through agent forwarding"
	}
	return reason
}
