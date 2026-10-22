package agent

import (
	"fmt"
	"time"

	"github.com/tryhisar/hisar/core/sshagent"
)

// signingScope is what a remembered commit signature covers: one live agent session (its ID), or else one app bundle,
// with one repository and one key.
type signingScope struct {
	appPath        string
	agentSession   uint64
	repositoryPath string
	fingerprint    string
}

type rememberedSigning struct {
	expiresAtByScope map[signingScope]time.Time
	epoch            uint64
}

func formatRememberDuration(duration time.Duration) string {
	if duration%time.Hour == 0 {
		hourCount := int(duration / time.Hour)
		if hourCount == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hourCount)
	}
	minuteCount := int(duration.Round(time.Minute) / time.Minute)
	if minuteCount == 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", minuteCount)
}

func (approver *Approver) getSigningRememberDuration() time.Duration {
	if approver.config.GetSigningRememberDuration == nil {
		return 0
	}
	return max(approver.config.GetSigningRememberDuration(), 0)
}

// findSigningScope returns the scope of a commit signature in a known repository from an agent session or else from a
// resolved app; tags, forwarded requests, keys that always ask, requests never remembered and requests without an app
// or repository have none.
func findSigningScope(event ApprovalEvent, request sshagent.ApprovalRequest) (signingScope, bool) {
	if event.Purpose != PurposeSignCommit || event.IsForwarded || request.IsForwarded || request.IsAlwaysAskKey || request.IsNeverRemembered {
		return signingScope{}, false
	}
	if event.RepositoryPath == "" || request.Fingerprint == "" {
		return signingScope{}, false
	}
	if live := request.AgentSession; live != nil {
		return signingScope{agentSession: live.ID, repositoryPath: event.RepositoryPath, fingerprint: request.Fingerprint}, true
	}
	if !isAppRequester(event) || event.AppPath == "" {
		return signingScope{}, false
	}
	return signingScope{appPath: event.AppPath, repositoryPath: event.RepositoryPath, fingerprint: request.Fingerprint}, true
}

func (approver *Approver) isSigningRemembered(scope signingScope) bool {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	expiresAt, isFound := approver.signing.expiresAtByScope[scope]
	return isFound && approver.config.Now().Before(expiresAt)
}

func (approver *Approver) getSigningEpoch() uint64 {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	return approver.signing.epoch
}

// rememberSigning keeps an approved signature's scope unless Hisar locked or the setting changed since it was asked for.
func (approver *Approver) rememberSigning(scope signingScope, duration time.Duration, epoch uint64) {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	if approver.signing.epoch != epoch || duration <= 0 {
		return
	}
	if approver.signing.expiresAtByScope == nil {
		approver.signing.expiresAtByScope = map[signingScope]time.Time{}
	}
	approver.signing.expiresAtByScope[scope] = approver.config.Now().Add(duration)
}

// ClearRememberedSignings forgets every remembered commit signature; Hisar calls it when it locks and when the setting changes.
func (approver *Approver) ClearRememberedSignings() {
	approver.mutex.Lock()
	defer approver.mutex.Unlock()
	approver.signing.expiresAtByScope = nil
	approver.signing.epoch++
}

// offerSigningMemory adds the unchecked "Don't ask again" checkbox to a commit signature while the setting allows it.
func offerSigningMemory(event *ApprovalEvent, duration time.Duration) {
	event.IsRememberable = true
	event.IsRememberedByDefault = false
	event.RememberLabel = fmt.Sprintf("Don’t ask again for %s in %s for %s", event.AppName, event.RepositoryPath, formatRememberDuration(duration))
}
