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
		TitleStyle: titleStyle{
			Color:    color.RGBA{A: 255},
			FontSize: 15,
		},
	}
	if !reflect.DeepEqual(stories[0], want) {
		t.Errorf("first story = %#v, want %#v", stories[0], want)
	}
}

func TestParseStoriesAppliesCSS(t *testing.T) {
	page, err := os.ReadFile("testdata/hackernews-styled.html")
	if err != nil {
		t.Fatal(err)
	}
	stories, err := parseStories(page)
	if err != nil {
		t.Fatal(err)
	}
	if len(stories) != 1 {
		t.Fatalf("got %d stories, want 1", len(stories))
	}
	want := titleStyle{Color: color.RGBA{R: 255, A: 255}, FontSize: 24}
	if stories[0].TitleStyle != want {
		t.Errorf("title style = %#v, want %#v", stories[0].TitleStyle, want)
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
		_, _ = response.Write([]byte("html"))
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

func TestFetchPageRejectsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		http.Error(response, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	if _, err := fetchPage(t.Context(), server.URL); err == nil {
		t.Fatal("fetchPage succeeded for a non-success status")
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

func TestRenderPNGAppliesCSSColorAndFontSize(t *testing.T) {
	page, err := os.ReadFile("testdata/hackernews-styled.html")
	if err != nil {
		t.Fatal(err)
	}
	stories, err := parseStories(page)
	if err != nil {
		t.Fatal(err)
	}

	var styledOutput bytes.Buffer
	if err := renderPNG(&styledOutput, stories, 800); err != nil {
		t.Fatal(err)
	}
	styledImage, err := png.Decode(&styledOutput)
	if err != nil {
		t.Fatal(err)
	}

	unstyled := stories[0]
	unstyled.TitleStyle = defaultTitleStyle()
	var unstyledOutput bytes.Buffer
	if err := renderPNG(&unstyledOutput, []Story{unstyled}, 800); err != nil {
		t.Fatal(err)
	}
	unstyledImage, err := png.Decode(&unstyledOutput)
	if err != nil {
		t.Fatal(err)
	}

	styledInk, redPixels := titleInk(styledImage)
	unstyledInk, _ := titleInk(unstyledImage)
	if redPixels == 0 {
		t.Error("styled title has no red pixels")
	}
	if styledInk <= unstyledInk {
		t.Errorf("styled title ink pixels = %d, want more than unstyled %d", styledInk, unstyledInk)
	}
}

func titleInk(img image.Image) (ink, red int) {
	background := pageColor
	for y := 52; y < 82; y++ {
		for x := 60; x < 420; x++ {
			pixel := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
			if pixel != background {
				ink++
			}
			if pixel.R > pixel.G*2 && pixel.R > pixel.B*2 {
				red++
			}
		}
	}
	return ink, red
}

func TestRenderPNGRejectsInvalidInput(t *testing.T) {
	if err := renderPNG(&bytes.Buffer{}, nil, 800); err == nil {
		t.Fatal("renderPNG succeeded without stories")
	}
	for _, width := range []int{500, 5000} {
		if err := renderPNG(&bytes.Buffer{}, []Story{{Title: "story"}}, width); err == nil {
			t.Errorf("renderPNG succeeded with unsupported width %d", width)
		}
	}
}
