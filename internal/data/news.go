package data

import (
	"encoding/xml"
	"html"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Feed is a named RSS source.
type Feed struct {
	Name string
	URL  string
}

// DefaultFeeds are the news sources fetched by default: upstream Arch news
// (manual-intervention announcements) and CachyOS announcements.
var DefaultFeeds = []Feed{
	{Name: "Arch Linux", URL: "https://archlinux.org/feeds/news/"},
	{Name: "CachyOS", URL: "https://discuss.cachyos.org/c/announcements/5.rss"},
}

type rss struct {
	Channel struct {
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			PubDate     string `xml:"pubDate"`
			Description string `xml:"description"`
		} `xml:"item"`
	} `xml:"channel"`
}

var (
	tagRe = regexp.MustCompile(`<[^>]*>`)
	wsRe  = regexp.MustCompile(`\s+`)
)

// FetchNews retrieves and merges items from the given feeds, newest first.
// Per-feed failures are returned in errs but do not abort the others.
func FetchNews(feeds []Feed, timeout time.Duration, perFeed int) (items []NewsItem, errs []error) {
	client := &http.Client{Timeout: timeout}
	for _, f := range feeds {
		got, err := fetchFeed(client, f, perFeed)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		items = append(items, got...)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Date.After(items[j].Date) })
	return items, errs
}

func fetchFeed(client *http.Client, f Feed, limit int) ([]NewsItem, error) {
	req, err := http.NewRequest(http.MethodGet, f.URL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "arch-update-notes/0.1 (+https://github.com/captainmustard/arch-update-notes)")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}

	var doc rss
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, err
	}

	var out []NewsItem
	for i, it := range doc.Channel.Items {
		if limit > 0 && i >= limit {
			break
		}
		out = append(out, NewsItem{
			Title:   strings.TrimSpace(it.Title),
			Link:    strings.TrimSpace(it.Link),
			Date:    parseDate(it.PubDate),
			Summary: stripHTML(it.Description),
			Source:  f.Name,
		})
	}
	return out, nil
}

func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func stripHTML(s string) string {
	s = tagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = wsRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}
