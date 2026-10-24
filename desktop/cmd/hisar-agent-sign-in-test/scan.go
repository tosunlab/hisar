package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/tryhisar/hisar/core/totp"
)

// canaries are the values no file the run leaves may hold: the logins' passwords, the master password, the one-time
// password's secret and every code it made while the run lasted.
type canaries struct {
	Passwords      []string  `json:"passwords"`
	MasterPassword string    `json:"masterPassword"`
	OneTimeSecret  string    `json:"oneTimeSecret"`
	Started        time.Time `json:"started"`
}

type match struct {
	Path     string `json:"path"`
	Medium   string `json:"medium"`
	Kind     string `json:"kind"`
	Context  string `json:"context,omitempty"`
	Category string `json:"category,omitempty"`
}

type scanned struct {
	Path     string `json:"path"`
	Medium   string `json:"medium"`
	Bytes    int64  `json:"bytes"`
	Category string `json:"category,omitempty"`
}

type scanReport struct {
	Scanned []scanned `json:"scanned"`
	Matches []match   `json:"matches"`
}

// A code may remain in a capture or browser-owned storage, but never in an agent channel.
func (found match) isAllowed() bool {
	return found.Kind == "one-time code" && (found.Category == "browser profile storage" ||
		slices.Contains([]string{"image", "embedded image", "video frame"}, found.Medium))
}

func (report *scanReport) markBrowserProfiles(root string) {
	if root == "" {
		return
	}
	contains := func(path string) bool {
		relative, err := filepath.Rel(root, path)
		return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
	}
	for index := range report.Scanned {
		if contains(report.Scanned[index].Path) {
			report.Scanned[index].Category = "browser profile storage"
		}
	}
	for index := range report.Matches {
		if contains(report.Matches[index].Path) {
			report.Matches[index].Category = "browser profile storage"
		}
	}
}

type scanner struct {
	needles    map[string]string
	codes      map[string]bool
	ocrPath    string
	report     scanReport
	references map[string]string
	imageDir   string
}

var standaloneCode = regexp.MustCompile(`\b[0-9]{8}\b`)

var digitsWithSpaces = regexp.MustCompile(`([0-9])[ \t]+([0-9])`)

// embeddedImage is base64 PNG or JPEG data inside a text file, such as a screenshot in a JSON transcript.
var embeddedImage = regexp.MustCompile(`(iVBORw0KGgo|/9j/)[A-Za-z0-9+/=\\]{800,}`)

type scanPaths []string

func (paths *scanPaths) String() string { return strings.Join(*paths, ", ") }

func (paths *scanPaths) Set(path string) error {
	*paths = append(*paths, path)
	return nil
}

func scanCommand(arguments []string) {
	flags := flag.NewFlagSet("scan", flag.ExitOnError)
	canaryPath := flags.String("canaries", "", "the run's canaries.json")
	ocrPath := flags.String("ocr", "", "scripts/agent-sign-in/ocr.swift, built")
	reportPath := flags.String("report", "", "where to write the scan's JSON report")
	browserProfiles := flags.String("browser-profiles", "", "the throwaway browser profile root, reported separately")
	var fixtureLogs scanPaths
	flags.Var(&fixtureLogs, "fixture-log", "a fixture POST log that intentionally holds test secrets, repeatable")
	flags.Parse(arguments)
	data, err := os.ReadFile(*canaryPath)
	if err != nil {
		log.Fatal(err)
	}
	var values canaries
	if err := json.Unmarshal(data, &values); err != nil {
		log.Fatal(err)
	}
	imageDir, err := os.MkdirTemp(filepath.Dir(*canaryPath), "scan-images-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(imageDir)
	scan := newScanner(values, *ocrPath, imageDir)
	scan.references = map[string]string{filepath.Clean(*canaryPath): "canary reference"}
	for _, path := range fixtureLogs {
		scan.references[filepath.Clean(path)] = "fixture POST"
	}
	for _, root := range flags.Args() {
		if err := scan.scanPath(root); err != nil {
			log.Fatalf("scanning %s: %v", root, err)
		}
	}
	scan.report.markBrowserProfiles(*browserProfiles)
	encoded, _ := json.MarshalIndent(scan.report, "", "  ")
	if *reportPath != "" {
		os.WriteFile(*reportPath, encoded, 0o600)
	}
	var total int64
	for _, entry := range scan.report.Scanned {
		label := entry.Medium
		if entry.Category != "" {
			label = entry.Category + " / " + label
		}
		fmt.Printf("scanned %10d bytes  %-40s %s\n", entry.Bytes, label, entry.Path)
		total += entry.Bytes
	}
	fmt.Printf("scanned %d paths, %d bytes\n", len(scan.report.Scanned), total)
	failed := false
	for _, found := range scan.report.Matches {
		verdict := "FAIL"
		if found.Medium == "canary reference" || found.Medium == "fixture POST" {
			verdict = "EXPECTED TEST INPUT"
		} else if found.Kind == "one-time code" && found.Category == "browser profile storage" {
			verdict = "BROWSER PROFILE STORAGE RESIDUAL"
		} else if found.isAllowed() {
			verdict = "SCREENSHOT/RECORDING RESIDUAL"
		} else {
			failed = true
		}
		fmt.Printf("%s: %s in %s (%s)\n", verdict, found.Kind, found.Path, found.Medium)
		if verdict != "EXPECTED TEST INPUT" && found.Context != "" {
			fmt.Printf("redacted context: %q\n", found.Context)
		}
	}
	if len(scan.report.Matches) == 0 {
		fmt.Println("no canary found")
	}
	if failed {
		os.Exit(1)
	}
}

func newScanner(values canaries, ocrPath, imageDir string) *scanner {
	scan := &scanner{needles: map[string]string{}, codes: map[string]bool{}, ocrPath: ocrPath, imageDir: imageDir}
	for _, password := range values.Passwords {
		scan.needles[password] = "login password"
	}
	if values.MasterPassword != "" {
		scan.needles[values.MasterPassword] = "master password"
	}
	if values.OneTimeSecret != "" {
		scan.needles[values.OneTimeSecret] = "one-time password secret"
		config, err := totp.Parse(oneTimePasswordURI(values.OneTimeSecret))
		if err != nil {
			log.Fatal(err)
		}
		period := time.Duration(config.Period) * time.Second
		for at := values.Started.Add(-2 * period); at.Before(time.Now().Add(2 * period)); at = at.Add(period) {
			code, _ := config.GenerateCode(at)
			scan.codes[code] = true
		}
	}
	return scan
}

func (scan *scanner) scanPath(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if entry.IsDir() && path == scan.imageDir {
			return fs.SkipDir
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		return scan.scanFile(path)
	})
}

func (scan *scanner) scanFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if medium, isReference := scan.references[filepath.Clean(path)]; isReference {
		scan.record(path, medium, data)
		return nil
	}
	switch {
	case bytes.HasPrefix(data, []byte("SQLite format 3\x00")):
		dump, err := exec.Command("sqlite3", "-readonly", path, ".dump").Output()
		if err != nil {
			return fmt.Errorf("sqlite3 .dump %s: %w", path, err)
		}
		scan.record(path, "file", data)
		scan.record(path+" (.dump)", "sqlite dump", dump)
	case isImage(data):
		scan.report.Scanned = append(scan.report.Scanned, scanned{Path: path, Medium: "image", Bytes: int64(len(data))})
		return scan.recognize(path, path, "image")
	case isVideo(path):
		return scan.scanVideo(path, data)
	default:
		scan.record(path, "file", data)
		return scan.scanEmbeddedImages(path, data)
	}
	return nil
}

func isImage(data []byte) bool {
	return bytes.HasPrefix(data, []byte("\x89PNG")) || bytes.HasPrefix(data, []byte("\xff\xd8\xff")) || bytes.HasPrefix(data, []byte("GIF8")) ||
		len(data) > 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP"
}

func isVideo(path string) bool {
	return slices.Contains([]string{".mp4", ".mov", ".m4v"}, strings.ToLower(filepath.Ext(path)))
}

func (scan *scanner) record(path, medium string, data []byte) {
	scan.report.Scanned = append(scan.report.Scanned, scanned{Path: path, Medium: medium, Bytes: int64(len(data))})
	scan.find(path, medium, string(data))
}

func (scan *scanner) find(path, medium, text string) {
	kinds := map[string]bool{}
	var codeContext string
	for needle, kind := range scan.needles {
		if strings.Contains(text, needle) {
			kinds[kind] = true
		}
	}
	if medium == "image" || medium == "embedded image" || medium == "video frame" {
		text = digitsWithSpaces.ReplaceAllString(digitsWithSpaces.ReplaceAllString(text, "$1$2"), "$1$2")
	}
	for _, position := range standaloneCode.FindAllStringIndex(text, -1) {
		start, end := position[0], position[1]
		isFraction := start > 0 && text[start-1] == '.'
		hasFraction := end+1 < len(text) && text[end] == '.' && text[end+1] >= '0' && text[end+1] <= '9'
		if !isFraction && !hasFraction && scan.codes[text[start:end]] {
			kinds["one-time code"] = true
			codeContext = text[max(0, start-100):min(len(text), end+100)]
			for needle, kind := range scan.needles {
				codeContext = strings.ReplaceAll(codeContext, needle, "["+kind+"]")
			}
			for code := range scan.codes {
				codeContext = strings.ReplaceAll(codeContext, code, "[one-time code]")
			}
		}
	}
	for kind := range kinds {
		found := match{Path: path, Medium: medium, Kind: kind}
		if kind == "one-time code" {
			found.Context = codeContext
		}
		scan.report.Matches = append(scan.report.Matches, found)
	}
}

func (scan *scanner) recognize(imagePath, reportedPath, medium string) error {
	output, err := exec.Command(scan.ocrPath, imagePath).Output()
	if err != nil {
		return fmt.Errorf("ocr %s: %w", imagePath, err)
	}
	var recognized struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(output, &recognized); err != nil {
		return fmt.Errorf("ocr %s answered %s", imagePath, output)
	}
	scan.find(reportedPath, medium, recognized.Text)
	return nil
}

func (scan *scanner) scanEmbeddedImages(path string, data []byte) error {
	for index, encoded := range embeddedImage.FindAll(data, -1) {
		cleaned := strings.ReplaceAll(strings.TrimRight(string(encoded), "\\"), `\/`, "/")
		decoded, err := base64.StdEncoding.DecodeString(cleaned)
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(cleaned, "="))
		}
		if err != nil || !isImage(decoded) {
			continue
		}
		imagePath := filepath.Join(scan.imageDir, fmt.Sprintf("embedded-%d-%d", len(scan.report.Scanned), index))
		if err := os.WriteFile(imagePath, decoded, 0o600); err != nil {
			return err
		}
		reported := fmt.Sprintf("%s (embedded image %d)", path, index)
		scan.report.Scanned = append(scan.report.Scanned, scanned{Path: reported, Medium: "embedded image", Bytes: int64(len(decoded))})
		if err := scan.recognize(imagePath, reported, "embedded image"); err != nil {
			return err
		}
	}
	return nil
}

func (scan *scanner) scanVideo(path string, data []byte) error {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("scanning recording %s requires ffmpeg: %w", path, err)
	}
	frames, err := os.MkdirTemp(scan.imageDir, "frames-")
	if err != nil {
		return err
	}
	if output, err := exec.Command(ffmpeg, "-loglevel", "error", "-i", path, "-vf", "fps=1", filepath.Join(frames, "%05d.png")).CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg %s: %v: %s", path, err, output)
	}
	scan.report.Scanned = append(scan.report.Scanned, scanned{Path: path, Medium: "video", Bytes: int64(len(data))})
	entries, err := os.ReadDir(frames)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := scan.recognize(filepath.Join(frames, entry.Name()), path+" frame "+entry.Name(), "video frame"); err != nil {
			return err
		}
	}
	return nil
}

func writeCanaries(path string, values canaries) error {
	encoded, err := json.Marshal(values)
	if err != nil {
		return err
	}
	return os.WriteFile(path, encoded, 0o600)
}
