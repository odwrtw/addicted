package addicted

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

// TestParseOffline tests subtitle parsing against a synthetic episode page fixture (no network).
// Fixture: testdata/episode_page.html — 5 container95m blocks:
//   - 2 English (Completed), 1 French (Completed), 1 German (67% → filtered), 1 Spanish (Completed)
//   - Row 1 has HearingImpaired; rows 3 and 5 use /original/ links; row 4 must be dropped.
func TestParseOffline(t *testing.T) {
	f, err := os.Open("testdata/episode_page.html")
	if err != nil {
		t.Fatalf("open testdata: %v", err)
	}
	defer f.Close()

	c := &Client{}
	subs, err := c.parseSubtitleFromReader(f)
	if err != nil {
		t.Fatalf("parseSubtitleFromReader: %v", err)
	}

	if len(subs) != 4 {
		t.Fatalf("expected 4 subtitles (1 filtered), got %d", len(subs))
	}

	for _, s := range subs {
		t.Logf("lang=%q release=%q downloads=%d hi=%v link=%s",
			s.Language, s.Release, s.Download, s.HearingImpaired, s.Link)
		if s.Language == "" {
			t.Error("empty language")
		}
		if s.Link == "" {
			t.Error("empty link")
		}
		if s.Download <= 0 {
			t.Errorf("expected download count > 0 for %q %q, got %d", s.Language, s.Release, s.Download)
		}
	}

	// Row 1: English LOL must have HI and /updated/ link
	english := subs.FilterByLang("english")
	if len(english) != 2 {
		t.Fatalf("expected 2 english subtitles, got %d", len(english))
	}
	var foundEnglishHI bool
	for _, s := range english {
		if s.Release == "LOL" && s.HearingImpaired {
			if s.Download != 12345 {
				t.Errorf("expected download=12345, got %d", s.Download)
			}
			if s.Link != "/updated/1/10001/1" {
				t.Errorf("unexpected link: %s", s.Link)
			}
			foundEnglishHI = true
		}
	}
	if !foundEnglishHI {
		t.Error("expected english LOL subtitle with HearingImpaired=true")
	}

	// Row 3: French must use /original/ link
	french := subs.FilterByLang("french")
	if len(french) != 1 {
		t.Fatalf("expected 1 french subtitle, got %d", len(french))
	}
	if french[0].Link != "/original/10001/1" {
		t.Errorf("expected /original/ link for french, got %s", french[0].Link)
	}
}

// TestIntegration tests live connectivity. Run with:
//
//	ADDICTED_USER=x ADDICTED_PASS=y go test -v -run TestIntegration
func TestIntegration(t *testing.T) {
	user, pass := os.Getenv("ADDICTED_USER"), os.Getenv("ADDICTED_PASS")
	if user == "" || pass == "" {
		t.Skip("set ADDICTED_USER and ADDICTED_PASS to run integration tests")
	}

	c, err := NewWithAuth(user, pass)
	if err != nil {
		t.Fatalf("NewWithAuth: %v", err)
	}

	showName := "andor"
	season := 1
	episode := 1

	subs, err := c.GetSubtitles(context.Background(), showName, season, episode)
	if err != nil {
		t.Fatalf("GetSubtitles: %v", err)
	}
	if len(subs) == 0 {
		t.Fatal("expected at least one subtitle, got none")
	}
	t.Logf("%s S%02dE%02d: got %d subtitle(s)", showName, season, episode, len(subs))
	for _, s := range subs {
		t.Logf("lang=%q release=%q downloads=%d HI=%t title=%q",
			s.Language, s.Release, s.Download, s.HearingImpaired, s.Title)
	}

	english := subs.FilterByLang("english")
	if len(english) == 0 {
		t.Fatal("no english subtitles found")
	}
	sort.Sort(sort.Reverse(ByDownloads(english)))
	best := english[0]
	t.Logf("downloading most popular english subtitle: release=%q downloads=%d", best.Release, best.Download)

	rc, err := c.Download(context.Background(), best)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer rc.Close()

	scanner := bufio.NewScanner(rc)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// SRT format: skip blank lines, sequence numbers, and timestamp lines
		if line == "" || strings.Contains(line, "-->") {
			continue
		}
		if _, err := fmt.Sscanf(line, "%d", new(int)); err == nil {
			continue // sequence number
		}
		t.Logf("first subtitle line: %q", line)
		break
	}
	if err := scanner.Err(); err != nil {
		t.Errorf("reading subtitle: %v", err)
	}
}
