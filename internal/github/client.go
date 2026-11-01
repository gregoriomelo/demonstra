package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"

	"github.com/demonstra-me/demonstra/internal/collector"
)

const CommentMarker = "<!-- demonstra-pr-comment -->"

type ScenarioAssetInput struct {
	Scenario       collector.Scenario
	GIFPath        string
	WebPPath       string
	VideoPath      string
	ScreenshotPath []string
}

type UploadedScenario struct {
	Scenario       collector.Scenario
	GIFURL         string
	WebPURL        string
	VideoURL       string
	ScreenshotURLs []string
}

type PreviousRun struct {
	ShortSHA string
	TreeURL  string
}

type Client struct {
	BaseURL    string
	WebBaseURL string
	Owner      string
	Repo       string
	Token      string
	Branch     string
	HTTPClient *http.Client
}

func NewClient(owner, repo, token, branch string) *Client {
	if branch == "" {
		branch = "demonstra-assets"
	}
	return &Client{
		BaseURL:    "https://api.github.com",
		WebBaseURL: "https://github.com",
		Owner:      owner,
		Repo:       repo,
		Token:      token,
		Branch:     branch,
		HTTPClient: http.DefaultClient,
	}
}

func ShortCommitSHA(sha string) string {
	sha = strings.TrimSpace(sha)
	if len(sha) > 7 {
		return sha[:7]
	}
	if sha == "" {
		return "unknown"
	}
	return sha
}

type treeEntry struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	Type string `json:"type"`
	SHA  string `json:"sha"`
}

func (c *Client) UploadPRScenarios(ctx context.Context, prNumber int, commitSHA string, inputs []ScenarioAssetInput) ([]UploadedScenario, []PreviousRun, error) {
	shortSHA := ShortCommitSHA(commitSHA)
	prPrefix := fmt.Sprintf("pr-%d/", prNumber)

	parentCommitSHA, baseTreeSHA, prevRuns, branchExists, err := c.inspectExistingBranch(ctx, prPrefix, shortSHA)
	if err != nil {
		return nil, nil, err
	}

	var entries []treeEntry
	uploaded := make([]UploadedScenario, 0, len(inputs))
	allocatedSlugs := make(map[string]int)

	for idx, in := range inputs {
		baseSlug := in.Scenario.Slug
		if baseSlug == "" {
			baseSlug = collector.Slugify(in.Scenario.Title)
		}
		if baseSlug == "" {
			baseSlug = fmt.Sprintf("scenario-%d", idx+1)
		}
		allocatedSlugs[baseSlug]++
		slug := baseSlug
		if count := allocatedSlugs[baseSlug]; count > 1 {
			slug = fmt.Sprintf("%s-%d", baseSlug, count)
		}
		up := UploadedScenario{Scenario: in.Scenario}

		if in.GIFPath != "" {
			blobSHA, err := c.createBlobFromFile(ctx, in.GIFPath)
			if err != nil {
				return nil, nil, fmt.Errorf("upload gif blob for %s: %w", slug, err)
			}
			repoPath := fmt.Sprintf("pr-%d/%s/%s/flow.gif", prNumber, shortSHA, slug)
			entries = append(entries, treeEntry{
				Path: repoPath,
				Mode: "100644",
				Type: "blob",
				SHA:  blobSHA,
			})
			up.GIFURL = c.rawBlobURL(repoPath)
		}

		if in.WebPPath != "" {
			blobSHA, err := c.createBlobFromFile(ctx, in.WebPPath)
			if err != nil {
				return nil, nil, fmt.Errorf("upload webp blob for %s: %w", slug, err)
			}
			repoPath := fmt.Sprintf("pr-%d/%s/%s/flow.webp", prNumber, shortSHA, slug)
			entries = append(entries, treeEntry{
				Path: repoPath,
				Mode: "100644",
				Type: "blob",
				SHA:  blobSHA,
			})
			up.WebPURL = c.rawBlobURL(repoPath)
		}

		if in.VideoPath != "" {
			blobSHA, err := c.createBlobFromFile(ctx, in.VideoPath)
			if err != nil {
				return nil, nil, fmt.Errorf("upload video blob for %s: %w", slug, err)
			}
			repoPath := fmt.Sprintf("pr-%d/%s/%s/video.mp4", prNumber, shortSHA, slug)
			entries = append(entries, treeEntry{
				Path: repoPath,
				Mode: "100644",
				Type: "blob",
				SHA:  blobSHA,
			})
			up.VideoURL = c.blobURL(repoPath)
		}

		for i, shot := range in.ScreenshotPath {
			blobSHA, err := c.createBlobFromFile(ctx, shot)
			if err != nil {
				return nil, nil, fmt.Errorf("upload screenshot blob for %s: %w", slug, err)
			}
			repoPath := fmt.Sprintf("pr-%d/%s/%s/screenshots/step-%d.png", prNumber, shortSHA, slug, i+1)
			entries = append(entries, treeEntry{
				Path: repoPath,
				Mode: "100644",
				Type: "blob",
				SHA:  blobSHA,
			})
			up.ScreenshotURLs = append(up.ScreenshotURLs, c.rawBlobURL(repoPath))
		}

		uploaded = append(uploaded, up)
	}

	if len(entries) == 0 {
		return uploaded, prevRuns, nil
	}

	newTreeSHA, err := c.createTree(ctx, baseTreeSHA, entries)
	if err != nil {
		return nil, nil, err
	}

	commitMsg := fmt.Sprintf("chore(demonstra): record visual assets for PR #%d (%s)", prNumber, shortSHA)
	newCommitSHA, err := c.createCommit(ctx, commitMsg, newTreeSHA, parentCommitSHA)
	if err != nil {
		return nil, nil, err
	}

	if branchExists {
		if err := c.updateRef(ctx, newCommitSHA); err != nil {
			return nil, nil, err
		}
	} else {
		if err := c.createRef(ctx, newCommitSHA); err != nil {
			return nil, nil, err
		}
	}

	return uploaded, prevRuns, nil
}

func (c *Client) rawBlobURL(repoPath string) string {
	web := strings.TrimRight(c.WebBaseURL, "/")
	if web == "" {
		web = "https://github.com"
	}
	return fmt.Sprintf("%s/%s/%s/blob/%s/%s?raw=true", web, c.Owner, c.Repo, c.Branch, repoPath)
}

func (c *Client) blobURL(repoPath string) string {
	web := strings.TrimRight(c.WebBaseURL, "/")
	if web == "" {
		web = "https://github.com"
	}
	return fmt.Sprintf("%s/%s/%s/blob/%s/%s", web, c.Owner, c.Repo, c.Branch, repoPath)
}

func (c *Client) treeFolderURL(repoPath string) string {
	web := strings.TrimRight(c.WebBaseURL, "/")
	if web == "" {
		web = "https://github.com"
	}
	return fmt.Sprintf("%s/%s/%s/tree/%s/%s", web, c.Owner, c.Repo, c.Branch, repoPath)
}

func (c *Client) inspectExistingBranch(ctx context.Context, prPrefix, currentShortSHA string) (commitSHA, treeSHA string, prevRuns []PreviousRun, exists bool, err error) {
	var refResp struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	status, err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/git/ref/heads/%s", c.Owner, c.Repo, c.Branch), nil, &refResp)
	if status == http.StatusNotFound {
		return "", "", nil, false, nil
	}
	if err != nil {
		return "", "", nil, false, fmt.Errorf("lookup branch ref: %w", err)
	}
	commitSHA = refResp.Object.SHA

	var commitResp struct {
		Tree struct {
			SHA string `json:"sha"`
		} `json:"tree"`
	}
	if _, err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/git/commits/%s", c.Owner, c.Repo, commitSHA), nil, &commitResp); err != nil {
		return "", "", nil, true, fmt.Errorf("lookup commit %s: %w", commitSHA, err)
	}
	treeSHA = commitResp.Tree.SHA

	var treeResp struct {
		Tree []struct {
			Path string `json:"path"`
		} `json:"tree"`
	}
	if _, err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/git/trees/%s?recursive=1", c.Owner, c.Repo, treeSHA), nil, &treeResp); err == nil {
		seen := make(map[string]bool)
		for _, item := range treeResp.Tree {
			if !strings.HasPrefix(item.Path, prPrefix) {
				continue
			}
			rest := strings.TrimPrefix(item.Path, prPrefix)
			parts := strings.SplitN(rest, "/", 2)
			if len(parts) < 1 || parts[0] == "" || parts[0] == currentShortSHA {
				continue
			}
			sha := parts[0]
			if !seen[sha] {
				seen[sha] = true
				prevRuns = append(prevRuns, PreviousRun{
					ShortSHA: sha,
					TreeURL:  c.treeFolderURL(prPrefix + sha),
				})
			}
		}
		sort.Slice(prevRuns, func(i, j int) bool {
			return prevRuns[i].ShortSHA < prevRuns[j].ShortSHA
		})
	}

	return commitSHA, treeSHA, prevRuns, true, nil
}

func (c *Client) createBlobFromFile(ctx context.Context, filePath string) (string, error) {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	payload := map[string]string{
		"content":  base64.StdEncoding.EncodeToString(raw),
		"encoding": "base64",
	}
	var resp struct {
		SHA string `json:"sha"`
	}
	if _, err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/git/blobs", c.Owner, c.Repo), payload, &resp); err != nil {
		return "", err
	}
	return resp.SHA, nil
}

func (c *Client) createTree(ctx context.Context, baseTreeSHA string, entries []treeEntry) (string, error) {
	payload := map[string]any{
		"tree": entries,
	}
	if baseTreeSHA != "" {
		payload["base_tree"] = baseTreeSHA
	}
	var resp struct {
		SHA string `json:"sha"`
	}
	if _, err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/git/trees", c.Owner, c.Repo), payload, &resp); err != nil {
		return "", fmt.Errorf("create git tree: %w", err)
	}
	return resp.SHA, nil
}

func (c *Client) createCommit(ctx context.Context, message, treeSHA, parentSHA string) (string, error) {
	parents := []string{}
	if parentSHA != "" {
		parents = []string{parentSHA}
	}
	payload := map[string]any{
		"message": message,
		"tree":    treeSHA,
		"parents": parents,
	}
	var resp struct {
		SHA string `json:"sha"`
	}
	if _, err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/git/commits", c.Owner, c.Repo), payload, &resp); err != nil {
		return "", fmt.Errorf("create git commit: %w", err)
	}
	return resp.SHA, nil
}

func (c *Client) createRef(ctx context.Context, commitSHA string) error {
	payload := map[string]string{
		"ref": "refs/heads/" + c.Branch,
		"sha": commitSHA,
	}
	_, err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/git/refs", c.Owner, c.Repo), payload, nil)
	return err
}

func (c *Client) updateRef(ctx context.Context, commitSHA string) error {
	payload := map[string]any{
		"sha":   commitSHA,
		"force": true,
	}
	_, err := c.doJSON(ctx, http.MethodPatch, fmt.Sprintf("/repos/%s/%s/git/refs/heads/%s", c.Owner, c.Repo, c.Branch), payload, nil)
	return err
}

func (c *Client) UpsertPRComment(ctx context.Context, prNumber int, markdownBody string) error {
	var comments []struct {
		ID   int64  `json:"id"`
		Body string `json:"body"`
	}
	if _, err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/issues/%d/comments?per_page=100", c.Owner, c.Repo, prNumber), nil, &comments); err != nil {
		return fmt.Errorf("list PR comments: %w", err)
	}

	var existingID int64
	for _, cm := range comments {
		if strings.Contains(cm.Body, CommentMarker) {
			existingID = cm.ID
			break
		}
	}

	payload := map[string]string{"body": markdownBody}
	if existingID != 0 {
		_, err := c.doJSON(ctx, http.MethodPatch, fmt.Sprintf("/repos/%s/%s/issues/comments/%d", c.Owner, c.Repo, existingID), payload, nil)
		return err
	}
	_, err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/issues/%d/comments", c.Owner, c.Repo, prNumber), payload, nil)
	return err
}

func (c *Client) doJSON(ctx context.Context, method, path string, reqBody any, respOut any) (int, error) {
	base := strings.TrimRight(c.BaseURL, "/")
	if base == "" {
		base = "https://api.github.com"
	}
	var bodyReader io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return 0, err
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, bodyReader)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	hc := c.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("github api %s %s returned %d: %s", method, path, resp.StatusCode, string(respBytes))
	}
	if respOut != nil && len(respBytes) > 0 {
		if err := json.Unmarshal(respBytes, respOut); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}
