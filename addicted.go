package addicted

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

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
	tvShows                        map[string]string
)

// Subtitle represent a subtitle
type Subtitle struct {
	Language string
	Release  string
	Download int
	Link     string
	conn     io.ReadCloser
}

// Read subtitle content
func (sub *Subtitle) Read(p []byte) (int, error) {
	if sub.conn == nil {
		req, err := http.NewRequest("GET", fmt.Sprintf("%v%v", baseURL, sub.Link[1:]), nil)
		if err != nil {
			return 0, err
		}
		req.Header.Add("Referer", baseURL)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return 0, err
		}
		sub.conn = resp.Body
	}
	return sub.conn.Read(p)
}

// Close close subtitle connection
func (sub *Subtitle) Close() {
	sub.conn.Close()
}

func getShows() (map[string]string, error) {
	if len(tvShows) != 0 {
		return tvShows, nil
	}
	return parseShows()
}

func parseShows() (map[string]string, error) {
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

func parseSubtitle(showID, s, e string) ([]Subtitle, error) {
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
			}
			sub = append(sub, subtitle)
		}
	}
	return sub, nil

}

// GetTvShows return a map of show's title as keysand ids as values
func GetTvShows() (*map[string]string, error) {
	shows, err := getShows()
	if err != nil {
		return nil, err
	}
	return &shows, nil
}

// GetSubtitles return available subtitles
func GetSubtitles(showID string, season, episode int) ([]Subtitle, error) {
	s := strconv.Itoa(season)
	e := strconv.Itoa(episode)
	return parseSubtitle(showID, s, e)
}
