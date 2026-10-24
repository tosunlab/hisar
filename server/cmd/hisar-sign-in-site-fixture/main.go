// Command hisar-sign-in-site-fixture serves loopback sign-in pages and logs submitted forms.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tryhisar/hisar/core/totp"
)

// DefaultOneTimePassword is the fixture account's one-time password: eight digits, so a code is easy to search for.
const DefaultOneTimePassword = "otpauth://totp/Fixture:user?secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP&digits=8"

//go:embed pages/*.html
var pageFiles embed.FS

type origins struct {
	Main  string `json:"main"`
	Other string `json:"other"`
}

type post struct {
	Time   time.Time         `json:"time"`
	Origin string            `json:"origin"`
	Path   string            `json:"path"`
	Form   map[string]string `json:"form,omitempty"`
	JSON   json.RawMessage   `json:"json,omitempty"`
}

// steering is where the test wants the browser's top page to go next: every page of a -steer site polls it, so a test
// navigates a browser it does not otherwise control, without activating it or opening a debugging port.
type steering struct {
	ID  int    `json:"id"`
	URL string `json:"url"`
}

type site struct {
	pages     *template.Template
	origins   origins
	code      totp.Config
	isSteered bool
	mutex     sync.Mutex
	log       io.Writer
	steering  steering
}

func main() {
	mainAddress := flag.String("main", "127.0.0.1:0", "loopback address of the main origin, http://127.0.0.1:<port>")
	otherAddress := flag.String("other", "127.0.0.1:0", "loopback address of the other origin, reached as http://localhost:<port>")
	logPath := flag.String("log", "", "file the posted forms are appended to (default: standard error)")
	oneTimePassword := flag.String("totp", DefaultOneTimePassword, "otpauth:// URI the code pages verify against")
	isSteered := flag.Bool("steer", false, "let a test navigate the browser: pages follow POST /steer url=…")
	flag.Parse()

	code, err := totp.Parse(*oneTimePassword)
	if err != nil {
		log.Fatal(err)
	}
	var postLog io.Writer = os.Stderr
	if *logPath != "" {
		file, err := os.OpenFile(*logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()
		postLog = file
	}
	mainListener, err := net.Listen("tcp", *mainAddress)
	if err != nil {
		log.Fatal(err)
	}
	otherListener, err := net.Listen("tcp", *otherAddress)
	if err != nil {
		log.Fatal(err)
	}
	served := origins{
		Main:  "http://127.0.0.1:" + port(mainListener),
		Other: "http://localhost:" + port(otherListener),
	}
	pages, err := template.ParseFS(pageFiles, "pages/*.html")
	if err != nil {
		log.Fatal(err)
	}
	fixture := &site{pages: pages, origins: served, code: code, isSteered: *isSteered, log: postLog}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	for _, listener := range []net.Listener{mainListener, otherListener} {
		server := &http.Server{Handler: fixture.routes(), ReadHeaderTimeout: 10 * time.Second}
		go server.Serve(listener)
		context.AfterFunc(ctx, func() { server.Close() })
	}
	if err := json.NewEncoder(os.Stdout).Encode(served); err != nil {
		log.Fatal(err)
	}
	<-ctx.Done()
}

func port(listener net.Listener) string {
	_, value, _ := net.SplitHostPort(listener.Addr().String())
	return value
}

func (fixture *site) routes() http.Handler {
	mux := http.NewServeMux()
	for _, name := range []string{
		"plain", "controlled", "iframe", "step1", "step2", "spa-steps", "totp", "totp-code", "split-totp", "spoof", "signup",
		"newsletter", "wrong", "invisible", "phish", "srcdoc", "return", "hidden", "signed-in",
	} {
		mux.HandleFunc("GET /"+name, func(writer http.ResponseWriter, request *http.Request) {
			fixture.render(writer, name, request.URL.Query())
		})
	}
	mux.HandleFunc("POST /submit/{page}", fixture.submit)
	if fixture.isSteered {
		mux.HandleFunc("GET /steer", fixture.getSteering)
		mux.HandleFunc("POST /steer", fixture.steer)
	}
	return mux
}

func (fixture *site) render(writer http.ResponseWriter, name string, query url.Values) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	data := map[string]any{
		"Other": fixture.origins.Other, "Username": query.Get("username"), "Error": query.Get("error"),
		"IsSteered": fixture.isSteered, "SteeringID": fixture.getSteeringID(),
		"Visit": query.Get("visit"),
	}
	if err := fixture.pages.ExecuteTemplate(writer, name+".html", data); err != nil {
		log.Print(err)
	}
}

func (fixture *site) getSteeringID() int {
	fixture.mutex.Lock()
	defer fixture.mutex.Unlock()
	return fixture.steering.ID
}

func (fixture *site) getSteering(writer http.ResponseWriter, _ *http.Request) {
	fixture.mutex.Lock()
	current := fixture.steering
	fixture.mutex.Unlock()
	writer.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(writer).Encode(current)
}

func (fixture *site) steer(writer http.ResponseWriter, request *http.Request) {
	fixture.mutex.Lock()
	fixture.steering = steering{ID: fixture.steering.ID + 1, URL: request.FormValue("url")}
	fixture.mutex.Unlock()
	writer.WriteHeader(http.StatusNoContent)
}

func (fixture *site) submit(writer http.ResponseWriter, request *http.Request) {
	page := request.PathValue("page")
	entry := post{Time: time.Now().UTC(), Origin: "http://" + request.Host, Path: request.URL.Path}
	if strings.HasPrefix(request.Header.Get("Content-Type"), "application/json") {
		body, err := io.ReadAll(io.LimitReader(request.Body, 1<<16))
		if err != nil || !json.Valid(body) {
			http.Error(writer, "bad JSON", http.StatusBadRequest)
			return
		}
		entry.JSON = body
	} else {
		if err := request.ParseForm(); err != nil {
			http.Error(writer, "bad form", http.StatusBadRequest)
			return
		}
		entry.Form = map[string]string{}
		for key := range request.PostForm {
			entry.Form[key] = request.PostForm.Get(key)
		}
	}
	fixture.record(entry)

	switch page {
	case "step1":
		redirect(writer, "/step2?username="+url.QueryEscape(entry.Form["username"]))
	case "totp":
		redirect(writer, "/totp-code")
	case "totp-code", "split-totp":
		if !fixture.isCodeValid(entry.Form["code"]) {
			redirect(writer, "/"+page+"?error=wrong-code")
			return
		}
		redirect(writer, "/signed-in")
	case "wrong":
		writer.WriteHeader(http.StatusUnauthorized)
	case "controlled", "spa-steps", "hidden", "hidden-report":
		writer.WriteHeader(http.StatusNoContent)
	default:
		redirect(writer, "/signed-in")
	}
}

func redirect(writer http.ResponseWriter, location string) {
	writer.Header().Set("Location", location)
	writer.WriteHeader(http.StatusSeeOther)
}

func (fixture *site) isCodeValid(code string) bool {
	now := time.Now()
	for _, at := range []time.Time{now, now.Add(-time.Duration(fixture.code.Period) * time.Second)} {
		if expected, _ := fixture.code.GenerateCode(at); code == expected {
			return true
		}
	}
	return false
}

func (fixture *site) record(entry post) {
	line, err := json.Marshal(entry)
	if err != nil {
		log.Print(err)
		return
	}
	fixture.mutex.Lock()
	defer fixture.mutex.Unlock()
	fixture.log.Write(append(line, '\n'))
}
