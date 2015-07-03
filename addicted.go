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

	"golang.org/x/net/publicsuffix"

	"gopkg.in/xmlpath.v2"
)

var (
	baseURL                        = "http://www.addic7ed.com/"
	reDownloadCount                = regexp.MustCompile("(\\d+) Downloads")
	xpathTvShowIDFromTitle         = xmlpath.MustCompile("./@value")
	xpathTvShowTitle               = xmlpath.MustCompile("//*[@id=\"qsShow\"]/option")
	xpathRelease                   = xmlpath.MustCompile("//div[@id=\"container95m\"]//td[@class=\"NewsTitle\"]")
	xpathLanguageFromRelease       = xmlpath.MustCompile("../..//td[@class=\"language\"]")
	xpathDownloadFromLanguage      = xmlpath.MustCompile("..//a[@class=\"buttonDownload\"]/@href")
	xpathDownloadCountFromLanguage = xmlpath.MustCompile("../following-sibling::tr[1]/td[1]")
	xpathCheckSubtilePage          = xmlpath.MustCompile("//div[@id=\"container\"]")
	xpathCheckLogged               = xmlpath.MustCompile("//a[@href=\"/logout.php\"]")
	//ErrNoCreditial returned when attempt to login without creditial set
	ErrNoCreditial = errors.New("No creditial provided")
	//ErrInvalidCredential returned when login failed
	ErrInvalidCredential = errors.New("Invalid creditial")
	//ErrEpisodeNotFound returned when try to find subtitles for an unknow episode or season or show
	ErrEpisodeNotFound = errors.New("Episode not found")
	//ErrUnexpectedContent returned when addic7ed's website seem to have change
	ErrUnexpectedContent = errors.New("Unexpected content")
	// ErrDownloadLimit retuned when download limit by day exceeded
	ErrDownloadLimit = errors.New("Download count exceeded")
)

// Subtitle represents a subtitle
type Subtitle struct {
	Language string
	Release  string
	Download int
	Link     string
	conn     io.ReadCloser
	client   *Client
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
func (sub *Subtitle) Close() {
	sub.conn.Close()
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

// NewWithAuth returns new addicted's client with autentification
func NewWithAuth(user, passwd string) (*Client, error) {
	options := cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	}
	jar, err := cookiejar.New(&options)
	if err != nil {
		return nil, err
	}
	httpClient := http.Client{Jar: jar}
	return &Client{user, passwd, nil, &httpClient, false}, nil
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
	return &Client{httpClient: &httpClient}, nil
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
	req.Header.Add("User-Agent", "Mozilla/5.0 (Windows; U; Windows NT 6.1; fr; rv:1.9.0.6) Gecko/2009011913 Firefox/3.0.6")
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
	root, err := xmlpath.ParseHTML(resp.Body)
	if err != nil {
		return ErrUnexpectedContent
	}
	if !xpathCheckLogged.Exists(root) {
		return ErrInvalidCredential
	}
	return nil
}

// GetTvShows returns a map of show's title as keys and ids as values
func (c *Client) GetTvShows() (map[string]string, error) {
	if len(c.shows) == 0 {
		var err error
		c.shows, err = c.parseShows()
		if err != nil {
			return nil, err
		}
	}
	return c.shows, nil
}

// GetSubtitles returns available subtitles
func (c *Client) GetSubtitles(showID string, season, episode int) (Subtitles, error) {
	s := strconv.Itoa(season)
	e := strconv.Itoa(episode)
	return c.parseSubtitle(showID, s, e)
}

func (c *Client) parseShows() (map[string]string, error) {
	resp, err := http.Get(baseURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	root, err := xmlpath.ParseHTML(resp.Body)
	if err != nil {
		return nil, ErrUnexpectedContent
	}

	shows := map[string]string{}
	iter := xpathTvShowTitle.Iter(root)
	for iter.Next() {
		title := iter.Node().String()
		id, ok := xpathTvShowIDFromTitle.String(iter.Node())
		if !ok {
			return nil, ErrUnexpectedContent
		}
		shows[title] = id
	}
	return shows, nil
}

func (c *Client) parseSubtitle(showID, s, e string) (Subtitles, error) {
	resp, err := http.Get(fmt.Sprintf("%vre_episode.php?ep=%s-%sx%s", baseURL, showID, s, e))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	root, err := xmlpath.ParseHTML(resp.Body)
	if err != nil {
		return nil, ErrUnexpectedContent
	}
	if !xpathCheckSubtilePage.Exists(root) {
		return nil, ErrEpisodeNotFound
	}

	sub := []Subtitle{}
	iter := xpathRelease.Iter(root)
	for iter.Next() {
		release := iter.Node().String()
		iterlang := xpathLanguageFromRelease.Iter(iter.Node())
		for iterlang.Next() {
			download, ok := xpathDownloadFromLanguage.String(iterlang.Node())
			if !ok {
				return nil, ErrUnexpectedContent
			}
			downloadText, ok := xpathDownloadCountFromLanguage.String(iterlang.Node())
			if !ok {
				return nil, ErrUnexpectedContent
			}
			refound := reDownloadCount.FindAllStringSubmatch(downloadText, 1)
			if len(refound) == 0 || len(refound[0]) == 0 {
				return nil, ErrUnexpectedContent
			}
			downloadcount, err := strconv.Atoi(refound[0][1])
			if err != nil {
				return nil, ErrUnexpectedContent
			}
			subtitle := Subtitle{
				Language: strings.ToLower(strings.TrimSpace(iterlang.Node().String())),
				Download: downloadcount,
				Link:     download,
				Release:  release,
				client:   c,
			}
			sub = append(sub, subtitle)
		}
	}
	return Subtitles(sub), nil

}
