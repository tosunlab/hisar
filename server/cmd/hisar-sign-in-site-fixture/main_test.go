package main

import (
	"bytes"
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/tryhisar/hisar/core/totp"
)

func createSite(t *testing.T) (*httptest.Server, *bytes.Buffer, totp.Config) {
	t.Helper()
	pages, err := template.ParseFS(pageFiles, "pages/*.html")
	if err != nil {
		t.Fatal(err)
	}
	code, err := totp.Parse(DefaultOneTimePassword)
	if err != nil {
		t.Fatal(err)
	}
	postLog := &bytes.Buffer{}
	fixture := &site{pages: pages, origins: origins{Main: "http://127.0.0.1:1", Other: "http://localhost:2"}, code: code, isSteered: true, log: postLog}
	server := httptest.NewServer(fixture.routes())
	t.Cleanup(server.Close)
	return server, postLog, code
}

func postForm(t *testing.T, server *httptest.Server, path string, form url.Values) *http.Response {
	t.Helper()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.PostForm(server.URL+path, form)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	return response
}

func TestEveryPageRenders(t *testing.T) {
	server, _, _ := createSite(t)
	for _, name := range []string{"plain", "iframe", "step2?username=a%40example.com", "split-totp", "totp-code?error=wrong-code", "signed-in"} {
		response, err := http.Get(server.URL + "/" + name)
		if err != nil {
			t.Fatal(err)
		}
		var body bytes.Buffer
		body.ReadFrom(response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusOK || !strings.Contains(body.String(), "<title>") {
			t.Errorf("%s: %d %q", name, response.StatusCode, body.String())
		}
		if name == "iframe" && !strings.Contains(body.String(), `src="http://localhost:2/plain"`) {
			t.Errorf("the frame is not on the other origin: %s", body.String())
		}
	}
}

func TestSignedInVisitDistinguishesNavigation(t *testing.T) {
	server, _, _ := createSite(t)
	for _, visit := range []string{"", "first", "second"} {
		response, err := http.Get(server.URL + "/signed-in?visit=" + visit)
		if err != nil {
			t.Fatal(err)
		}
		var body bytes.Buffer
		body.ReadFrom(response.Body)
		response.Body.Close()
		title := "Signed in"
		if visit != "" {
			title += " " + visit
		}
		if !strings.Contains(body.String(), "<title>"+title+"</title>") {
			t.Errorf("visit %q has no distinct title: %s", visit, body.String())
		}
	}
}

func TestPostsAreLoggedAsJSONLines(t *testing.T) {
	server, postLog, _ := createSite(t)
	response := postForm(t, server, "/submit/step1", url.Values{"username": {"a@example.com"}})
	if location := response.Header.Get("Location"); response.StatusCode != http.StatusSeeOther || location != "/step2?username=a%40example.com" {
		t.Errorf("step1 answered %d %q", response.StatusCode, location)
	}
	jsonResponse, err := http.Post(server.URL+"/submit/wrong", "application/json", strings.NewReader(`{"username":"a","password":"b"}`))
	if err != nil {
		t.Fatal(err)
	}
	jsonResponse.Body.Close()
	if jsonResponse.StatusCode != http.StatusUnauthorized {
		t.Errorf("the wrong-password page was accepted: %d", jsonResponse.StatusCode)
	}

	lines := strings.Split(strings.TrimSpace(postLog.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("log = %q", postLog.String())
	}
	var form, body post
	if err := json.Unmarshal([]byte(lines[0]), &form); err != nil || form.Path != "/submit/step1" || form.Form["username"] != "a@example.com" {
		t.Errorf("form line = %s (%v)", lines[0], err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &body); err != nil || string(body.JSON) != `{"username":"a","password":"b"}` {
		t.Errorf("JSON line = %s (%v)", lines[1], err)
	}
}

func TestCodePagesVerifyTheCode(t *testing.T) {
	server, _, code := createSite(t)
	current, _ := code.GenerateCode(time.Now())
	if len(current) != 8 {
		t.Fatalf("code %q is not 8 digits", current)
	}
	if location := postForm(t, server, "/submit/totp-code", url.Values{"code": {current}}).Header.Get("Location"); location != "/signed-in" {
		t.Errorf("the current code went to %q", location)
	}
	// The site accepts the codes of two periods; around the request, three periods' codes may be valid, so one of four
	// candidates is wrong whatever the clock says.
	now, period := time.Now(), time.Duration(code.Period)*time.Second
	valid := map[string]bool{}
	for _, at := range []time.Time{now.Add(-period), now, now.Add(period)} {
		accepted, _ := code.GenerateCode(at)
		valid[accepted] = true
	}
	wrong := "00000000"
	for _, candidate := range []string{"11111111", "22222222", "33333333"} {
		if valid[wrong] {
			wrong = candidate
		}
	}
	if location := postForm(t, server, "/submit/split-totp", url.Values{"code": {wrong}}).Header.Get("Location"); location != "/split-totp?error=wrong-code" {
		t.Errorf("a wrong code went to %q", location)
	}
}

func TestSteeringMovesEveryTopPage(t *testing.T) {
	server, _, _ := createSite(t)
	if response := postForm(t, server, "/steer", url.Values{"url": {"http://localhost:2/phish"}}); response.StatusCode != http.StatusNoContent {
		t.Fatalf("steer answered %d", response.StatusCode)
	}
	response, err := http.Get(server.URL + "/steer")
	if err != nil {
		t.Fatal(err)
	}
	var current steering
	json.NewDecoder(response.Body).Decode(&current)
	response.Body.Close()
	if current != (steering{ID: 1, URL: "http://localhost:2/phish"}) {
		t.Errorf("steering = %+v", current)
	}
	page, err := http.Get(server.URL + "/plain")
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	body.ReadFrom(page.Body)
	page.Body.Close()
	if !strings.Contains(body.String(), "steering.id >  1 ") {
		t.Errorf("a page loaded after the steer would follow it again: %s", body.String())
	}
}
