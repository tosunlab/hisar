package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

type site struct {
	Main    string `json:"main"`
	Other   string `json:"other"`
	logPath string
	command *exec.Cmd
}

type post struct {
	Origin string            `json:"origin"`
	Path   string            `json:"path"`
	Form   map[string]string `json:"form"`
	JSON   json.RawMessage   `json:"json"`
}

func (sent post) holds(value string) bool {
	for _, field := range sent.Form {
		if strings.Contains(field, value) {
			return true
		}
	}
	return strings.Contains(string(sent.JSON), value)
}

func startSite(binary, logPath, oneTimePassword string) (*site, error) {
	mainAddress, err := reservePort()
	if err != nil {
		return nil, err
	}
	otherAddress, err := reservePort()
	if err != nil {
		return nil, err
	}
	command := exec.Command(binary, "-steer", "-log", logPath, "-main", mainAddress, "-other", otherAddress, "-totp", oneTimePassword)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		return nil, err
	}
	served := &site{logPath: logPath, command: command}
	if err := json.NewDecoder(stdout).Decode(served); err != nil {
		served.stop()
		return nil, fmt.Errorf("reading the site's origins: %w", err)
	}
	return served, nil
}

func reservePort() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer listener.Close()
	return listener.Addr().String(), nil
}

func (served *site) stop() {
	served.command.Process.Signal(syscall.SIGTERM)
	served.command.Wait()
}

func (served *site) steer(pageURL string) error {
	response, err := http.PostForm(served.Main+"/steer", url.Values{"url": {pageURL}})
	if err != nil {
		return err
	}
	return response.Body.Close()
}

func (served *site) readPosts() ([]post, error) {
	file, err := os.Open(served.logPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	posts := []post{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1<<16), 1<<20)
	for scanner.Scan() {
		var line post
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			return nil, err
		}
		posts = append(posts, line)
	}
	return posts, scanner.Err()
}

func (served *site) waitForPosts(count, want int, timeout time.Duration) []post {
	var posts []post
	for deadline := time.Now().Add(timeout); ; time.Sleep(100 * time.Millisecond) {
		all, _ := served.readPosts()
		if len(all) >= count {
			posts = all[count:]
		}
		if len(posts) >= want || time.Now().After(deadline) {
			return posts
		}
	}
}
