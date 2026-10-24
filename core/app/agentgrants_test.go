package app

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
	"github.com/tryhisar/hisar/core/clisocket"
	"github.com/tryhisar/hisar/core/localstate"
)

var (
	allGrantDurations     = []clisocket.AgentGrantDuration{clisocket.AgentGrantOnce, clisocket.AgentGrantHour, clisocket.AgentGrantDay, clisocket.AgentGrantAlways}
	sessionGrantDurations = []clisocket.AgentGrantDuration{clisocket.AgentGrantOnce, clisocket.AgentGrantHour}
	errKeychainDenied     = fmt.Errorf("%w: the user denied access", ErrAgentGrantHeadUnavailable)
	revealDeployToken     = cliprotocol.Request{Type: cliprotocol.TypeAgentRevealSecret, References: []string{deployTokenReference}}
)

func getTestAgentGrantHeads(service *Service) *memoryAgentGrantHeads {
	return service.config.AgentGrantHeads.(*memoryAgentGrantHeads)
}

func (test *agentSessionTest) connectAgent(t *testing.T) *commandLineTest {
	t.Helper()
	connection := test.connect(t)
	connection.mustSend(t, cliprotocol.Request{Type: cliprotocol.TypeAgentHello, AgentName: "omp"}, nil)
	return connection
}

func (test *agentSessionTest) relock(t *testing.T) {
	t.Helper()
	test.testService.service.Lock()
	if _, err := test.testService.service.Unlock("master password"); err != nil {
		t.Fatal(err)
	}
}

func (test *agentSessionTest) getLastGrantDurations() []clisocket.AgentGrantDuration {
	test.agentApprover.mutex.Lock()
	defer test.agentApprover.mutex.Unlock()
	return test.agentApprover.requests[len(test.agentApprover.requests)-1].GrantDurations
}

// replaceSealedAgentGrants puts sealed in the device state while Hisar is locked, as a program that writes the state
// file would, and has the service load it as a restart would.
func (test *agentSessionTest) replaceSealedAgentGrants(t *testing.T, sealed []byte) {
	t.Helper()
	service := test.testService.service
	state, err := localstate.Load(service.config.StatePath)
	if err != nil {
		t.Fatal(err)
	}
	state.SealedAgentGrants = sealed
	if err := state.Save(); err != nil {
		t.Fatal(err)
	}
	service.mutex.Lock()
	service.state = state
	service.mutex.Unlock()
}

func readSealedAgentGrants(t *testing.T, service *Service) []byte {
	t.Helper()
	state, err := localstate.Load(service.config.StatePath)
	if err != nil {
		t.Fatal(err)
	}
	return state.SealedAgentGrants
}

func TestRestoringRevokedSealedBlobCannotRestoreGrant(t *testing.T) {
	test := createAgentSessionTest(t, clisocket.AgentGrantAlways)
	service := test.testService.service
	prepareTestApprovalUnlock(t, test.testService, "master password")
	test.runDeploy(t, test.connect(t))
	revokedBlob := readSealedAgentGrants(t, service)
	if _, err := service.RevokeAgentGrant(service.ListAgentGrants()[0].ID); err != nil {
		t.Fatal(err)
	}

	service.Lock()
	test.replaceSealedAgentGrants(t, revokedBlob)
	if _, err := service.Unlock("master password"); err != nil {
		t.Fatal(err)
	}
	if grants := service.ListAgentGrants(); len(grants) != 0 {
		t.Errorf("a restored copy of the state reopened a revoked grant: %+v", grants)
	}
	test.runDeploy(t, test.connect(t))
	if count := test.agentApprover.getCount(); count != 2 {
		t.Errorf("the revoked grant skipped the dialog: %d approvals", count)
	}
}

func TestRestoringSealedGrantsOverNewerOnesOpensNone(t *testing.T) {
	test := createAgentSessionTest(t, clisocket.AgentGrantAlways)
	service := test.testService.service
	prepareTestApprovalUnlock(t, test.testService, "master password")
	test.runDeploy(t, test.connect(t))
	olderBlob := readSealedAgentGrants(t, service)
	test.connectAgent(t).mustSend(t, revealDeployToken, nil)
	if grants := service.ListAgentGrants(); len(grants) != 2 {
		t.Fatalf("grants %+v", grants)
	}

	service.Lock()
	test.replaceSealedAgentGrants(t, olderBlob)
	if _, err := service.Unlock("master password"); err != nil {
		t.Fatal(err)
	}
	if grants := service.ListAgentGrants(); len(grants) != 0 {
		t.Errorf("an older copy of the grants opened: %+v", grants)
	}
}

func TestAgentGrantsWithoutTheirHeadAreDropped(t *testing.T) {
	test := createAgentSessionTest(t, clisocket.AgentGrantAlways)
	service := test.testService.service
	heads := getTestAgentGrantHeads(service)
	prepareTestApprovalUnlock(t, test.testService, "master password")
	test.runDeploy(t, test.connect(t))
	if err := heads.DeleteAgentGrantHead(); err != nil {
		t.Fatal(err)
	}

	test.relock(t)
	if grants := service.ListAgentGrants(); len(grants) != 0 {
		t.Errorf("grants without a head opened: %+v", grants)
	}
	if head := heads.getHead(); head != nil {
		t.Errorf("a head was made for grants that had none: %x", head)
	}
	if sealed := readSealedAgentGrants(t, service); len(sealed) != 0 {
		t.Errorf("the state keeps the dropped grants: %x", sealed)
	}
	test.runDeploy(t, test.connect(t))
	if count := test.agentApprover.getCount(); count != 2 {
		t.Errorf("grants without a head skipped the dialog: %d approvals", count)
	}
}

func TestUnavailableAgentGrantHeadOffersOnlySessionChoices(t *testing.T) {
	test := createAgentSessionTest(t, clisocket.AgentGrantAlways)
	service := test.testService.service
	heads := getTestAgentGrantHeads(service)
	prepareTestApprovalUnlock(t, test.testService, "master password")
	test.runDeploy(t, test.connect(t))
	if durations := test.getLastGrantDurations(); !slices.Equal(durations, allGrantDurations) {
		t.Fatalf("offered %v", durations)
	}

	heads.setError(errKeychainDenied)
	test.relock(t)
	if grants := service.ListAgentGrants(); len(grants) != 0 {
		t.Errorf("grants opened without their head: %+v", grants)
	}
	test.runDeploy(t, test.connect(t))
	if durations := test.getLastGrantDurations(); test.agentApprover.getCount() != 2 || !slices.Equal(durations, sessionGrantDurations) {
		t.Errorf("approval %d offered %v", test.agentApprover.getCount(), durations)
	}

	heads.setError(nil)
	test.relock(t)
	service.RevokeAgentSession(test.resolver.omp.PID, test.resolver.omp.StartTime)
	test.runDeploy(t, test.connect(t))
	if durations := test.getLastGrantDurations(); !slices.Equal(durations, allGrantDurations) {
		t.Errorf("a working head store offered %v", durations)
	}
}

func TestWithoutAgentGrantHeadStoreGrantsEndWithTheSession(t *testing.T) {
	test := createAgentSessionTest(t, clisocket.AgentGrantAlways)
	service := test.testService.service
	service.mutex.Lock()
	service.config.AgentGrantHeads = nil
	service.mutex.Unlock()
	prepareTestApprovalUnlock(t, test.testService, "master password")
	test.runDeploy(t, test.connect(t))
	if durations := test.getLastGrantDurations(); !slices.Equal(durations, sessionGrantDurations) {
		t.Errorf("offered %v", durations)
	}
	if grants := service.ListAgentGrants(); len(grants) != 0 {
		t.Errorf("grants %+v", grants)
	}
}

func TestOneHourWithoutSavedGrantsLastsForTheSession(t *testing.T) {
	for name, disable := range map[string]func(t *testing.T, test *agentSessionTest){
		"no head store": func(t *testing.T, test *agentSessionTest) {
			service := test.testService.service
			service.mutex.Lock()
			service.config.AgentGrantHeads = nil
			service.mutex.Unlock()
		},
		"head store unavailable": func(t *testing.T, test *agentSessionTest) {
			getTestAgentGrantHeads(test.testService.service).setError(errKeychainDenied)
			test.relock(t)
		},
	} {
		t.Run(name, func(t *testing.T) {
			test := createAgentSessionTest(t, clisocket.AgentGrantHour)
			service := test.testService.service
			prepareTestApprovalUnlock(t, test.testService, "master password")
			disable(t, test)
			test.runDeploy(t, test.connect(t))
			if durations := test.getLastGrantDurations(); !slices.Equal(durations, sessionGrantDurations) {
				t.Errorf("offered %v", durations)
			}
			if grants := service.ListAgentGrants(); len(grants) != 1 || grants[0].Scope != "session" {
				t.Errorf("grants %+v; want one for the session", grants)
			}
			test.runDeploy(t, test.connect(t))
			if count := test.agentApprover.getCount(); count != 1 {
				t.Errorf("the hour asked again: %d approvals", count)
			}
		})
	}
}

func TestAgentGrantIsNotSavedWhenItsHeadCannotBeWritten(t *testing.T) {
	test := createAgentSessionTest(t, clisocket.AgentGrantAlways)
	service := test.testService.service
	heads := getTestAgentGrantHeads(service)
	prepareTestApprovalUnlock(t, test.testService, "master password")
	test.runDeploy(t, test.connect(t))
	saved := service.ListAgentGrants()
	if len(saved) != 1 {
		t.Fatalf("grants %+v", saved)
	}

	heads.setError(errKeychainDenied)
	if _, err := service.RevokeAgentGrant(saved[0].ID); err == nil {
		t.Error("revoking a grant whose head cannot be written succeeded")
	}
	test.connectAgent(t).mustSend(t, revealDeployToken, nil)
	if count := test.agentApprover.getCount(); count != 2 {
		t.Fatalf("%d approvals", count)
	}
	if grants := service.ListAgentGrants(); len(grants) != 0 {
		t.Errorf("a grant survived a failed revocation: %+v", grants)
	}
	if sealed := readSealedAgentGrants(t, service); len(sealed) != 0 {
		t.Error("the state kept grants after a failed revocation")
	}
	test.connectAgent(t).mustSend(t, revealDeployToken, nil)
	if durations := test.getLastGrantDurations(); test.agentApprover.getCount() != 3 || !slices.Equal(durations, sessionGrantDurations) {
		t.Errorf("approval %d offered %v after the head failed", test.agentApprover.getCount(), durations)
	}
}

func TestAgentGrantWhoseAuthorizationEndsDuringItsHeadWriteIsNotKept(t *testing.T) {
	for name, end := range map[string]func(test *agentSessionTest){
		"session revoked": func(test *agentSessionTest) {
			test.approvals.RevokeAgentSession(test.resolver.omp.PID, test.resolver.omp.StartTime)
		},
		"approvals cleared": func(test *agentSessionTest) { test.approvals.ClearAll() },
		"another account":   func(test *agentSessionTest) { test.approvals.SetAccount("another-account") },
	} {
		t.Run(name, func(t *testing.T) {
			test := createAgentSessionTest(t, clisocket.AgentGrantAlways)
			service := test.testService.service
			prepareTestApprovalUnlock(t, test.testService, "master password")
			test.runDeploy(t, test.connect(t))
			saved := service.ListAgentGrants()
			if len(saved) != 1 {
				t.Fatalf("grants %+v", saved)
			}

			getTestAgentGrantHeads(service).setOnWrite(func() { end(test) })
			connection := test.connect(t).connection
			connection.SetDeadline(time.Now().Add(10 * time.Second))
			if err := cliprotocol.WriteJSON(connection, cliprotocol.Request{ID: "1", Type: cliprotocol.TypeAgentHello, AgentName: "omp"}); err != nil {
				t.Fatal(err)
			}
			var response cliprotocol.Response
			if err := cliprotocol.ReadJSON(connection, &response); err != nil || response.Error != nil {
				t.Fatalf("hello: %v %+v", err, response.Error)
			}
			if err := cliprotocol.WriteJSON(connection, cliprotocol.Request{ID: "2", Type: revealDeployToken.Type, References: revealDeployToken.References}); err != nil {
				t.Fatal(err)
			}
			response = cliprotocol.Response{}
			if err := cliprotocol.ReadJSON(connection, &response); err == nil && response.Error == nil {
				t.Errorf("handed out %s", response.Result)
			}
			if grants := service.ListAgentGrants(); len(grants) != 1 || grants[0].ID != saved[0].ID {
				t.Errorf("listed %+v; want only the grant saved before", grants)
			}

			test.relock(t)
			if grants := service.ListAgentGrants(); len(grants) != 1 || grants[0].ID != saved[0].ID {
				t.Errorf("after a restart %+v; want only the grant saved before", grants)
			}
			if name == "another account" {
				return
			}
			test.runDeploy(t, test.connect(t))
			test.connectAgent(t).mustSend(t, revealDeployToken, nil)
			if count := test.agentApprover.getCount(); count != 3 {
				t.Errorf("%d approvals; want the first and the reveal twice", count)
			}
		})
	}
}
