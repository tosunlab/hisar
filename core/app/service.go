package app

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cache"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/itemicon"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/session"
)

// Service may be a request-scoped view; callbacks and goroutines that outlive a request must retain only its shared state.
type Service struct {
	*serviceState
	requestEpoch *uint64
}

// serviceState is shared by the app and request-scoped CLI views, including their mutex.
type serviceState struct {
	mutex           sync.Mutex
	config          Config
	state           *localstate.State
	unlockedSession *session.Session
	// sessionEpoch advances on session end and explicit lock intent, even while already locked.
	sessionEpoch                 uint64
	recoveryMutex                sync.Mutex
	pendingRecoveryFactors       map[string]session.PendingFactor
	vaultsByID                   map[string]*session.Vault
	itemsByVaultID               map[string][]item.Item
	favorites                    *session.Preferences
	lockTimer                    *time.Timer
	lockDeadline                 time.Duration
	clipboardClearTimer          *time.Timer
	clearCopiedText              func()
	name                         string
	hasPasswordUnlockSinceLaunch bool
	biometricClock               biometricPasswordClock
	newAccountSecretKey          *crypt.SecretKey
	newAccountUnlockKey          []byte
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
	return &Service{serviceState: &serviceState{config: config, state: state, phoneApprovals: createPhoneApprovalRelay()}}, nil
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

func (s *Service) notifySessionChange() {
	if s.onSessionChange != nil {
		s.onSessionChange(s.state.Server, s.state.Token, s.state.IsLoggedIn())
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
