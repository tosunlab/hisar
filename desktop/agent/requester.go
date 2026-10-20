package agent

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tryhisar/hisar/desktop/appinfo"
)

const maximumPhoneIconSize = 6 << 10

// ChainProcess is one program between the requester and its app, as the dialog lists it.
type ChainProcess struct {
	Name        string `json:"name"`
	CommandLine string `json:"commandLine"`
}

func (approver *Approver) resolveRequester(pid int) (appinfo.Requester, bool) {
	if approver.config.ResolveRequester == nil || pid <= 0 {
		return appinfo.Requester{}, false
	}
	return approver.config.ResolveRequester(pid), true
}

func buildChainProcesses(processes []appinfo.Process) []ChainProcess {
	chainProcesses := []ChainProcess{}
	for _, process := range processes {
		name := appinfo.GetProgramName(process)
		if name == "" {
			continue
		}
		commandLine := formatShortCommandLine(process.Arguments)
		if commandLine == "" {
			commandLine = process.Path
		}
		chainProcesses = append(chainProcesses, ChainProcess{Name: truncateText(name, maximumAgentNameLength), CommandLine: commandLine})
	}
	return chainProcesses
}

// describeAppSignature says who signed an app; the warning is for an app macOS cannot vouch for.
func describeAppSignature(app appinfo.App) (string, string) {
	signature := app.Signature
	switch signature.Status {
	case appinfo.SignatureStatusApple:
		return "Signed by Apple", ""
	case appinfo.SignatureStatusDeveloper:
		signerName := signature.SignerName
		if signerName == "" {
			signerName = "Team ID " + signature.TeamID
		} else if signature.TeamID != "" && !strings.Contains(signerName, signature.TeamID) {
			signerName += " (" + signature.TeamID + ")"
		}
		return "Signed by " + signerName, ""
	case appinfo.SignatureStatusUnsigned:
		return "Not signed", fmt.Sprintf("%s is not signed, so macOS cannot confirm who made it. Approve only if you started this yourself.", app.Name)
	case appinfo.SignatureStatusInvalid:
		return "Signature not valid", fmt.Sprintf("The code signature of %s does not verify: the app may have been modified. Approve only if you started this yourself.", app.Name)
	}
	return "", ""
}

func applyRequester(event *ApprovalEvent, requester appinfo.Requester) {
	event.RequesterKind = requester.Kind
	event.AppName = requester.Name
	event.ChainProcesses = buildChainProcesses(requester.ChainProcesses)
	if requester.Kind != appinfo.RequesterKindApp {
		return
	}
	event.AppPath, event.AppIconURL = requester.App.Path, requester.App.IconURL
	event.AppSignature = requester.App.Signature.Status
	event.AppSigner, event.AppWarning = describeAppSignature(requester.App)
}

func isAppRequester(event ApprovalEvent) bool {
	return event.RequesterKind == "" || event.RequesterKind == appinfo.RequesterKindApp
}

func lowerFirstLetter(text string) string {
	first, size := utf8.DecodeRuneInString(text)
	if size == 0 || (len(text) > size && unicode.IsUpper(rune(text[size]))) {
		return text
	}
	return string(unicode.ToLower(first)) + text[size:]
}

// describeInlineAppName is the app inside a sentence: "iTerm2", or "a background process".
func describeInlineAppName(event ApprovalEvent) string {
	if isAppRequester(event) {
		return event.AppName
	}
	return lowerFirstLetter(event.AppName)
}

// describeReasonRequester names the requester for Touch ID: the app, or the program and what it runs in.
func describeReasonRequester(event ApprovalEvent) string {
	if isAppRequester(event) || event.ClientName == "" {
		return event.AppName
	}
	return event.ClientName + " in " + describeInlineAppName(event)
}

// getPhoneAppIcon is the app's small icon for the phone, left out when it would crowd the sealed request.
func (approver *Approver) getPhoneAppIcon(event ApprovalEvent) []byte {
	if approver.config.GetAppIconPNG == nil || event.AppPath == "" || event.IsForwarded {
		return nil
	}
	icon, err := approver.config.GetAppIconPNG(event.AppPath, appinfo.PhoneIconSize)
	if err != nil || len(icon) > maximumPhoneIconSize {
		return nil
	}
	return icon
}
