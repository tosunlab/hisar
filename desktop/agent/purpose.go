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

	maximumCommandLineLength  = 160
	maximumSubjectLength      = 100
	maximumSignedPayloadSize  = 64 * 1024
	maximumGitHeadSize        = 4 * 1024
	defaultSSHPort            = 22
	redactedValue             = "•••"
	sshOptionLetterWithValue  = "BbcDEeFIiJLlmOoPpQRSWw"
	gitGlobalOptionWithValue  = "|-C|-c|--git-dir|--work-tree|--namespace|--super-prefix|--config-env|"
	sensitiveConfigNamePieces = "token|password|secret|header|auth"
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
	file, err := os.Open(path)
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
	host              string
	port              int
	allRemoteArgument []string
}

func getProgramName(process appinfo.Process) string {
	if process.Path != "" {
		return filepath.Base(process.Path)
	}
	if len(process.AllArgument) > 0 {
		return filepath.Base(process.AllArgument[0])
	}
	return ""
}

func isGitProcess(process appinfo.Process) bool {
	return getProgramName(process) == "git"
}

// parseSSHCommandLine reads the destination and remote command of an ssh invocation.
func parseSSHCommandLine(allArgument []string) sshCommandLine {
	parsed := sshCommandLine{}
	for index := 1; index < len(allArgument); index++ {
		argument := allArgument[index]
		if argument == "--" {
			if index+1 < len(allArgument) {
				parsed.setDestination(allArgument[index+1])
				parsed.allRemoteArgument = allArgument[index+2:]
			}
			return parsed
		}
		if !strings.HasPrefix(argument, "-") || argument == "-" {
			parsed.setDestination(argument)
			parsed.allRemoteArgument = allArgument[index+1:]
			return parsed
		}
		for letterIndex := 1; letterIndex < len(argument); letterIndex++ {
			letter := argument[letterIndex]
			if !strings.ContainsRune(sshOptionLetterWithValue, rune(letter)) {
				continue
			}
			value := argument[letterIndex+1:]
			if value == "" && index+1 < len(allArgument) {
				index++
				value = allArgument[index]
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
func findGitSubcommand(allArgument []string) string {
	for index := 1; index < len(allArgument); index++ {
		argument := allArgument[index]
		if !strings.HasPrefix(argument, "-") {
			return argument
		}
		if strings.Contains(gitGlobalOptionWithValue, "|"+argument+"|") {
			index++
		}
	}
	return ""
}

func describeRemoteGitCommand(allRemoteArgument []string) (string, bool) {
	allWord := strings.Fields(strings.Join(allRemoteArgument, " "))
	if len(allWord) == 0 {
		return "", false
	}
	command := filepath.Base(allWord[0])
	if command == "git" && len(allWord) > 1 {
		command = "git-" + allWord[1]
		allWord = allWord[1:]
	}
	switch command {
	case "git-receive-pack":
		return PurposePush, true
	case "git-upload-pack", "git-upload-archive":
		return PurposeFetch, true
	case "git-lfs-authenticate", "git-lfs-transfer":
		for _, word := range allWord[1:] {
			if strings.Trim(word, `'"`) == "upload" {
				return PurposePush, true
			}
		}
		return PurposeFetch, true
	}
	return "", false
}

var urlPasswordPattern = regexp.MustCompile(`(://[^/:@\s]*:)[^@/\s]+@`)
var sensitiveConfigPattern = regexp.MustCompile(`(?i)^([^=]*(?:` + sensitiveConfigNamePieces + `)[^=]*=).+$`)

func redactArgument(previousArgument, argument string) string {
	argument = urlPasswordPattern.ReplaceAllString(argument, "${1}"+redactedValue+"@")
	if previousArgument == "-c" {
		argument = sensitiveConfigPattern.ReplaceAllString(argument, "${1}"+redactedValue)
	}
	return argument
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

// formatCommandLine shows a process's arguments like a shell would, without secrets, cut to fit the dialog.
func formatCommandLine(allArgument []string) string {
	if len(allArgument) == 0 {
		return ""
	}
	allQuoted := []string{quoteArgument(filepath.Base(allArgument[0]))}
	for index := 1; index < len(allArgument); index++ {
		allQuoted = append(allQuoted, quoteArgument(redactArgument(allArgument[index-1], allArgument[index])))
	}
	return truncateText(strings.Join(allQuoted, " "), maximumCommandLineLength)
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
	allArgument := process.AllArgument
	if getProgramName(process) != "ssh-keygen" || len(allArgument) < 3 {
		return ""
	}
	isSign, isGitNamespace := false, false
	for index := 1; index+1 < len(allArgument); index++ {
		isSign = isSign || (allArgument[index] == "-Y" && allArgument[index+1] == "sign")
		isGitNamespace = isGitNamespace || (allArgument[index] == "-n" && allArgument[index+1] == sshagent.GitSignatureNamespace)
	}
	path := allArgument[len(allArgument)-1]
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

func findGitChain(allProcess []appinfo.Process) gitChain {
	chain := gitChain{}
	for index := range allProcess {
		process := &allProcess[index]
		if !isGitProcess(*process) {
			continue
		}
		if chain.nearest == nil {
			chain.nearest = process
		}
		chain.outermost = process
		switch findGitSubcommand(process.AllArgument) {
		case "pull":
			chain.isPull = true
		case "clone":
			chain.isClone = true
		}
	}
	return chain
}

// DescribeRequest says what a request is for from the requesting process chain (nearest first) and the agent request.
func DescribeRequest(allProcess []appinfo.Process, request sshagent.ApprovalRequest, homeDirectory string, readFile FileReader) RequestDescription {
	if readFile == nil {
		readFile = ReadSmallRegularFile
	}
	description := RequestDescription{Purpose: PurposeUseKey}
	var peer appinfo.Process
	if len(allProcess) > 0 {
		peer = allProcess[0]
	}
	chain := findGitChain(allProcess)
	if chain.nearest != nil && chain.nearest.WorkingDirectory != "" {
		description.RepositoryPath = shortenHomePath(chain.nearest.WorkingDirectory, homeDirectory)
	}
	host, port := request.DestinationHost, request.DestinationPort
	parsedSSH := sshCommandLine{}
	if getProgramName(peer) == "ssh" {
		parsedSSH = parseSSHCommandLine(peer.AllArgument)
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
	case len(parsedSSH.allRemoteArgument) > 0 && isGitRemoteCommand(parsedSSH.allRemoteArgument):
		purpose, _ := describeRemoteGitCommand(parsedSSH.allRemoteArgument)
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
	if (commandProcess == nil || request.IsForwarded) && len(peer.AllArgument) > 0 {
		commandProcess = &peer
	}
	if commandProcess != nil {
		description.CommandLine = formatCommandLine(commandProcess.AllArgument)
	}
	return description
}

func isGitRemoteCommand(allRemoteArgument []string) bool {
	_, isGit := describeRemoteGitCommand(allRemoteArgument)
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
