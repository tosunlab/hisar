package commandline

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/cliprotocol"
)

func TestTimeoutNamesTheRealDeadline(t *testing.T) {
	for _, testCase := range []struct {
		timeout  time.Duration
		expected string
	}{
		{time.Minute, "Hisar denied the request (it was not answered within a minute)"},
		{2 * time.Minute, "Hisar denied the request (it was not answered within 2 minutes)"},
	} {
		fixture := createServerFixture(t)
		fixture.approver.result = TimeoutError{Timeout: testCase.timeout}
		response := fixture.connect(t).send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault})
		if getErrorCode(response) != cliprotocol.ErrorDenied || response.Error.Message != testCase.expected {
			t.Errorf("timeout %v: %+v", testCase.timeout, response.Error)
		}
	}
	if !errors.Is(TimeoutError{Timeout: time.Minute}, ErrDenied) {
		t.Error("a timeout is not a denial")
	}
}

func TestSecretRequestShowsSecretsAndCommandInTheApproval(t *testing.T) {
	fixture := createServerFixture(t)
	client := fixture.connect(t)
	allReference := []string{"hisar://Personal/Example/password", "hisar://Work/AWS%20prod/secret%20key", "not a reference"}
	response := client.send(t, cliprotocol.Request{Type: cliprotocol.TypeResolveSecret, References: allReference, CommandLine: []string{"npm", "test"}})
	if !response.OK {
		t.Fatalf("secrets.resolve: %+v", response.Error)
	}
	if count := fixture.approver.requestCount.Load(); count != 1 {
		t.Fatalf("approval asked %d times", count)
	}
	request := fixture.approver.allRequest[0]
	allExpectedName := []string{"Personal › Example › password", "Work › AWS prod › secret key", "not a reference"}
	if !request.IsSecretRequest || !slices.Equal(request.AllSecretName, allExpectedName) || !slices.Equal(request.CommandLine, []string{"npm", "test"}) || request.TerminalName != "ttys003" {
		t.Errorf("approval request: %+v", request)
	}
	if response := client.send(t, cliprotocol.Request{Type: cliprotocol.TypeResolveSecret, References: allReference[:1]}); !response.OK {
		t.Fatalf("second secrets.resolve: %+v", response.Error)
	}
	if count := fixture.approver.requestCount.Load(); count != 1 {
		t.Errorf("a secret read in an approved session asked again (%d approvals)", count)
	}
}

type describingHandler struct {
	*fakeHandler
}

func (describingHandler) DescribeAllSecret(allReference []string) []string {
	allName := []string{}
	for range allReference {
		allName = append(allName, "Personal › Example › password")
	}
	return allName
}

func TestSecretRequestUsesTheHandlersNames(t *testing.T) {
	fixture := createServerFixture(t)
	fixture.server.config.Handler = describingHandler{fixture.handler}
	fixture.connect(t).send(t, cliprotocol.Request{Type: cliprotocol.TypeResolveSecret, References: []string{"hisar://v1/i1/password"}})
	if request := fixture.approver.allRequest[0]; !slices.Equal(request.AllSecretName, []string{"Personal › Example › password"}) {
		t.Errorf("names: %v", request.AllSecretName)
	}
}

func TestOtherRequestsCarryNoSecrets(t *testing.T) {
	fixture := createServerFixture(t)
	fixture.connect(t).send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault})
	if request := fixture.approver.allRequest[0]; request.IsSecretRequest || len(request.AllSecretName) != 0 || len(request.CommandLine) != 0 {
		t.Errorf("vaults.list approval: %+v", request)
	}
}

func TestPrivateKeyReferenceNeedsNoSpecialApproval(t *testing.T) {
	fixture := createServerFixture(t)
	client := fixture.connect(t)
	client.send(t, cliprotocol.Request{Type: cliprotocol.TypeListVault})
	client.send(t, cliprotocol.Request{Type: cliprotocol.TypeResolveSecret, References: []string{"hisar://Personal/Deploy%20key/private%20key"}})
	if count := fixture.approver.requestCount.Load(); count != 1 {
		t.Errorf("%d approvals; the app refuses private keys, so they ask nothing extra", count)
	}
}

func TestSecretRequestNeedsReferences(t *testing.T) {
	fixture := createServerFixture(t)
	client := fixture.connect(t)
	if response := client.send(t, cliprotocol.Request{Type: cliprotocol.TypeResolveSecret}); getErrorCode(response) != cliprotocol.ErrorInvalidRequest {
		t.Errorf("no references: %+v", response)
	}
	allTooMany := make([]string, cliprotocol.MaxReferenceCount+1)
	for index := range allTooMany {
		allTooMany[index] = "hisar://v/i/password"
	}
	if response := client.send(t, cliprotocol.Request{Type: cliprotocol.TypeResolveSecret, References: allTooMany}); getErrorCode(response) != cliprotocol.ErrorInvalidRequest {
		t.Errorf("too many references: %+v", response)
	}
	if count := fixture.approver.requestCount.Load(); count != 0 {
		t.Errorf("an invalid request asked for approval")
	}
}
