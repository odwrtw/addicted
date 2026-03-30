package addicted

import (
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"golang.org/x/net/publicsuffix"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

var (
	baseURL         = "https://www.addic7ed.com/"
	reDownloadCount = regexp.MustCompile(`(\d+) Downloads`)

	// ErrShowNotFound returned when the show name is not in the show list
	ErrShowNotFound = errors.New("show not found")
	//ErrEpisodeNotFound returned when try to find subtitles for an unknown episode or season or show
	ErrEpisodeNotFound = errors.New("episode not found")
	//ErrUnexpectedContent returned when addic7ed's website seem to have changed
	ErrUnexpectedContent = errors.New("unexpected content")
	// ErrDownloadLimit returned when download limit by day exceeded
	ErrDownloadLimit = errors.New("download count exceeded")
)

// Subtitle represents a subtitle
type Subtitle struct {
	Language        string
	Release         string
	Title           string
	Download        int
	Link            string
	HearingImpaired bool
	conn            io.ReadCloser
	client          *Client
}

// Read subtitle's content
func (sub *Subtitle) Read(p []byte) (int, error) {
	if sub.conn == nil {
		resp, err := sub.client.Get(fmt.Sprintf("%s%s", baseURL, sub.Link[1:]), true)
		if err != nil {
			return 0, err
		}
		if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
			resp.Body.Close()
			return 0, ErrDownloadLimit
		}
		sub.conn = resp.Body
	}
	return sub.conn.Read(p)
}

// Close close subtitle's connection
func (sub *Subtitle) Close() error {
	if sub.conn != nil {
		return sub.conn.Close()
	}

	return nil
}

// ByDownloads helper for sorting
type ByDownloads []Subtitle

func (a ByDownloads) Len() int           { return len(a) }
func (a ByDownloads) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByDownloads) Less(i, j int) bool { return a[i].Download < a[j].Download }

// Subtitles helper for filter subtitle
type Subtitles []Subtitle

// FilterByLang filter by language
func (a Subtitles) FilterByLang(lang string) Subtitles {
	subs := []Subtitle{}
	for _, sub := range a {
		if sub.Language == lang {
			subs = append(subs, sub)
		}
	}
	return Subtitles(subs)
}

// Client store information for interaction with addic7ed as logged user
type Client struct {
	user       string
	passwd     string
	httpClient *http.Client
}

// NewWithAuth returns new addicted's client with authentication
func NewWithAuth(user, passwd string) (*Client, error) {
	c, err := New()
	if err != nil {
		return nil, err
	}
	c.SetCredential(user, passwd)
	return c, nil
}

// New returns new addicted's client
func New() (*Client, error) {
	options := cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	}
	jar, err := cookiejar.New(&options)
	if err != nil {
		return nil, err
	}
	httpClient := http.Client{Jar: jar}
	return &Client{
		httpClient: &httpClient,
	}, nil
}

// SetCredential sets user credentials, persisting them as session cookies.
func (c *Client) SetCredential(user, password string) {
	h := md5.Sum([]byte(password))
	u, _ := url.Parse(baseURL) // constant URL, never fails
	c.httpClient.Jar.SetCookies(u, []*http.Cookie{
		{Name: "wikisubtitlesuser", Value: user},
		{Name: "wikisubtitlespass", Value: fmt.Sprintf("%x", h)},
	})
}

// Get performs a GET request. Credentials are sent automatically via cookies.
// The auth parameter is kept for API compatibility but is a no-op.
func (c *Client) Get(urlStr string, _ bool) (resp *http.Response, err error) {
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", baseURL)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept-Language", "en-US,en;q=1.0")
	return c.httpClient.Do(req)
}

// GetTvShows returns a map of show's title as keys and ids as values
func (c *Client) GetTvShows() (map[string]string, error) {
	resp, err := c.Get(baseURL, false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, ErrUnexpectedContent
	}
	shows := map[string]string{}
	doc.Find("select > option").Each(func(_ int, s *goquery.Selection) {
		id, _ := s.Attr("value")
		name := strings.TrimSpace(s.Text())
		if id != "" && name != "" {
			shows[name] = id
		}
	})
	return shows, nil
}

// GetShowID returns the numeric ID for the named show (case-insensitive).
func (c *Client) GetShowID(name string) (string, error) {
	_, id, err := c.findShow(name)
	return id, err
}

// findShow returns the canonical show name and numeric ID for a case-insensitive match.
func (c *Client) findShow(name string) (canonical, id string, err error) {
	shows, err := c.GetTvShows()
	if err != nil {
		return "", "", err
	}
	needle := strings.ToLower(name)
	for show, id := range shows {
		if strings.ToLower(show) == needle {
			return show, id, nil
		}
	}
	return "", "", ErrShowNotFound
}

// GetSubtitles returns available subtitles for the named show.
func (c *Client) GetSubtitles(showName string, season, episode int) (Subtitles, error) {
	return c.parseSubtitle(showName, season, episode)
}

func (c *Client) parseSubtitle(showName string, season, episode int) (Subtitles, error) {
	// Resolve canonical name and ID (case-insensitive, canonical name used in URL)
	canonicalName, showID, err := c.findShow(showName)
	if err != nil {
		return nil, err
	}

	// Get episode title for URL construction
	episodeTitle, err := c.getEpisodeTitle(showID, season, episode)
	if err != nil {
		return nil, err
	}

	// Fetch episode page
	epURL := fmt.Sprintf("%sserie/%s/%d/%d/%s",
		baseURL,
		url.PathEscape(canonicalName),
		season,
		episode,
		url.PathEscape(episodeTitle),
	)
	resp, err := c.Get(epURL, true)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	subs, err := c.parseSubtitleFromReader(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(subs) == 0 {
		return nil, ErrEpisodeNotFound
	}

	for i := range subs {
		subs[i].Title = episodeTitle
	}
	return subs, nil
}

func (c *Client) getEpisodeTitle(showID string, season, episode int) (string, error) {
	u := fmt.Sprintf("%sajax_getEpisodes.php?showID=%s&season=%d", baseURL, showID, season)
	resp, err := c.Get(u, true)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", ErrUnexpectedContent
	}

	var title string
	doc.Find("option").Each(func(_ int, s *goquery.Selection) {
		numStr, rest, ok := strings.Cut(strings.TrimSpace(s.Text()), ". ")
		if !ok {
			return
		}
		n, err := strconv.Atoi(numStr)
		if err != nil {
			return
		}
		if n == episode {
			title = rest
		}
	})

	if title == "" {
		return "", ErrEpisodeNotFound
	}
	return title, nil
}

func (c *Client) parseSubtitleFromReader(r io.Reader) (Subtitles, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, ErrUnexpectedContent
	}

	var subs []Subtitle
	doc.Find("#container95m").Each(func(_ int, container *goquery.Selection) {
		// Release name from "Version X, Duration: Y"
		titleText := strings.TrimSpace(container.Find("td.NewsTitle").First().Text())
		release, _, _ := strings.Cut(strings.TrimPrefix(titleText, "Version "), ", Duration:")
		release = strings.TrimSpace(release)

		container.Find("td.language").Each(func(_ int, langTd *goquery.Selection) {
			status := strings.TrimSpace(langTd.Next().Text())
			if status != "Completed" {
				return
			}

			lang := strings.ToLower(strings.TrimSpace(langTd.Text()))
			if lang == "" {
				return
			}

			row := langTd.Parent()

			link, exists := row.Find(`a[href*="/updated/"]`).First().Attr("href")
			if !exists {
				link, exists = row.Find(`a[href*="/original/"]`).First().Attr("href")
				if !exists {
					return
				}
			}

			nextRow := row.Next()
			hi := nextRow.Find(`img[title="Hearing Impaired"]`).Length() > 0

			var downloads int
			if m := reDownloadCount.FindStringSubmatch(nextRow.Find("td.newsDate").Text()); len(m) > 1 {
				downloads, _ = strconv.Atoi(m[1])
			}

			subs = append(subs, Subtitle{
				Language:        lang,
				Release:         release,
				Download:        downloads,
				Link:            link,
				HearingImpaired: hi,
				client:          c,
			})
		})
	})
	return Subtitles(subs), nil
}
