package app

import (
	"context"
	"sync"
	"time"

	"github.com/tryhisar/hisar/core/apitypes"
	"github.com/tryhisar/hisar/core/cache"
	"github.com/tryhisar/hisar/core/crypt"
	"github.com/tryhisar/hisar/core/item"
	"github.com/tryhisar/hisar/core/localstate"
	"github.com/tryhisar/hisar/core/session"
)

type Service struct {
	mutex                        sync.Mutex
	config                       Config
	state                        *localstate.State
	unlockedSession              *session.Session
	vaultsByID                   map[string]*session.Vault
	itemsByVaultID               map[string][]item.Item
	lockTimer                    *time.Timer
	clipboardClearTimer          *time.Timer
	name                         string
	hasPasswordUnlockSinceLaunch bool
	newAccountSecretKey          *crypt.SecretKey
	newAccountUnlockKey          []byte
	localCache                   *cache.Cache
	pendingImport                *pendingImport
	stopEvents                   context.CancelFunc
	reconnectEvents              chan struct{}
	onSessionChange              func(server, token string, isSignedIn bool)
	shareCompletionError         string
	lockListeners                []func()
	statusListeners              []func()
	phoneApprovals               *phoneApprovalRelay
	signedOutReason              string
	updateRequirement            updateRequirementState
}

// getCache opens the offline cache on first use; if it cannot be opened, even after starting it over, Hisar stays online-only.
func (s *Service) getCache() session.Cache {
	if s.config.CachePath == "" {
		return nil
	}
	if s.localCache == nil {
		openedCache, err := cache.Open(s.config.CachePath)
		if err != nil {
			cache.Remove(s.config.CachePath)
			if openedCache, err = cache.Open(s.config.CachePath); err != nil {
				return nil
			}
		}
		s.localCache = openedCache
	}
	return s.localCache
}

func (s *Service) removeCache() {
	if s.localCache != nil {
		s.localCache.Close()
		s.localCache = nil
	}
	if s.config.CachePath != "" {
		cache.Remove(s.config.CachePath)
	}
}

func (s *Service) attachCache(ctx context.Context, unlockedSession *session.Session) {
	if localCache := s.getCache(); localCache != nil {
		unlockedSession.AttachCache(ctx, localCache)
	}
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
	state, err := localstate.Load(config.StatePath)
	if err != nil {
		return nil, err
	}
	return &Service{config: config, state: state, phoneApprovals: createPhoneApprovalRelay()}, nil
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
	}
	switch {
	case s.unlockedSession != nil:
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
