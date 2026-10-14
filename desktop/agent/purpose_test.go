package agent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/sshagent"
	"github.com/tryhisar/hisar/desktop/appinfo"
)

const (
	testHomeDirectory   = "/Users/developer"
	testRepositoryPath  = "/Users/developer/src/core"
	testXcodeGitPath    = "/Applications/Xcode.app/Contents/Developer/usr/bin/git"
	testTerminalPath    = "/System/Applications/Utilities/Terminal.app/Contents/MacOS/Terminal"
	testSigningBuffer   = "/var/folders/xy/T/.git_signing_buffer_tmpAbC123"
	testRemoteHost      = "git.tryhisar.com"
	testRemoteGitPushed = "git-receive-pack 'hisar/core.git'"
)

type fakeFileSystem map[string]string

func (fileSystem fakeFileSystem) readFile(path string, maximumSize int64) ([]byte, error) {
	content, isFound := fileSystem[path]
	if !isFound {
		return nil, errors.New("no such file")
	}
	if int64(len(content)) > maximumSize {
		return nil, errors.New("too large")
	}
	return []byte(content), nil
}

func createSSHProcess(arguments ...string) appinfo.Process {
	return appinfo.Process{PID: 50, Path: "/usr/bin/ssh", Arguments: append([]string{"ssh"}, arguments...), WorkingDirectory: testRepositoryPath}
}

func createGitProcess(pid int, arguments ...string) appinfo.Process {
	return appinfo.Process{PID: pid, Path: testXcodeGitPath, Arguments: append([]string{"git"}, arguments...), WorkingDirectory: testRepositoryPath}
}

func createShellAndTerminal() []appinfo.Process {
	return []appinfo.Process{
		{PID: 30, Path: "/bin/zsh", Arguments: []string{"-zsh"}, WorkingDirectory: testRepositoryPath},
		{PID: 10, Path: testTerminalPath, Arguments: []string{testTerminalPath}, WorkingDirectory: "/"},
	}
}

func describeChain(processes []appinfo.Process, request sshagent.ApprovalRequest, fileSystem fakeFileSystem) RequestDescription {
	return DescribeRequest(processes, request, testHomeDirectory, fileSystem.readFile)
}

func TestDescribeGitOverSSH(t *testing.T) {
	cases := []struct {
		name           string
		processes      []appinfo.Process
		wantPurpose    string
		wantAction     string
		wantCommand    string
		wantHostAndPor string
	}{
		{"git push", append([]appinfo.Process{
			createSSHProcess("-o", "SendEnv=GIT_PROTOCOL", "git@"+testRemoteHost, testRemoteGitPushed),
			createGitProcess(40, "push", "origin", "main"),
		}, createShellAndTerminal()...), PurposePush, "push to", "git push origin main", "git.tryhisar.com:22"},
		{"git pull runs git fetch", append([]appinfo.Process{
			createSSHProcess("-o", "SendEnv=GIT_PROTOCOL", "-p", "2222", "git@"+testRemoteHost, "git-upload-pack 'hisar/core.git'"),
			createGitProcess(41, "fetch", "--update-head-ok"),
			createGitProcess(40, "pull", "--rebase"),
		}, createShellAndTerminal()...), PurposePull, "pull from", "git pull --rebase", "git.tryhisar.com:2222"},
		{"git fetch with global options", append([]appinfo.Process{
			createSSHProcess("-o", "SendEnv=GIT_PROTOCOL", "git@"+testRemoteHost, "git-upload-pack '/srv/core.git'"),
			createGitProcess(40, "-C", "pull", "-c", "core.sshCommand=ssh", "fetch", "--all"),
		}, createShellAndTerminal()...), PurposeFetch, "fetch from", "git -C pull -c core.sshCommand=ssh fetch --all", "git.tryhisar.com:22"},
		{"git clone", append([]appinfo.Process{
			createSSHProcess("-o", "SendEnv=GIT_PROTOCOL", "ssh://git@"+testRemoteHost+":2200/hisar/core.git", "git-upload-pack '/hisar/core.git'"),
			createGitProcess(40, "clone", "ssh://git@"+testRemoteHost+":2200/hisar/core.git"),
		}, createShellAndTerminal()...), PurposeClone, "clone from", "git clone ssh://git@git.tryhisar.com:2200/hisar/core.git", "git.tryhisar.com:2200"},
		{"Git LFS upload", append([]appinfo.Process{
			createSSHProcess("git@"+testRemoteHost, "git-lfs-authenticate", "'hisar/core.git'", "upload"),
			{PID: 45, Path: "/opt/homebrew/bin/git-lfs", Arguments: []string{"git-lfs", "pre-push", "origin"}},
			createGitProcess(40, "push"),
		}, createShellAndTerminal()...), PurposePush, "push to", "git push", "git.tryhisar.com:22"},
		{"git as a remote command word", []appinfo.Process{createSSHProcess("-oPort=443", "git@"+testRemoteHost, "git upload-pack 'x'")}, PurposeFetch, "fetch from", "ssh -oPort=443 git@git.tryhisar.com 'git upload-pack '\\''x'\\'''", "git.tryhisar.com:443"},
		{"ssh to a server", append([]appinfo.Process{
			createSSHProcess("-l", "deploy", "-i", "~/.ssh/id_ed25519.pub", "prod.example.com"),
		}, createShellAndTerminal()...), PurposeSignIn, "sign in to", "ssh -l deploy -i ~/.ssh/id_ed25519.pub prod.example.com", "prod.example.com:22"},
		{"ssh with a port option and an IPv6 host", []appinfo.Process{createSSHProcess("-p2201", "-o", "Port 2202", "admin@2001:db8::1")}, PurposeSignIn, "sign in to", "ssh -p2201 -o 'Port 2202' admin@2001:db8::1", "[2001:db8::1]:2202"},
	}
	for _, testCase := range cases {
		description := describeChain(testCase.processes, sshagent.ApprovalRequest{}, nil)
		if description.Purpose != testCase.wantPurpose || description.Action != testCase.wantAction || description.CommandLine != testCase.wantCommand || description.HostAndPort != testCase.wantHostAndPor {
			t.Errorf("%s: %+v", testCase.name, description)
		}
		if description.Purpose != PurposeSignIn && description.RepositoryPath != "" && description.RepositoryPath != "~/src/core" {
			t.Errorf("%s: repository %q", testCase.name, description.RepositoryPath)
		}
	}
}

func TestDescribeUsesTheSessionBindingWithoutArguments(t *testing.T) {
	request := sshagent.ApprovalRequest{DestinationHost: "github.com", DestinationPort: 443, DestinationFingerprint: "SHA256:host"}
	description := describeChain(nil, request, nil)
	if description.Purpose != PurposeSignIn || description.Target != "github.com" || description.HostAndPort != "github.com:443" || description.CommandLine != "" {
		t.Errorf("bound request: %+v", description)
	}
	description = describeChain(nil, sshagent.ApprovalRequest{DestinationFingerprint: "SHA256:host"}, nil)
	if description.Purpose != PurposeUseKey || description.Action != "use your SSH key" || description.HostAndPort != "SHA256:host" {
		t.Errorf("unknown host: %+v", description)
	}
	description = describeChain([]appinfo.Process{{PID: 9, Path: "/usr/local/bin/deploy-tool", Arguments: []string{"deploy-tool", "--env", "prod"}}}, sshagent.ApprovalRequest{}, nil)
	if description.Purpose != PurposeUseKey || description.CommandLine != "deploy-tool --env prod" || description.HostAndPort != "" {
		t.Errorf("other program: %+v", description)
	}
}

func createSigningChain() []appinfo.Process {
	return append([]appinfo.Process{
		{PID: 60, Path: "/usr/bin/ssh-keygen", Arguments: []string{"ssh-keygen", "-Y", "sign", "-n", "git", "-f", "/Users/developer/.ssh/signing.pub", testSigningBuffer}, WorkingDirectory: testRepositoryPath},
		createGitProcess(40, "commit", "-S", "-m", "Fix the login redirect"),
	}, createShellAndTerminal()...)
}

func TestDescribeCommitSigning(t *testing.T) {
	fileSystem := fakeFileSystem{
		testSigningBuffer:                 "tree 4b825dc642cb6eb9a060e54bf8d69288fbee4904\nparent 1111111111111111111111111111111111111111\nauthor Deniz <deniz@example.com> 1760000000 +0300\ncommitter Deniz <deniz@example.com> 1760000000 +0300\n\n  Fix the login redirect  \n\nThe callback lost its state.\n",
		testRepositoryPath + "/.git/HEAD": "ref: refs/heads/feature/login\n",
	}
	request := sshagent.ApprovalRequest{SignatureNamespace: "git"}
	description := describeChain(createSigningChain(), request, fileSystem)
	if description.Purpose != PurposeSignCommit || description.Action != "sign a commit" || description.CommitSubject != "Fix the login redirect" ||
		description.Branch != "feature/login" || description.RepositoryPath != "~/src/core" || description.HostAndPort != "" || description.CommandLine != `git commit -S -m 'Fix the login redirect'` {
		t.Errorf("commit: %+v", description)
	}

	delete(fileSystem, testSigningBuffer)
	delete(fileSystem, testRepositoryPath+"/.git/HEAD")
	description = describeChain(createSigningChain(), request, fileSystem)
	if description.Purpose != PurposeSignCommit || description.CommitSubject != "" || description.Branch != "" {
		t.Errorf("without the buffer: %+v", description)
	}

	unlockRequest := sshagent.ApprovalRequest{IsUnlockRequired: true}
	if description := describeChain(createSigningChain(), unlockRequest, fileSystem); description.Purpose != PurposeSignCommit {
		t.Errorf("unlock before signing: %+v", description)
	}
	otherNamespace := createSigningChain()
	otherNamespace[0].Arguments[4] = "file"
	if description := describeChain(otherNamespace, unlockRequest, fileSystem); description.Purpose == PurposeSignCommit {
		t.Errorf("a file signature counted as a commit: %+v", description)
	}
}

func TestDescribeTagSigningInAWorktree(t *testing.T) {
	relativeChain := createSigningChain()
	relativeChain[0].Arguments[len(relativeChain[0].Arguments)-1] = ".git_signing_buffer_tmpZ"
	fileSystem := fakeFileSystem{
		testRepositoryPath + "/.git_signing_buffer_tmpZ":       "object 1111111111111111111111111111111111111111\ntype commit\ntag v1.4.0\ntagger Deniz <deniz@example.com> 1760000000 +0300\n\nRelease 1.4.0\n",
		testRepositoryPath + "/.git":                           "gitdir: ../core.git/worktrees/release\n",
		"/Users/developer/src/core.git/worktrees/release/HEAD": "ref: refs/heads/release/1.4\n",
	}
	description := describeChain(relativeChain, sshagent.ApprovalRequest{SignatureNamespace: "git"}, fileSystem)
	if description.Purpose != PurposeSignTag || description.Action != "sign the tag" || description.Target != "v1.4.0" || description.CommitSubject != "Release 1.4.0" || description.Branch != "release/1.4" {
		t.Errorf("tag: %+v", description)
	}
	fileSystem[testRepositoryPath+"/.git"] = "not a pointer"
	if description := describeChain(relativeChain, sshagent.ApprovalRequest{SignatureNamespace: "git"}, fileSystem); description.Branch != "" {
		t.Errorf("a broken .git file gave branch %q", description.Branch)
	}
	fileSystem[testRepositoryPath+"/.git/HEAD"] = "4b825dc642cb6eb9a060e54bf8d69288fbee4904\n"
	if description := describeChain(relativeChain, sshagent.ApprovalRequest{SignatureNamespace: "git"}, fileSystem); description.Branch != "" {
		t.Errorf("a detached HEAD gave branch %q", description.Branch)
	}
}

func TestDescribeForwardedRequest(t *testing.T) {
	processes := append([]appinfo.Process{createSSHProcess("-A", "dev@build.example.com")}, createShellAndTerminal()...)
	request := sshagent.ApprovalRequest{IsForwarded: true, ForwardedFromHost: "build.example.com", ForwardedFromFingerprint: "SHA256:build", DestinationHost: "github.com", DestinationFingerprint: "SHA256:github"}
	description := describeChain(processes, request, nil)
	if description.ForwardedFromHost != "build.example.com" || description.Purpose != PurposeSignIn || description.Target != "github.com" || description.HostAndPort != "github.com:22" || description.CommandLine != "ssh -A dev@build.example.com" || description.RepositoryPath != "" {
		t.Errorf("forwarded: %+v", description)
	}
	request = sshagent.ApprovalRequest{IsForwarded: true, ForwardedFromPort: 2222, ForwardedFromHost: "build.example.com", ForwardedFromFingerprint: "SHA256:build"}
	if description := describeChain(processes, request, nil); description.ForwardedFromHost != "build.example.com:2222" || description.Purpose != PurposeUseKey {
		t.Errorf("forwarded without a hop: %+v", description)
	}
	request = sshagent.ApprovalRequest{IsForwarded: true, ForwardedFromFingerprint: "SHA256:build"}
	if description := describeChain(processes, request, nil); description.ForwardedFromHost != "build.example.com" {
		t.Errorf("forwarded host from the arguments: %+v", description)
	}
	if description := describeChain(nil, request, nil); description.ForwardedFromHost != "SHA256:build" {
		t.Errorf("forwarded host from the fingerprint: %+v", description)
	}
}

func TestFormatCommandLine(t *testing.T) {
	for _, testCase := range []struct {
		arguments []string
		want      string
	}{
		{[]string{"/usr/bin/git", "push", "origin", "main"}, "git push origin main"},
		{[]string{"git", "commit", "-m", "it's done"}, `git commit -m 'it'\''s done'`},
		{[]string{"git", "clone", "https://deniz:ghp_secret@github.com/x.git"}, "git clone https://deniz:•••@github.com/x.git"},
		{[]string{"git", "-c", "http.extraHeader=Authorization: Bearer abc", "-c", "user.name=Deniz", "fetch"}, "git -c http.extraHeader=••• -c user.name=Deniz fetch"},
		{[]string{"git", "-c", "credential.helper=store", "-c", "github.token=abc", "push"}, "git -c credential.helper=store -c github.token=••• push"},
		{[]string{"git", "commit", "-m", ""}, "git commit -m ''"},
		{[]string{"git", "commit", "-m", strings.Repeat("ğ", 300)}, "git commit -m '" + strings.Repeat("ğ", maximumCommandLineLength-len([]rune("git commit -m '"))-1) + "…"},
		{nil, ""},
	} {
		if commandLine := formatCommandLine(testCase.arguments); commandLine != testCase.want {
			t.Errorf("%q: %q, want %q", testCase.arguments, commandLine, testCase.want)
		}
	}
	if commandLine := formatCommandLine([]string{"git", "commit", "-m", "\xff\xfe"}); !strings.Contains(commandLine, "�") {
		t.Errorf("invalid UTF-8 kept: %q", commandLine)
	}
}

func TestShortenHomePath(t *testing.T) {
	for path, want := range map[string]string{
		testRepositoryPath:            "~/src/core",
		testHomeDirectory:             "~",
		"/Users/developer2/src":       "/Users/developer2/src",
		"/opt/src":                    "/opt/src",
		"":                            "",
		testHomeDirectory + "/a b/c'": "~/a b/c'",
	} {
		if shortened := shortenHomePath(path, testHomeDirectory+"/"); shortened != want {
			t.Errorf("%q: %q, want %q", path, shortened, want)
		}
	}
	if shortened := shortenHomePath(testRepositoryPath, ""); shortened != testRepositoryPath {
		t.Errorf("without a home directory: %q", shortened)
	}
}

func TestFormatApprovalReason(t *testing.T) {
	for _, testCase := range []struct {
		description RequestDescription
		want        string
	}{
		{RequestDescription{Purpose: PurposePush, Action: "push to", Target: "git.tryhisar.com"}, `allow "Terminal" to push to git.tryhisar.com`},
		{RequestDescription{Purpose: PurposeSignCommit, Action: "sign a commit"}, `allow "Terminal" to sign a commit`},
		{RequestDescription{Purpose: PurposeUseKey, Action: "use your SSH key", HostAndPort: "build.example.com:22"}, `allow "Terminal" to use your SSH key for build.example.com:22`},
		{RequestDescription{Purpose: PurposeUseKey, Action: "use your SSH key", HostAndPort: "SHA256:host"}, `allow "Terminal" to use your SSH key`},
		{RequestDescription{Purpose: PurposeSignIn, Action: "sign in to", Target: "github.com", ForwardedFromHost: "build.example.com"}, `allow "Terminal" to sign in to github.com from build.example.com through agent forwarding`},
	} {
		if reason := formatApprovalReason("Terminal", testCase.description); reason != testCase.want {
			t.Errorf("%q, want %q", reason, testCase.want)
		}
	}
}

func TestReadSmallRegularFile(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "payload")
	if err := os.WriteFile(path, []byte("tree x\n\nsubject\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if content, err := ReadSmallRegularFile(path, 64); err != nil || string(content) != "tree x\n\nsubject\n" {
		t.Errorf("small file: %q %v", content, err)
	}
	if _, err := ReadSmallRegularFile(path, 4); err == nil {
		t.Error("a file larger than the limit was read")
	}
	if _, err := ReadSmallRegularFile(directory, 64); err == nil {
		t.Error("a directory was read")
	}
	if _, err := ReadSmallRegularFile(filepath.Join(directory, "missing"), 64); err == nil {
		t.Error("a missing file was read")
	}
}
