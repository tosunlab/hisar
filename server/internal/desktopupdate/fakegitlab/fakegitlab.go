// Package fakegitlab is an in-memory GitLab generic package registry for tests.
package fakegitlab

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const ProjectID = "2"

type File struct {
	Name    string
	Content []byte
}

type Package struct {
	ID          int64
	Name        string
	Version     string
	PackageType string
	AllFile     []File
}

type Server struct {
	*httptest.Server
	Token string
	// BeforeDownload runs before a generic file is sent, e.g. to block or to redirect.
	BeforeDownload func(writer http.ResponseWriter, request *http.Request, fileName string) bool

	mutex          sync.Mutex
	allPackage     []*Package
	allRequestPath []string
	allToken       []string
}

func Start(t testing.TB, token string) *Server {
	fake := &Server{Token: token}
	fake.Server = httptest.NewServer(http.HandlerFunc(fake.serveHTTP))
	t.Cleanup(fake.Close)
	return fake
}

func (fake *Server) AddPackage(name, version string, allFile ...File) *Package {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	added := &Package{ID: int64(len(fake.allPackage) + 1), Name: name, Version: version, PackageType: "generic", AllFile: allFile}
	fake.allPackage = append(fake.allPackage, added)
	return added
}

func (fake *Server) GetAllRequestPath() []string {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return append([]string(nil), fake.allRequestPath...)
}

// GetAllToken returns the PRIVATE-TOKEN header of every request, in order.
func (fake *Server) GetAllToken() []string {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	return append([]string(nil), fake.allToken...)
}

func (fake *Server) findPackage(test func(*Package) bool) *Package {
	fake.mutex.Lock()
	defer fake.mutex.Unlock()
	for _, candidate := range fake.allPackage {
		if test(candidate) {
			return candidate
		}
	}
	return nil
}

func writeJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	json.NewEncoder(writer).Encode(value)
}

func (fake *Server) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	fake.mutex.Lock()
	fake.allRequestPath = append(fake.allRequestPath, request.URL.EscapedPath())
	fake.allToken = append(fake.allToken, request.Header.Get("PRIVATE-TOKEN"))
	fake.mutex.Unlock()
	if request.Header.Get("PRIVATE-TOKEN") != fake.Token {
		http.Error(writer, `{"message":"401 Unauthorized"}`, http.StatusUnauthorized)
		return
	}
	path, found := strings.CutPrefix(request.URL.Path, "/api/v4/projects/"+ProjectID+"/packages")
	if !found {
		http.NotFound(writer, request)
		return
	}
	switch {
	case path == "":
		fake.listAllPackage(writer, request)
	case strings.HasPrefix(path, "/generic/"):
		allPart := strings.Split(strings.TrimPrefix(path, "/generic/"), "/")
		if len(allPart) != 3 {
			http.NotFound(writer, request)
			return
		}
		fake.sendFile(writer, request, allPart[0], allPart[1], allPart[2])
	case strings.HasSuffix(path, "/package_files"):
		packageID, _ := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(path, "/"), "/package_files"), 10, 64)
		listed := fake.findPackage(func(candidate *Package) bool { return candidate.ID == packageID })
		if listed == nil {
			http.NotFound(writer, request)
			return
		}
		allFile := []map[string]any{}
		for _, file := range listed.AllFile {
			allFile = append(allFile, map[string]any{"file_name": file.Name, "size": len(file.Content)})
		}
		writeJSON(writer, allFile)
	default:
		http.NotFound(writer, request)
	}
}

// listAllPackage mirrors GitLab: package_name is a partial match and the newest package comes first.
func (fake *Server) listAllPackage(writer http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	fake.mutex.Lock()
	allListed := []map[string]any{}
	for index := len(fake.allPackage) - 1; index >= 0; index-- {
		listed := fake.allPackage[index]
		if !strings.Contains(listed.Name, query.Get("package_name")) || (query.Get("package_type") != "" && query.Get("package_type") != listed.PackageType) {
			continue
		}
		allListed = append(allListed, map[string]any{
			"id": listed.ID, "name": listed.Name, "version": listed.Version, "package_type": listed.PackageType, "status": "default",
		})
	}
	fake.mutex.Unlock()
	writeJSON(writer, allListed)
}

func (fake *Server) sendFile(writer http.ResponseWriter, request *http.Request, name, version, fileName string) {
	listed := fake.findPackage(func(candidate *Package) bool { return candidate.Name == name && candidate.Version == version })
	if listed == nil {
		http.NotFound(writer, request)
		return
	}
	for index := len(listed.AllFile) - 1; index >= 0; index-- {
		file := listed.AllFile[index]
		if file.Name != fileName {
			continue
		}
		if fake.BeforeDownload != nil && !fake.BeforeDownload(writer, request, fileName) {
			return
		}
		writer.Header().Set("Content-Type", "application/octet-stream")
		writer.Header().Set("Content-Length", fmt.Sprint(len(file.Content)))
		writer.Write(file.Content)
		return
	}
	http.NotFound(writer, request)
}

type ReleaseConfig struct {
	Version            string
	BuildNumber        int64
	Archive            []byte
	ReleaseNotes       string
	ReleaseNotesFormat string
	PublishedAt        time.Time
}

// CreateAllReleaseFile returns the zip and the update.json the release pipeline uploads, with a fixed signature.
func CreateAllReleaseFile(config ReleaseConfig) []File {
	archiveName := "Hisar-" + config.Version + ".zip"
	if config.ReleaseNotesFormat == "" {
		config.ReleaseNotesFormat = "markdown"
	}
	metadata, _ := json.Marshal(map[string]any{
		"version":              config.Version,
		"buildNumber":          config.BuildNumber,
		"archive":              archiveName,
		"edSignature":          base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 64)),
		"length":               len(config.Archive),
		"minimumSystemVersion": "12.0",
		"releaseNotes":         config.ReleaseNotes,
		"releaseNotesFormat":   config.ReleaseNotesFormat,
		"publishedAt":          config.PublishedAt.UTC().Format(time.RFC3339),
	})
	return []File{{Name: archiveName, Content: config.Archive}, {Name: "update.json", Content: metadata}}
}
