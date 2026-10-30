package collector

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Status string

const (
	StatusPassed  Status = "passed"
	StatusFailed  Status = "failed"
	StatusSkipped Status = "skipped"
)

type Scenario struct {
	Title       string
	Slug        string
	SpecFile    string
	Line        int
	Status      Status
	Duration    time.Duration
	VideoPath   string
	Screenshots []string
}

func Slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	prevDash := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteByte(c)
			prevDash = false
		} else if !prevDash && b.Len() > 0 {
			b.WriteByte('-')
			prevDash = true
		}
	}
	res := strings.Trim(b.String(), "-")
	if len(res) > 64 {
		res = strings.TrimRight(res[:64], "-")
	}
	if res == "" {
		return "scenario"
	}
	return res
}

type pwReport struct {
	Suites []pwSuite `json:"suites"`
}

type pwSuite struct {
	Title  string    `json:"title"`
	File   string    `json:"file"`
	Suites []pwSuite `json:"suites"`
	Specs  []pwSpec  `json:"specs"`
}

type pwSpec struct {
	Title string   `json:"title"`
	File  string   `json:"file"`
	Line  int      `json:"line"`
	Tests []pwTest `json:"tests"`
}

type pwTest struct {
	Results []pwResult `json:"results"`
}

type pwResult struct {
	Status      string         `json:"status"`
	Duration    int64          `json:"duration"`
	Attachments []pwAttachment `json:"attachments"`
}

type pwAttachment struct {
	Name        string `json:"name"`
	ContentType string `json:"contentType"`
	Path        string `json:"path"`
}

func Discover(resultsDir string, reportFile string) ([]Scenario, error) {
	if reportFile != "" {
		if data, err := os.ReadFile(reportFile); err == nil && len(data) > 0 {
			scenarios, err := parsePlaywrightReport(data)
			if err == nil && len(scenarios) > 0 {
				return scenarios, nil
			}
		}
	}
	return scanResultsDir(resultsDir)
}

func parsePlaywrightReport(data []byte) ([]Scenario, error) {
	var rep pwReport
	if err := json.Unmarshal(data, &rep); err != nil {
		return nil, err
	}
	var scenarios []Scenario
	var walkSuite func(suite pwSuite, parentFile string)
	walkSuite = func(suite pwSuite, parentFile string) {
		specFile := suite.File
		if specFile == "" {
			specFile = parentFile
		}
		for _, spec := range suite.Specs {
			sf := spec.File
			if sf == "" {
				sf = specFile
			}
			for _, t := range spec.Tests {
				if len(t.Results) == 0 {
					continue
				}
				res := t.Results[len(t.Results)-1]
				var videoPath string
				var screenshots []string
				for _, att := range res.Attachments {
					if att.Path == "" {
						continue
					}
					if _, err := os.Stat(att.Path); err != nil {
						continue
					}
					ext := strings.ToLower(filepath.Ext(att.Path))
					switch {
					case ext == ".webm" || ext == ".mp4" || strings.HasPrefix(att.ContentType, "video/"):
						if videoPath == "" {
							videoPath = att.Path
						}
					case ext == ".png" || att.ContentType == "image/png":
						screenshots = append(screenshots, att.Path)
					}
				}
				if videoPath == "" && len(screenshots) == 0 {
					continue
				}
				scenarios = append(scenarios, Scenario{
					Title:       spec.Title,
					Slug:        Slugify(spec.Title),
					SpecFile:    sf,
					Line:        spec.Line,
					Status:      mapStatus(res.Status),
					Duration:    time.Duration(res.Duration) * time.Millisecond,
					VideoPath:   videoPath,
					Screenshots: screenshots,
				})
			}
		}
		for _, child := range suite.Suites {
			walkSuite(child, specFile)
		}
	}
	for _, s := range rep.Suites {
		walkSuite(s, "")
	}
	return scenarios, nil
}

func mapStatus(s string) Status {
	switch strings.ToLower(s) {
	case "passed", "expected":
		return StatusPassed
	case "failed", "unexpected", "timedout":
		return StatusFailed
	default:
		return StatusSkipped
	}
}

func scanResultsDir(resultsDir string) ([]Scenario, error) {
	if resultsDir == "" {
		return nil, nil
	}
	info, err := os.Stat(resultsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}

	type dirGroup struct {
		dir         string
		videoPath   string
		screenshots []string
		failed      bool
	}
	groups := make(map[string]*dirGroup)

	err = filepath.WalkDir(resultsDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".webm" && ext != ".mp4" && ext != ".png" {
			return nil
		}
		parent := filepath.Dir(path)
		g, ok := groups[parent]
		if !ok {
			g = &dirGroup{dir: parent}
			groups[parent] = g
		}
		if ext == ".webm" || ext == ".mp4" {
			if g.videoPath == "" {
				g.videoPath = path
			}
		} else if ext == ".png" {
			g.screenshots = append(g.screenshots, path)
			if strings.Contains(strings.ToLower(filepath.Base(path)), "failed") {
				g.failed = true
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	dirs := make([]string, 0, len(groups))
	for d := range groups {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)

	scenarios := make([]Scenario, 0, len(dirs))
	for _, d := range dirs {
		g := groups[d]
		sort.Strings(g.screenshots)
		base := filepath.Base(d)
		if d == resultsDir {
			base = "default-flow"
		}
		title := strings.ReplaceAll(base, "-", " ")
		status := StatusPassed
		if g.failed {
			status = StatusFailed
		}
		scenarios = append(scenarios, Scenario{
			Title:       title,
			Slug:        Slugify(base),
			Status:      status,
			VideoPath:   g.videoPath,
			Screenshots: g.screenshots,
		})
	}
	return scenarios, nil
}
