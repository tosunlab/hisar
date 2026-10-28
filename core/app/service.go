package app

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cache"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/itemicon"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/session"
	"github.com/tryhisar/hisar/core/signin"
)

// Service may be a request-scoped view; callbacks and goroutines that outlive a request must retain only its shared state.
type Service struct {
	*serviceState
	requestEpoch        *uint64
	requestBrowserToken string
}

// serviceState is shared by the app and request-scoped CLI views, including their mutex.
type serviceState struct {
	mutex sync.Mutex
	// biometricMutex owns the platform's single-use authentication context, never logical session locking.
	biometricMutex  sync.Mutex
	config          Config
	state           *localstate.State
	unlockedSession *session.Session
	// sessionEpoch advances on session end and explicit lock intent, even while already locked.
	sessionEpoch                 uint64
	recoveryMutex                sync.Mutex
	pendingRecoveryFactors       map[string]session.PendingFactor
	recoveryRecords              map[string]PendingRecoverySummary
	passkeyEpoch                 atomic.Uint64
	vaultsByID                   map[string]*session.Vault
	itemsByVaultID               map[string][]item.Item
	vaultReadIssues              []session.ReadIssue
	itemReadIssuesByVaultID      map[string][]session.ReadIssue
	itemProvenanceByVaultID      map[string]session.ReadProvenance
	favorites                    *session.Preferences
	lockTimer                    *time.Timer
	lockDeadline                 time.Duration
	clipboardClearTimer          *time.Timer
	clearCopiedText              func()
	name                         string
	hasPasswordUnlockSinceLaunch bool
	biometricClock               biometricPasswordClock
	biometricSetupError          string
	biometricSetupRevision       uint64
	kitGrant                     *kitGrant
	pendingKitExport             *pendingKitExport
	isKitReminderHidden          bool
	localCache                   *cache.Cache
	pendingImport                *pendingImport
	stopEvents                   context.CancelFunc
	reconnectEvents              chan struct{}
	onSessionChange              func(server, token string, isSignedIn bool)
	shareCompletionError         string
	vaultKeyDeliveryError        string
	vaultRotationError           string
	unsavedPinPasses             completionPasses
	lockListeners                []func()
	statusListeners              []func()
	phoneApprovals               *phoneApprovalRelay
	signedOutReason              string
	updateRequirement            updateRequirementState
	websiteIcons                 websiteIcons
	defaultWebsiteIconFetcher    WebsiteIconFetcher
	serverTransport              http.RoundTripper
	// commandLineApprovals are the CLI socket's approvals, set by NewCommandLineBridge; nil where Hisar has no socket.
	commandLineApprovals *clisocket.SessionApprovals
	// agentGrants are the harness grants opened from state.SealedAgentGrants while Hisar is unlocked; nil while locked.
	agentGrants                 []clisocket.AgentGrant
	isAgentGrantHeadUnavailable bool
	lockedGrants                *lockedGrantView
	lockedGrantGeneration       atomic.Uint64
	signer                      *signin.Signer
	agentSignInCancels          map[uint64]context.CancelFunc
	lastAgentSignInID           uint64
	// pushPlatform and pushToken are what an iPhone's OS gave for push notifications at this launch, pushPayloadKey
	// the key its notification service extension opens push details with, and webPush the subscription an Android
	// phone's UnifiedPush distributor gave; signedPushKey is the message key last registered while Hisar could sign
	// it. Never saved.
	pushPlatform        apitypes.PushPlatform
	pushToken           string
	pushPayloadKey      *apitypes.PushPayloadKey
	webPush             *apitypes.WebPushSubscription
	signedPushKey       string
	isPushServerSending bool
	pushErrorMessage    string
}

// getCache opens the offline cache on first use and recreates it only after confirmed corruption.
func (s *Service) getCache() session.Cache {
	if s.config.CachePath == "" {
		return nil
	}
	if s.localCache == nil {
		openedCache, err := cache.Open(s.config.CachePath)
		if err != nil {
			if !cache.IsCorrupt(err) {
				log.Printf("offline cache could not be opened; keeping the existing file: %v", err)
				return nil
			}
			if err := cache.Remove(s.config.CachePath); err != nil {
				log.Printf("corrupt offline cache could not be deleted: %v", err)
				return nil
			}
			if openedCache, err = cache.Open(s.config.CachePath); err != nil {
				log.Printf("offline cache could not be recreated: %v", err)
				return nil
			}
		}
		if err := openedCache.CheckAccount(s.state.AccountID); err != nil {
			openedCache.Close()
			log.Printf("offline cache cannot be used by this account; keeping the existing file: %v", err)
			return nil
		}
		s.localCache = openedCache
	}
	return s.localCache
}

func (s *Service) removeCache() error {
	if s.localCache != nil {
		s.localCache.Close()
		s.localCache = nil
	}
	if s.config.CachePath == "" {
		return nil
	}
	if err := cache.Remove(s.config.CachePath); err != nil {
		return fmt.Errorf("this device's offline copy of the vaults could not be deleted: %w", err)
	}
	return nil
}

func (s *Service) attachCache(ctx context.Context, unlockedSession *session.Session) error {
	if localCache := s.getCache(); localCache != nil {
		if err := unlockedSession.AttachCache(ctx, localCache); err != nil {
			return fmt.Errorf("this device's offline copy of the account could not be refreshed: %w", err)
		}
	} else if s.config.CachePath != "" {
		return fmt.Errorf("this device's offline copy of the vaults could not be opened")
	}
	return nil
}

func NewService(config Config) (*Service, error) {
	if config.AutoLockAfter == 0 {
		config.AutoLockAfter = 10 * time.Minute
	}
	if config.ClipboardClearAfter == 0 {
		config.ClipboardClearAfter = 90 * time.Second
	}
	if config.SecretKeyStore == nil {
		config.SecretKeyStore = KeychainSecretKeyStore{}
	}
	if config.BiometricPasswordInterval == 0 {
		config.BiometricPasswordInterval = 14 * 24 * time.Hour
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.SleepCountingClock == nil {
		config.SleepCountingClock = readSleepCountingClock
	}
	state, err := localstate.Load(config.StatePath)
	if err != nil {
		return nil, err
	}
	service := &Service{serviceState: &serviceState{config: config, state: state, phoneApprovals: createPhoneApprovalRelay()}}
	if config.SignInDriver != nil {
		homeDirectory, _ := os.UserHomeDir()
		service.signer = signin.NewSigner(config.SignInDriver, homeDirectory, config.Now)
	}
	return service, nil
}

// SetTrustedCertificates makes requests to the Hisar server and website icon fetches trust only roots, for platforms
// whose trust store Go cannot read (Android). It must run right after NewService; not a method, so the UI cannot call it.
func SetTrustedCertificates(s *Service, roots *x509.CertPool) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: roots}
	s.mutex.Lock()
	s.serverTransport = transport
	s.mutex.Unlock()
	s.websiteIcons.mutex.Lock()
	s.defaultWebsiteIconFetcher = itemicon.NewFetcher(roots)
	s.websiteIcons.mutex.Unlock()
}

// notifySessionChange runs with the mutex held, after a sign-in, a sign-up or a sign-out.
func (s *Service) notifySessionChange() {
	if s.onSessionChange != nil {
		s.onSessionChange(s.state.Server, s.state.Token, s.state.IsLoggedIn())
	}
	// A phone that already has a push token or subscription registers it for the account it just signed in to.
	if s.state.IsLoggedIn() && s.hasPushTarget() {
		go s.registerPush()
	}
}

func (s *Service) getDeviceInfo() apitypes.DeviceInfo {
	return apitypes.DeviceInfo{Name: s.config.DeviceName, Platform: s.config.Platform}
}

func (s *Service) buildStatus() Status {
	status := Status{State: StateSignedOut, Server: s.state.Server, Email: s.state.Email, Name: s.name}
	if !s.state.IsLoggedIn() {
		status.SignedOutReason = s.signedOutReason
		if status.Server == "" {
			// Signed out, the sign-in form starts from the server this device last used, never only the default one.
			status.Server = s.state.LastServer
		}
	}
	switch {
	case s.isUnlocked():
		status.State = StateUnlocked
		status.IsOffline = s.unlockedSession.IsOffline()
	case s.state.IsLoggedIn():
		status.State = StateLocked
	}
	status.BiometricAvailable = s.config.Biometric != nil && s.config.Biometric.IsAvailable()
	status.BiometricEnabled = len(s.state.BiometricUnlock) > 0
	status.BiometricReady = status.State == StateLocked && s.isBiometricReady()
	status.BiometricSetupError = s.biometricSetupError
	if status.BiometricEnabled && status.BiometricSetupError == "" {
		if migrator, ok := s.config.Biometric.(BiometricKeyMigrator); ok && !migrator.IsKeyProtected(s.state.AccountID) {
			status.BiometricSetupError = fmt.Sprintf("%s needs a password unlock to update its keychain protection. Your master password still unlocks Hisar.", s.config.BiometricName)
		}
	}
	if owed := s.owedEmergencyKit(); owed != nil {
		status.EmergencyKit = &EmergencyKitStatus{
			OperationID:      owed.OperationID,
			Kind:             owed.Kind,
			SavedBy:          owed.SavedBy,
			PostponedAt:      owed.PostponedAt,
			Reminder:         getEmergencyKitReminder(owed.PostponedAt, s.config.Now()),
			IsAuthorized:     status.State == StateUnlocked && s.kitGrant != nil && s.kitGrant.secretKey != nil && s.kitGrant.operationID == owed.OperationID,
			IsReminderHidden: s.isKitReminderHidden,
		}
	}
	status.BiometricOfferAvailable = status.State == StateUnlocked && s.kitGrant != nil && len(s.kitGrant.unlockKey) > 0 && s.isBiometricOfferPossible()
	status.AutoLockSeconds = int(s.getAutoLockAfter() / time.Second)
	status.UpdateRequired = s.getUpdateRequirement()
	return status
}

func (s *Service) GetStatus() Status {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.buildStatus()
}

func (status Status) isSignedIn() bool {
	return status.State != StateSignedOut
}
