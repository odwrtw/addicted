package addicted

import (
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
	releaseRe       = regexp.MustCompile(`Version ([-\(\)\.\w]+),`)

	// ErrNoCredential returned when attempt to login without credential set
	ErrNoCredential = errors.New("no credential provided")
	// ErrInvalidCredential returned when login failed
	ErrInvalidCredential = errors.New("invalid credential")
	//ErrEpisodeNotFound returned when try to find subtitles for an unknow episode or season or show
	ErrEpisodeNotFound = errors.New("episode not found")
	//ErrUnexpectedContent returned when addic7ed's website seem to have change
	ErrUnexpectedContent = errors.New("unexpected content")
	// ErrDownloadLimit retuned when download limit by day exceeded
	ErrDownloadLimit = errors.New("download count exceeded")
)

// Subtitle represents a subtitle
type Subtitle struct {
	Language string
	Release  string
	Download int
	Link     string
	conn io.ReadCloser
	client *Client
}

// Read subtitle's content
func (sub *Subtitle) Read(p []byte) (int, error) {
	if sub.conn == nil {
		resp, err := sub.client.Get(fmt.Sprintf("%s%s", baseURL, sub.Link[1:]), true)
		if err != nil {
			return 0, err
		}
		if resp.Request.URL.Path == "/downloadexceeded.php" {
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
	user        string
	passwd      string
	shows       map[string]string
	httpClient  *http.Client
	isConnected bool
}

// NewWithAuth returns new addicted's client with authentication
func NewWithAuth(user, passwd string) (*Client, error) {
	c, err := New()
	if err != nil {
		return nil, err
	}
	c.user = user
	c.passwd = passwd
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

// SetCredential set user password for addicted client
func (c *Client) SetCredential(user, password string) {
	c.user = user
	c.passwd = password
}

// Get wrapper function for http.Get which takes care of session's cookies
func (c *Client) Get(url string, auth bool) (resp *http.Response, err error) {
	if auth && !c.isConnected {
		if c.user != "" && c.passwd != "" {
			if err := c.connect(); err != nil {
				return nil, err
			}
			c.isConnected = true
		}
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Referer", baseURL)
	req.Header.Add("User-Agent", userAgent)
	return c.httpClient.Do(req)
}

func (c *Client) connect() error {
	values := url.Values{}
	values.Add("username", c.user)
	values.Add("password", c.passwd)
	values.Add("url", "")
	values.Add("Submit", "Log in")
	resp, err := c.httpClient.PostForm(baseURL+"dologin.php", values)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return ErrUnexpectedContent
	}
	if doc.Find(`a[href="/logout.php"]`).Length() == 0 {
		return ErrInvalidCredential
	}
	return nil
}

// GetTvShows returns a map of show's title as keys and ids as values
func (c *Client) GetTvShows() (map[string]string, error) {
	if len(c.shows) > 0 {
		return c.shows, nil
	}
	resp, err := c.Get(baseURL, false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, ErrUnexpectedContent
	}
	c.shows = map[string]string{}
	doc.Find("select > option").Each(func(_ int, s *goquery.Selection) {
		id, _ := s.Attr("value")
		name := strings.TrimSpace(s.Text())
		if id != "" && name != "" {
			c.shows[name] = id
		}
	})
	return c.shows, nil
}

// GetSubtitles returns available subtitles
func (c *Client) GetSubtitles(showID string, season, episode int) (Subtitles, error) {
	s := strconv.Itoa(season)
	e := strconv.Itoa(episode)
	return c.parseSubtitle(showID, s, e)
}

func (c *Client) parseSubtitle(showID, s, e string) (Subtitles, error) {
	resp, err := c.Get(fmt.Sprintf("%sre_episode.php?ep=%s-%sx%s", baseURL, showID, s, e), false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return c.parseSubtitleFromReader(resp.Body)
}

func (c *Client) parseSubtitleFromReader(r io.Reader) (Subtitles, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, ErrUnexpectedContent
	}
	if doc.Find("#container").Length() == 0 {
		return nil, ErrEpisodeNotFound
	}

	var subs []Subtitle
	doc.Find("#container95m").Each(func(_ int, releaseDiv *goquery.Selection) {
		releaseStr := releaseDiv.Find("td.NewsTitle").First().Text()
		releaseM := releaseRe.FindStringSubmatch(releaseStr)
		release := ""
		if len(releaseM) > 1 {
			release = releaseM[1]
		}

		releaseDiv.Find("td.language").Each(func(_ int, langTd *goquery.Selection) {
			row := langTd.Parent()
			link, exists := row.Find(`a[href*="/updated/"]`).Attr("href")
			if !exists {
				return // skip incomplete/untranslated
			}

			countText := row.Next().Find("td").First().Text()
			matches := reDownloadCount.FindStringSubmatch(countText)
			if len(matches) < 2 {
				return
			}
			downloadCount, err := strconv.Atoi(matches[1])
			if err != nil {
				return
			}

			lang := strings.ToLower(strings.TrimSpace(langTd.Text()))
			subs = append(subs, Subtitle{
				Language: lang,
				Download: downloadCount,
				Link:     link,
				Release:  release,
				client:   c,
			})
		})
	})
	return Subtitles(subs), nil
}
