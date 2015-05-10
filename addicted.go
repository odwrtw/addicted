package addicted

import (
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"launchpad.net/xmlpath"
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
	Language    string
	Release     string
	Download    int
	Link        string
	subtitleCon io.ReadCloser
}

func (sub *Subtitle) Read(p []byte) (n int, e error) {
	if sub.subtitleCon == nil {
		client := &http.Client{}
		req, e := http.NewRequest("GET", baseURL+sub.Link[1:], nil)
		req.Header.Add("Referer", baseURL)
		resp, e := client.Do(req)
		if e != nil {
			return 0, e
		}
		sub.subtitleCon = resp.Body
	}
	n, e = sub.subtitleCon.Read(p)
	return
}

// Close close subtitle connection
func (sub *Subtitle) Close() {
	sub.subtitleCon.Close()
}

func getShows() (map[string]string, error) {
	if len(tvShows) != 0 {
		return tvShows, nil
	}
	tvShows, err := parseShows()
	return tvShows, err
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
	var id string
	var title string
	shows := map[string]string{}
	iter := xpathTvShowTitle.Iter(root)
	for iter.Next() {
		title = iter.Node().String()
		id, _ = xpathTvShowIDFromTitle.String(iter.Node())
		shows[title] = id
	}
	return shows, nil
}

func parseSubtitle(showID, s string, e string) ([]Subtitle, error) {
	resp, err := http.Get(baseURL + "re_episode.php?ep=" + showID + "-" + s + "x" + e)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	root, err := xmlpath.ParseHTML(resp.Body)
	if err != nil {
		log.Fatal(err)
	}
	if !xpathCheckSubtilePage.Exists(root) {
		log.Fatal("Not found")
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

// GetTvShows return
func GetTvShows() (*map[string]string, error) {
	shows, err := getShows()
	if err != nil {
		return nil, err
	}
	return &shows, nil
}

// GetSubtitles return available subtitles
func GetSubtitles(showID string, s int, e int) ([]Subtitle, error) {
	season := strconv.Itoa(s)
	episode := strconv.Itoa(e)
	return parseSubtitle(showID, season, episode)
}
