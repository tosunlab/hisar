// Package mcpserver is hisar mcp: a Model Context Protocol server that lets coding agents use secrets in Hisar
// without seeing them. Every request goes to the Hisar app, which decides what the agent may see and asks the user.
package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tryhisar/hisar/cli/internal/masking"
	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/secretreference"
)

const instructions = `Hisar is the user's password and secrets manager. Use secrets without seeing them: find their hisar:// references with search_items, then pass them to run_with_secrets, which runs a command with the secrets in its environment, env files or stdin and masks them in the output. For a command that needs live output, interactive input or your own shell's environment, run NAME=hisar://vault/item/field hisar run -- <command> (or hisar run --env-file .env -- <command>) in your shell instead. Never print a secret, and never ask the user to paste one into the chat. Only the vaults the user made available to agents are visible. The user approves each use of a secret in Hisar; if they deny it, tell them and do not retry the same request.`

const revealInstructions = ` get_secret reveals a value and is only for when the value itself is needed.`

const noAgentVaultMessage = "No vault is available to coding agents. Ask the user to open Hisar → Settings → Developer tools → Coding agents and turn on “Available to agents” for the vaults you may use."

type Config struct {
	Version string
	Dial    AppDialer
	// IsRevealDisabled leaves out get_secret and get_one_time_code, the tools that return values.
	IsRevealDisabled bool
	// Environment is what commands start with; nil uses this process's environment.
	Environment []string
	// WorkingDirectory is where commands run by default; empty uses this process's directory.
	WorkingDirectory string
	Now              func() time.Time
	KillDelay        time.Duration
}

type toolServer struct {
	config  Config
	session *appSession
}

type ListVaultsInput struct{}

type ListVaultsOutput struct {
	Vaults  []cliprotocol.AgentVault `json:"vaults" jsonschema:"the vaults available to agents"`
	Message string                   `json:"message,omitempty"`
}

type SearchItemsInput struct {
	Query string `json:"query" jsonschema:"words to find in the title, username, URLs or vault name, ignoring case; empty lists every item"`
	Vault string `json:"vault,omitempty" jsonschema:"only search this vault, by name or ID"`
}

type SearchItemsOutput struct {
	Items      []cliprotocol.AgentItem `json:"items" jsonschema:"matching items: metadata and secret references, never values or notes"`
	TotalCount int                     `json:"totalCount" jsonschema:"how many items match; at most 50 are returned"`
	Message    string                  `json:"message,omitempty"`
}

type OneTimeCodeInput struct {
	Item string `json:"item" jsonschema:"a hisar:// reference to any field of the item, or the item's ID from search_items"`
}

type OneTimeCodeOutput struct {
	Code             string `json:"code"`
	SecondsRemaining int    `json:"secondsRemaining" jsonschema:"how long the code stays valid"`
	Period           int    `json:"period" jsonschema:"seconds between codes"`
}

type GetSecretInput struct {
	Reference string `json:"reference" jsonschema:"the hisar://vault/item/field reference of the secret"`
}

type GetSecretOutput struct {
	Value string `json:"value"`
}

func isTrue() *bool {
	value := true
	return &value
}

// NewServer returns the hisar MCP server; Close ends its connection to the app.
func NewServer(config Config) (*mcp.Server, func()) {
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.KillDelay == 0 {
		config.KillDelay = defaultKillDelay
	}
	server := &toolServer{config: config, session: &appSession{dial: config.Dial}}
	serverInstructions, runDescriptionEnd := instructions, "Secret values are never returned."
	if !config.IsRevealDisabled {
		serverInstructions, runDescriptionEnd = instructions+revealInstructions, "Secret values are never returned; prefer this tool over get_secret."
	}
	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "hisar", Title: "Hisar", Version: config.Version}, &mcp.ServerOptions{Instructions: serverInstructions})
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:        "list_vaults",
		Title:       "List vaults",
		Description: "List the Hisar vaults the user made available to coding agents: id, name and number of items. Other vaults are invisible to agents. Returns no secrets.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(bool)},
	}, server.listVaults)
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:  "search_items",
		Title: "Search items",
		Description: "Find items in the vaults available to agents by title, username, URL or vault name. Returns metadata only: id, vault, title, type, username, URLs, " +
			"whether notes or a one-time code exist, and each field's label with its hisar:// secret reference. Secret values and notes are never returned. " +
			"Pass the references to run_with_secrets to use the secrets without seeing them.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(bool)},
	}, server.searchItems)
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:  "run_with_secrets",
		Title: "Run a command with secrets",
		Description: "Run a command with secrets from Hisar, without seeing them. Pass command, the program and its arguments, which runs without a shell, or shell, " +
			"a command line for /bin/sh -c (pipes, redirects, &&). The secrets come from env, which maps environment variable names to hisar://vault/item/field " +
			"references from search_items; from envFiles, dotenv files whose hisar:// values are resolved as hisar run --env-file does; and from stdin, which is " +
			"literal text or one hisar:// reference whose value is sent, e.g. for --password-stdin. Without stdin the command reads nothing. The user approves each " +
			"run in Hisar, seeing the command, the working directory and every secret. When direnv is installed and the user allowed an .envrc in or above the " +
			"working directory, the command runs through direnv exec, so it gets the project's environment as in the user's shell. Every secret value in stdout " +
			"and stderr is replaced with <concealed by hisar>, and each stream keeps at most 64 KiB; with a progressToken, the masked output also arrives as " +
			"progress notifications while the command runs. Returns the exit code, the masked output and the duration. " + runDescriptionEnd,
		Annotations: &mcp.ToolAnnotations{DestructiveHint: isTrue(), OpenWorldHint: isTrue()},
	}, server.runWithSecrets)
	if config.IsRevealDisabled {
		return mcpServer, server.session.close
	}
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:  "get_one_time_code",
		Title: "Get a one-time code",
		Description: "Get the current one-time code (TOTP) of an item, for example to finish a sign-in, after the user approves it in Hisar. item is a hisar:// " +
			"reference to any field of the item, or the item's ID from search_items. Returns the code and how many seconds it stays valid.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(bool)},
	}, server.getOneTimeCode)
	mcp.AddTool(mcpServer, &mcp.Tool{
		Name:  "get_secret",
		Title: "Reveal a secret",
		Description: "Reveal one secret value to you. This is the only tool that returns a secret value, and every call needs the user's approval for this item in " +
			"Hisar, which warns them that the value becomes visible to you. Use it only when the value itself must be shown or entered somewhere " +
			"run_with_secrets cannot reach; otherwise use run_with_secrets. Fails with \"The user denied access in Hisar.\" when they decline. " +
			"SSH private keys are never returned; ssh and git use them through the Hisar SSH agent.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: new(bool)},
	}, server.getSecret)
	return mcpServer, server.session.close
}

func getIdentity(request *mcp.CallToolRequest) cliprotocol.Request {
	identity := cliprotocol.Request{}
	if clientInfo := request.ClientInfo(); clientInfo != nil {
		identity.AgentName, identity.AgentTitle, identity.AgentVersion = clientInfo.Name, clientInfo.Title, clientInfo.Version
	}
	return identity
}

func (server *toolServer) call(ctx context.Context, request *mcp.CallToolRequest, appRequest cliprotocol.Request, result any) error {
	return server.session.call(ctx, getIdentity(request), appRequest, result)
}

func (server *toolServer) listVaults(ctx context.Context, request *mcp.CallToolRequest, _ ListVaultsInput) (*mcp.CallToolResult, ListVaultsOutput, error) {
	var result cliprotocol.AgentVaultListResult
	if err := server.call(ctx, request, cliprotocol.Request{Type: cliprotocol.TypeAgentListVault}, &result); err != nil {
		return nil, ListVaultsOutput{}, err
	}
	output := ListVaultsOutput{Vaults: result.Vaults}
	if output.Vaults == nil {
		output.Vaults = []cliprotocol.AgentVault{}
	}
	if len(output.Vaults) == 0 {
		output.Message = noAgentVaultMessage
	}
	return nil, output, nil
}

func (server *toolServer) searchItems(ctx context.Context, request *mcp.CallToolRequest, input SearchItemsInput) (*mcp.CallToolResult, SearchItemsOutput, error) {
	var result cliprotocol.AgentItemSearchResult
	if err := server.call(ctx, request, cliprotocol.Request{Type: cliprotocol.TypeAgentSearchItem, Query: input.Query, Vault: input.Vault}, &result); err != nil {
		return nil, SearchItemsOutput{}, err
	}
	output := SearchItemsOutput{Items: result.Items, TotalCount: result.TotalCount}
	if output.Items == nil {
		output.Items = []cliprotocol.AgentItem{}
	}
	if output.TotalCount == 0 {
		var vaultList cliprotocol.AgentVaultListResult
		if err := server.call(ctx, request, cliprotocol.Request{Type: cliprotocol.TypeAgentListVault}, &vaultList); err == nil && len(vaultList.Vaults) == 0 {
			output.Message = noAgentVaultMessage
		}
	}
	return nil, output, nil
}

func (server *toolServer) getWorkingDirectory() string {
	if server.config.WorkingDirectory != "" {
		return server.config.WorkingDirectory
	}
	directory, err := os.Getwd()
	if err != nil {
		return "."
	}
	return directory
}

func (server *toolServer) getEnvironment() []string {
	if server.config.Environment != nil {
		return server.config.Environment
	}
	return os.Environ()
}

func (server *toolServer) runWithSecrets(ctx context.Context, request *mcp.CallToolRequest, input RunInput) (*mcp.CallToolResult, RunOutput, error) {
	prepared, err := prepareCommand(input, server.getWorkingDirectory(), server.getEnvironment())
	if err != nil {
		return nil, RunOutput{}, err
	}
	progress := newProgressReporter(request)
	progress.report(ctx, "Waiting for approval in Hisar")
	var result cliprotocol.ResolveSecretResult
	appRequest := cliprotocol.Request{Type: cliprotocol.TypeAgentResolveSecret, References: prepared.references, CommandLine: prepared.arguments, WorkingDirectory: prepared.workingDirectory}
	if err := server.call(ctx, request, appRequest, &result); err != nil {
		return nil, RunOutput{}, err
	}
	if len(result.Values) != len(prepared.references) {
		return nil, RunOutput{}, errors.New("the Hisar app answered with the wrong number of values")
	}
	valueByReference := map[string]string{}
	for index, reference := range prepared.references {
		valueByReference[reference] = result.Values[index]
	}
	var warnings []string
	useSecret := func(name, reference string) (string, bool) {
		value, isSecret := valueByReference[reference]
		if isSecret && len(value) < masking.MinimumLength {
			warnings = append(warnings, fmt.Sprintf("%s is shorter than %d bytes and is not masked", name, masking.MinimumLength))
		}
		return value, isSecret
	}
	environment := make([]string, 0, len(prepared.variables))
	for _, variable := range prepared.variables {
		value, isSecret := useSecret(variable.Name, variable.Value)
		if !isSecret {
			value = variable.Value
		}
		environment = append(environment, variable.Name+"="+value)
	}
	stdin, isSecret := useSecret("stdin", prepared.stdin)
	if !isSecret {
		stdin = prepared.stdin
	}
	output, err := server.run(ctx, prepared, environment, stdin, result.Values, progress)
	output.Warnings = warnings
	return nil, output, err
}

func (server *toolServer) getOneTimeCode(ctx context.Context, request *mcp.CallToolRequest, input OneTimeCodeInput) (*mcp.CallToolResult, OneTimeCodeOutput, error) {
	appRequest := cliprotocol.Request{Type: cliprotocol.TypeAgentGetCode, ItemID: input.Item}
	if secretreference.IsReference(input.Item) {
		if _, err := secretreference.Parse(input.Item); err != nil {
			return nil, OneTimeCodeOutput{}, fmt.Errorf("%s: %v", input.Item, err)
		}
		appRequest.ItemID, appRequest.References = "", []string{input.Item}
	}
	if input.Item == "" {
		return nil, OneTimeCodeOutput{}, errors.New("item is empty; pass a hisar:// reference or an item ID")
	}
	var result cliprotocol.CodeResult
	if err := server.call(ctx, request, appRequest, &result); err != nil {
		return nil, OneTimeCodeOutput{}, err
	}
	expiresAt, err := time.Parse(time.RFC3339, result.ExpiresAt)
	if err != nil {
		return nil, OneTimeCodeOutput{}, errors.New("the Hisar app answered with an invalid expiry")
	}
	secondsRemaining := int(math.Ceil(expiresAt.Sub(server.config.Now()).Seconds()))
	return nil, OneTimeCodeOutput{Code: result.Code, SecondsRemaining: max(secondsRemaining, 0), Period: result.Period}, nil
}

func (server *toolServer) getSecret(ctx context.Context, request *mcp.CallToolRequest, input GetSecretInput) (*mcp.CallToolResult, GetSecretOutput, error) {
	if _, err := secretreference.Parse(input.Reference); err != nil {
		return nil, GetSecretOutput{}, fmt.Errorf("%q: %v", input.Reference, err)
	}
	var result cliprotocol.ResolveSecretResult
	if err := server.call(ctx, request, cliprotocol.Request{Type: cliprotocol.TypeAgentRevealSecret, References: []string{input.Reference}}, &result); err != nil {
		return nil, GetSecretOutput{}, err
	}
	if len(result.Values) != 1 {
		return nil, GetSecretOutput{}, errors.New("the Hisar app answered with the wrong number of values")
	}
	return nil, GetSecretOutput{Value: result.Values[0]}, nil
}
