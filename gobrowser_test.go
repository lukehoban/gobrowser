package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
)

func TestParseStories(t *testing.T) {
	page, err := os.ReadFile("testdata/hackernews.html")
	if err != nil {
		t.Fatal(err)
	}

	stories, err := parseStories(page)
	if err != nil {
		t.Fatal(err)
	}
	if len(stories) != 8 {
		t.Fatalf("got %d stories, want 8", len(stories))
	}

	want := Story{
		Rank:     "1",
		Title:    "A small Go renderer that draws every pixel",
		Domain:   "example.com",
		Score:    "248 points",
		User:     "gopher",
		Age:      "2 hours ago",
		Comments: "42 comments",
	}
	if !reflect.DeepEqual(stories[0], want) {
		t.Errorf("first story = %#v, want %#v", stories[0], want)
	}
}

func TestParseStoriesRejectsUnknownPage(t *testing.T) {
	if _, err := parseStories([]byte("<html><body>no stories</body></html>")); err == nil {
		t.Fatal("parseStories succeeded without Hacker News story rows")
	}
}

func TestFetchPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("User-Agent") == "" {
			t.Error("request missing user agent")
		}
		response.Write([]byte("html"))
	}))
	defer server.Close()

	page, err := fetchPage(t.Context(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(page) != "html" {
		t.Errorf("page = %q, want html", page)
	}

	if _, err := fetchPage(t.Context(), "file:///etc/passwd"); err == nil {
		t.Fatal("fetchPage accepted a non-HTTP URL")
	}
}

func TestRenderPNG(t *testing.T) {
	var output bytes.Buffer
	err := renderPNG(&output, []Story{{Rank: "1", Title: "A sample story", Domain: "example.com"}}, 800)
	if err != nil {
		t.Fatal(err)
	}

	rendered, err := png.Decode(&output)
	if err != nil {
		t.Fatal(err)
	}
	if got := rendered.Bounds().Size(); got != image.Pt(800, 130) {
		t.Errorf("image size = %v, want (800, 130)", got)
	}
	if got, want := rendered.At(16, 14), color.Color(orangeColor); got != want {
		t.Errorf("header pixel = %v, want %v", got, want)
	}
}

func TestRenderPNGRejectsInvalidInput(t *testing.T) {
	if err := renderPNG(&bytes.Buffer{}, nil, 800); err == nil {
		t.Fatal("renderPNG succeeded without stories")
	}
	if err := renderPNG(&bytes.Buffer{}, []Story{{Title: "story"}}, 500); err == nil {
		t.Fatal("renderPNG succeeded with an unsupported width")
	}
}
