package app

import (
	"context"
	"strings"
	"testing"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/signin"
)

type arrivalEpochBridge struct {
	*CommandLineBridge
	onBind func()
}

func (bridge *arrivalEpochBridge) BindSession(ctx context.Context) context.Context {
	ctx = bridge.CommandLineBridge.BindSession(ctx)
	if hook := bridge.onBind; hook != nil {
		bridge.onBind = nil
		hook()
	}
	return ctx
}

func unlockAfterEpochEnds(t *testing.T, service *Service) {
	t.Helper()
	service.Lock()
	if _, err := service.Unlock("master password"); err != nil {
		t.Error(err)
	}
}

func TestAgentSecretCannotCrossArrivalEpoch(t *testing.T) {
	for _, kind := range []string{cliprotocol.TypeResolveSecret, cliprotocol.TypeAgentResolveSecret} {
		for _, duration := range []clisocket.AgentGrantDuration{clisocket.AgentGrantOnce, clisocket.AgentGrantAlways} {
			t.Run(kind+"/"+string(duration), func(t *testing.T) {
				fixture := createAgentSessionTest(t, duration)
				service := fixture.testService.service
				prepareTestApprovalUnlock(t, fixture.testService, "master password")
				bridge := &arrivalEpochBridge{CommandLineBridge: fixture.bridge}
				fixture.server = clisocket.NewServer(clisocket.ServerConfig{
					Handler: bridge, Approver: fixture.commandLineApprover, AgentApprover: fixture.agentApprover,
					Approvals: fixture.approvals, AgentHosts: fixture.bridge, PeerResolver: fixture.resolver,
				})
				connection := fixture.connect(t)
				if kind == cliprotocol.TypeAgentResolveSecret {
					connection.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeAgentHello, AgentName: "omp"}, nil)
				}
				bridge.onBind = func() { unlockAfterEpochEnds(t, service) }
				var result cliprotocol.ResolveSecretResult
				failure := connection.send(t, cliprotocol.Request{
					Type: kind, References: []string{deployTokenReference}, CommandLine: []string{"make", "deploy"},
				}, &result)
				if !strings.HasPrefix(failure, "denied: ") || len(result.Values) != 0 {
					t.Errorf("ended arrival epoch returned failure=%q values=%q", failure, result.Values)
				}
				assertNoSavedAgentGrants(t, service)
			})
		}
	}
}

func TestRelockDuringLockedAgentApprovalDenies(t *testing.T) {
	for _, kind := range []string{cliprotocol.TypeResolveSecret, cliprotocol.TypeAgentResolveSecret} {
		for _, duration := range []clisocket.AgentGrantDuration{clisocket.AgentGrantOnce, clisocket.AgentGrantAlways} {
			t.Run(kind+"/"+string(duration), func(t *testing.T) {
				fixture := createAgentSessionTest(t, duration)
				service := fixture.testService.service
				prepareTestApprovalUnlock(t, fixture.testService, "master password")
				service.Lock()
				fixture.agentApprover.onApprove = func() { unlockAfterEpochEnds(t, service) }
				connection := fixture.connect(t)
				if kind == cliprotocol.TypeAgentResolveSecret {
					connection.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeAgentHello, AgentName: "omp"}, nil)
				}
				var result cliprotocol.ResolveSecretResult
				failure := connection.send(t, cliprotocol.Request{
					Type: kind, References: []string{deployTokenReference}, CommandLine: []string{"make", "deploy"},
				}, &result)
				if !strings.HasPrefix(failure, "denied: ") || len(result.Values) != 0 {
					t.Errorf("re-lock during approval returned failure=%q values=%q", failure, result.Values)
				}
				assertNoSavedAgentGrants(t, service)
			})
		}
	}
}

func assertNoSavedAgentGrants(t *testing.T, service *Service) {
	t.Helper()
	if grants := service.ListAgentGrants(); len(grants) != 0 {
		t.Errorf("ended request saved grants: %+v", grants)
	}
	if sealed := readSealedAgentGrants(t, service); len(sealed) != 0 {
		t.Error("ended request left sealed grants on disk")
	}
	if head := getTestAgentGrantHeads(service).getHead(); len(head) != 0 {
		t.Error("ended request saved a grant head")
	}
}

func TestAgentSignInCannotCrossArrivalEpoch(t *testing.T) {
	for _, schedule := range []string{"before authorization", "relock during locked approval"} {
		t.Run(schedule, func(t *testing.T) {
			fixture := createAgentSessionTest(t, clisocket.AgentGrantAlways)
			service := fixture.testService.service
			prepareTestApprovalUnlock(t, fixture.testService, "master password")
			logins := saveSignInLogins(t, fixture.testService)
			window := &signInWindow{website: "https://tosunlab.com", values: map[signin.ElementID]string{}}
			service.config.SignInDriver = window
			service.signer = signin.NewSigner(window, t.TempDir(), service.config.Now)
			if _, err := service.SetAgentSignInEnabled(true); err != nil {
				t.Fatal(err)
			}
			bridge := &arrivalEpochBridge{CommandLineBridge: fixture.bridge}
			fixture.server = clisocket.NewServer(clisocket.ServerConfig{
				Handler: bridge, AgentApprover: fixture.agentApprover, Approvals: fixture.approvals, PeerResolver: fixture.resolver,
			})
			connection := fixture.connect(t)
			connection.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeAgentHello, AgentName: "omp"}, nil)
			if schedule == "before authorization" {
				bridge.onBind = func() { unlockAfterEpochEnds(t, service) }
			} else {
				service.Lock()
				fixture.agentApprover.onApprove = func() { unlockAfterEpochEnds(t, service) }
			}
			var result cliprotocol.AgentSignInResult
			failure := connection.send(t, cliprotocol.Request{
				Type: cliprotocol.TypeAgentSignIn, Window: signInWindowTarget, ItemID: logins.tosunlab.ID,
			}, &result)
			if !strings.HasPrefix(failure, "denied: ") || result.Outcome != "" || result.Login.ID != "" {
				t.Errorf("ended arrival epoch returned failure=%q result=%+v", failure, result)
			}
			if len(window.posted) != 0 || len(window.values) != 0 {
				t.Errorf("ended request entered values=%v or submitted=%v", window.values, window.posted)
			}
			assertNoSavedAgentGrants(t, service)
		})
	}
}

type staleArrivalGrantStore struct {
	clisocket.AgentGrantStore
	arrival context.Context
	t       *testing.T
}

func (store staleArrivalGrantStore) AddAgentGrants(_ context.Context, authorization clisocket.AgentAuthorization, grants []clisocket.AgentGrant) error {
	if !authorization.IsCurrent() {
		store.t.Error("the test must reach persistence with current socket authorization")
	}
	return store.AgentGrantStore.AddAgentGrants(store.arrival, authorization, grants)
}

func TestAgentGrantPersistenceRejectsEndedArrivalEpoch(t *testing.T) {
	fixture := createAgentSessionTest(t, clisocket.AgentGrantAlways)
	service := fixture.testService.service
	prepareTestApprovalUnlock(t, fixture.testService, "master password")
	arrival := fixture.bridge.BindSession(context.Background())
	unlockAfterEpochEnds(t, service)
	fixture.approvals.SetAgentGrantStore(staleArrivalGrantStore{AgentGrantStore: fixture.bridge, arrival: arrival, t: t})
	connection := fixture.connect(t)
	var result cliprotocol.ResolveSecretResult
	failure := connection.send(t, cliprotocol.Request{
		Type: cliprotocol.TypeResolveSecret, References: []string{deployTokenReference}, CommandLine: []string{"make", "deploy"},
	}, &result)
	if !strings.HasPrefix(failure, "denied: ") || len(result.Values) != 0 {
		t.Errorf("stale arrival at persistence returned failure=%q values=%q", failure, result.Values)
	}
	assertNoSavedAgentGrants(t, service)
}
