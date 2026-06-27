package data

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Release is upstream release notes for a specific version.
type Release struct {
	Title string
	URL   string
	Body  string
}

// Reference is everything we could discover about "what changed" for a package
// when it has no local changelog: an interpretation of the version delta, the
// upstream homepage, upstream release notes (best effort), and links/commits
// for the packaging source.
type Reference struct {
	VersionNote      string   // human summary of the version delta
	IsRebuild        bool     // pkgrel/epoch changed but upstream version did not
	UpstreamURL      string   // from `pacman -Qi` URL field
	Release          *Release // upstream release notes, if found
	PackagingLabel   string   // "Arch packaging" or "AUR"
	PackagingURL     string   // link to the PKGBUILD source
	PackagingCommits []string // recent packaging commit subjects (Arch GitLab)
	CachyOSURL       string   // CachyOS PKGBUILDs reference, if built by CachyOS
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

// GatherReferences collects fallback "what changed" information for a package.
// Network lookups are skipped when online is false.
func GatherReferences(c PackageChange, online bool) Reference {
	ref := Reference{}
	ref.VersionNote, ref.IsRebuild = classifyVersion(c)

	info := queryInfo(c.Name)
	ref.UpstreamURL = info["URL"]
	repo := info["Installed From"]

	base := pkgBase(c.Name)
	if foreign(c.Name) {
		ref.PackagingLabel = "AUR"
		ref.PackagingURL = "https://aur.archlinux.org/packages/" + c.Name
	} else {
		ref.PackagingLabel = "Arch packaging"
		ref.PackagingURL = "https://gitlab.archlinux.org/archlinux/packaging/packages/" + base
		if strings.Contains(strings.ToLower(repo), "cachyos") {
			ref.CachyOSURL = "https://github.com/CachyOS/CachyOS-PKGBUILDs"
		}
	}

	if !online {
		return ref
	}

	// Upstream release notes, only meaningful for an actual upstream change.
	if !ref.IsRebuild {
		if up := newUpstreamVersion(c); up != "" {
			if rel := fetchRelease(ref.UpstreamURL, up); rel != nil {
				ref.Release = rel
			}
		}
	}

	// Packaging commit subjects from the Arch GitLab (works even for
	// CachyOS-built packages, which track the same packaging).
	if ref.PackagingLabel == "Arch packaging" {
		ref.PackagingCommits = fetchArchPackagingCommits(base)
	}

	return ref
}

// --- version handling ---

type evr struct{ epoch, ver, rel string }

func parseEVR(s string) evr {
	var e evr
	e.epoch = "0"
	if i := strings.IndexByte(s, ':'); i >= 0 {
		e.epoch = s[:i]
		s = s[i+1:]
	}
	if i := strings.IndexByte(s, '-'); i >= 0 {
		e.ver = s[:i]
		e.rel = s[i+1:]
	} else {
		e.ver = s
	}
	return e
}

func classifyVersion(c PackageChange) (string, bool) {
	switch c.Action {
	case Installed:
		return "Newly installed at " + c.NewVersion + ".", false
	case Removed:
		return "Removed (was " + c.OldVersion + ").", false
	}
	o, n := parseEVR(c.OldVersion), parseEVR(c.NewVersion)
	if o.ver == n.ver {
		switch {
		case o.epoch != n.epoch:
			return "Epoch change (" + c.OldVersion + " → " + c.NewVersion + "); same upstream version — a packaging change.", true
		default:
			return "Rebuild only: upstream version unchanged (" + n.ver + "), pkgrel " + o.rel + " → " + n.rel + ". No upstream code change — usually a rebuild against an updated dependency or a distro patch.", true
		}
	}
	return "Upstream version " + o.ver + " → " + n.ver + ".", false
}

// newUpstreamVersion returns the pkgver of the new package, for tag matching.
func newUpstreamVersion(c PackageChange) string {
	return parseEVR(c.NewVersion).ver
}

// --- local pacman queries ---

func queryInfo(pkg string) map[string]string {
	out, err := exec.Command("pacman", "-Qi", pkg).Output()
	m := map[string]string{}
	if err != nil {
		return m
	}
	for _, line := range strings.Split(string(out), "\n") {
		i := strings.Index(line, ":")
		if i < 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		val := strings.TrimSpace(line[i+1:])
		if key != "" {
			m[key] = val
		}
	}
	return m
}

func pkgBase(pkg string) string {
	out, err := exec.Command("expac", "%e", pkg).Output()
	if err != nil {
		return pkg
	}
	base := strings.TrimSpace(string(out))
	if base == "" || base == "(null)" {
		return pkg
	}
	return base
}

var (
	foreignOnce sync.Once
	foreignSet  map[string]struct{}
)

func foreign(pkg string) bool {
	foreignOnce.Do(func() {
		foreignSet = map[string]struct{}{}
		out, err := exec.Command("pacman", "-Qmq").Output()
		if err != nil {
			return
		}
		for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if l = strings.TrimSpace(l); l != "" {
				foreignSet[l] = struct{}{}
			}
		}
	})
	_, ok := foreignSet[pkg]
	return ok
}

// --- forge release notes ---

type forge struct {
	host  string // api host
	owner string
	repo  string
	kind  string // "github" | "gitlab"
}

func detectForge(homepage string) (forge, bool) {
	u, err := url.Parse(homepage)
	if err != nil || u.Host == "" {
		return forge{}, false
	}
	parts := strings.FieldsFunc(u.Path, func(r rune) bool { return r == '/' })
	if len(parts) < 2 {
		return forge{}, false
	}
	owner, repo := parts[0], strings.TrimSuffix(parts[1], ".git")
	host := strings.ToLower(u.Host)
	switch {
	case host == "github.com":
		return forge{host: "api.github.com", owner: owner, repo: repo, kind: "github"}, true
	case host == "gitlab.com" || strings.HasPrefix(host, "gitlab."):
		return forge{host: u.Host, owner: owner, repo: repo, kind: "gitlab"}, true
	}
	return forge{}, false
}

func fetchRelease(homepage, upstreamVer string) *Release {
	f, ok := detectForge(homepage)
	if !ok {
		return nil
	}
	tags := []string{upstreamVer, "v" + upstreamVer}
	// also try replacing underscores (some pkgver use _ for upstream -)
	if strings.Contains(upstreamVer, "_") {
		alt := strings.ReplaceAll(upstreamVer, "_", "-")
		tags = append(tags, alt, "v"+alt)
	}
	for _, tag := range tags {
		var rel *Release
		switch f.kind {
		case "github":
			rel = githubRelease(f, tag)
		case "gitlab":
			rel = gitlabRelease(f, tag)
		}
		if rel != nil {
			return rel
		}
	}
	return nil
}

func getJSON(rawURL string, v any) bool {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "arch-update-notes/0.1")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return false
	}
	return json.Unmarshal(body, v) == nil
}

func githubRelease(f forge, tag string) *Release {
	var r struct {
		Name    string `json:"name"`
		HTMLURL string `json:"html_url"`
		Body    string `json:"body"`
		TagName string `json:"tag_name"`
	}
	api := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s",
		f.owner, f.repo, url.PathEscape(tag))
	if !getJSON(api, &r) {
		return nil
	}
	title := r.Name
	if title == "" {
		title = r.TagName
	}
	return &Release{Title: title, URL: r.HTMLURL, Body: trimBody(r.Body)}
}

func gitlabRelease(f forge, tag string) *Release {
	var r struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Links       struct {
			Self string `json:"self"`
		} `json:"_links"`
	}
	proj := url.PathEscape(f.owner + "/" + f.repo)
	api := fmt.Sprintf("https://%s/api/v4/projects/%s/releases/%s",
		f.host, proj, url.PathEscape(tag))
	if !getJSON(api, &r) {
		return nil
	}
	title := r.Name
	if title == "" {
		title = tag
	}
	return &Release{Title: title, URL: r.Links.Self, Body: trimBody(r.Description)}
}

func fetchArchPackagingCommits(base string) []string {
	var commits []struct {
		Title string `json:"title"`
	}
	proj := url.PathEscape("archlinux/packaging/packages/" + base)
	api := "https://gitlab.archlinux.org/api/v4/projects/" + proj + "/repository/commits?per_page=5"
	if !getJSON(api, &commits) {
		return nil
	}
	var out []string
	for _, c := range commits {
		if t := strings.TrimSpace(c.Title); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func trimBody(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSpace(s)
	const max = 2000
	if len(s) > max {
		s = s[:max] + "\n…"
	}
	return s
}
