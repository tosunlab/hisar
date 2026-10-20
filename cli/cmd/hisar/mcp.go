package main

import (
	"context"
	"errors"
	"flag"
	"io"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tryhisar/hisar/cli/mcpserver"
	"github.com/tryhisar/hisar/core/cliprotocol"
)

var errMCPNeedsApp = errors.New("hisar mcp works only through the Hisar app, which asks you before an agent uses a secret; unset HISAR_STANDALONE and leave out --standalone")

func dialAppForAgent(context.Context) (mcpserver.AppConnection, error) {
	socketPath, err := cliprotocol.GetSocketPath()
	if err != nil {
		return nil, err
	}
	client, err := dialApp(socketPath)
	if err != nil {
		return nil, err
	}
	return mcpserver.NewProtocolConnection(client.protocolClient, client.hello, appRequestTimeout), nil
}

// runMCP serves MCP on stdin and stdout until the agent closes stdin; stdout carries only protocol messages.
func runMCP(ctx context.Context, flags *flag.FlagSet, args []string) error {
	isRevealDisabled := flags.Bool("no-reveal", false, "leave out get_secret and get_one_time_code, the tools that return values")
	positionals, err := parse(flags, args)
	if err != nil {
		return err
	}
	if len(positionals) != 0 {
		return errUsage
	}
	if isStandaloneForcedByOption || isStandaloneRequested() {
		return errMCPNeedsApp
	}
	server, closeApp := mcpserver.NewServer(mcpserver.Config{Version: version, Dial: dialAppForAgent, IsRevealDisabled: *isRevealDisabled})
	defer closeApp()
	err = server.Run(ctx, &mcp.StdioTransport{})
	if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
