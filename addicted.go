package addicted

import (
	"bytes"
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
		resp, err := sub.client.Get(fmt.Sprintf("%v%v", baseURL, sub.Link[1:]), true)
		if err != nil {
			return 0, err
		}
		sub.conn = resp.Body
	}
	return sub.conn.Read(p)
}

// Close close subtitle's connection
func (sub *Subtitle) Close() {
	sub.conn.Close()
}

// Client store information for interaction with addic7ed as logged user
type Client struct {
	user        string
	passwd      string
	shows       map[string]string
	httpClient  *http.Client
	isConnected bool
}

// New returns new addicted's client
func New(user, passwd string) (*Client, error) {
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

// Get wrapper function for http.Get which takes care of session's cookies
func (c *Client) Get(url string, auth bool) (resp *http.Response, err error) {
	if auth && !c.isConnected {
		err := c.connect()
		if err != nil {
			return nil, err
		}
		c.isConnected = true
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Referer", baseURL)
	return c.httpClient.Do(req)
}

func (c *Client) connect() error {
	values := url.Values{}
	values.Add("username", c.user)
	values.Add("password", c.passwd)
	values.Add("url", "")
	values.Add("Submit", "Log in")
	req, _ := http.NewRequest("POST", baseURL+"dologin.php", bytes.NewBufferString(values.Encode()))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	root, err := xmlpath.ParseHTML(resp.Body)
	if err != nil {
		return err
	}
	if !xpathCheckLogged.Exists(root) {
		return errors.New("Invalid creditials")
	}
	return nil
}

// GetTvShows returns a map of show's title as keys and ids as values
func (c *Client) GetTvShows() (*map[string]string, error) {
	var err error
	if len(c.shows) == 0 {
		c.shows, err = c.parseShows()
	}
	if err != nil {
		return nil, err
	}
	return &c.shows, nil
}

// GetSubtitles returns available subtitles
func (c *Client) GetSubtitles(showID string, season, episode int) ([]Subtitle, error) {
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
		return nil, err
	}

	shows := map[string]string{}
	iter := xpathTvShowTitle.Iter(root)
	for iter.Next() {
		title := iter.Node().String()
		id, _ := xpathTvShowIDFromTitle.String(iter.Node())
		shows[title] = id
	}
	return shows, nil
}

func (c *Client) parseSubtitle(showID, s, e string) ([]Subtitle, error) {
	resp, err := http.Get(fmt.Sprintf("%vre_episode.php?ep=%v-%vx%v", baseURL, showID, s, e))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	root, err := xmlpath.ParseHTML(resp.Body)
	if err != nil {
		return nil, err
	}
	if !xpathCheckSubtilePage.Exists(root) {
		return nil, errors.New("Show not found")
	}

	sub := []Subtitle{}
	iter := xpathRelease.Iter(root)
	for iter.Next() {
		release := iter.Node().String()
		iterlang := xpathLanguageFromRelease.Iter(iter.Node())
		for iterlang.Next() {
			download, _ := xpathDownloadFromLanguage.String(iterlang.Node())
			downloadText, _ := xpathDownloadCountFromLanguage.String(iterlang.Node())
			downloadText = reDownloadCount.FindAllStringSubmatch(downloadText, 1)[0][1]
			downloadcount, _ := strconv.Atoi(downloadText)
			subtitle := Subtitle{
				Language: strings.TrimSpace(iterlang.Node().String()),
				Download: downloadcount,
				Link:     download,
				Release:  release,
				client:   c,
			}
			sub = append(sub, subtitle)
		}
	}
	return sub, nil

}
