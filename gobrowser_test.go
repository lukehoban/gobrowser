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
	"strings"
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

func TestParseStoriesAppliesCSS(t *testing.T) {
	page, err := os.ReadFile("testdata/hackernews-css.html")
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
	if got, want := stories[0].Style.color, (color.RGBA{R: 255, A: 255}); got != want {
		t.Errorf("title color = %#v, want %#v", got, want)
	}
	if !stories[0].Style.hasColor {
		t.Error("title CSS color was not applied")
	}
	if got, want := stories[0].Style.fontSize, float64(24); got != want {
		t.Errorf("title font size = %v, want %v", got, want)
	}
}

func TestInlineCSSOverridesStylesheet(t *testing.T) {
	page, err := os.ReadFile("testdata/hackernews-css.html")
	if err != nil {
		t.Fatal(err)
	}
	page = []byte(strings.Replace(string(page),
		`href="https://example.com/css-test"`,
		`href="https://example.com/css-test" style="color: blue; font-size: 18px"`,
		1))

	stories, err := parseStories(page)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := stories[0].Style.color, (color.RGBA{B: 255, A: 255}); got != want {
		t.Errorf("inline title color = %#v, want %#v", got, want)
	}
	if got, want := stories[0].Style.fontSize, float64(18); got != want {
		t.Errorf("inline title font size = %v, want %v", got, want)
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

func TestRenderPNGAppliesCSSPixels(t *testing.T) {
	styledPage, err := os.ReadFile("testdata/hackernews-css.html")
	if err != nil {
		t.Fatal(err)
	}
	styledStories, err := parseStories(styledPage)
	if err != nil {
		t.Fatal(err)
	}
	unstyledStory := styledStories[0]
	unstyledStory.Style = textStyle{}

	renderedStyled := renderStoryForTest(t, styledStories[0])
	renderedDefault := renderStoryForTest(t, unstyledStory)

	styledInk := inkBounds(renderedStyled, color.RGBA{R: 255, A: 255})
	defaultInk := inkBounds(renderedDefault, textColor)
	if styledInk.Empty() {
		t.Fatal("styled title has no red pixels")
	}
	if defaultInk.Empty() {
		t.Fatal("default title has no black pixels")
	}
	if got, want := styledInk.Dy() > defaultInk.Dy(), true; got != want {
		t.Errorf("styled title ink height = %d, default = %d; font-size CSS did not enlarge rendered pixels", styledInk.Dy(), defaultInk.Dy())
	}
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

func renderStoryForTest(t *testing.T, story Story) *image.RGBA {
	t.Helper()
	var output bytes.Buffer
	if err := renderPNG(&output, []Story{story}, 800); err != nil {
		t.Fatal(err)
	}
	rendered, err := png.Decode(&output)
	if err != nil {
		t.Fatal(err)
	}
	rgba, ok := rendered.(*image.RGBA)
	if !ok {
		t.Fatalf("decoded PNG has type %T, want *image.RGBA", rendered)
	}
	return rgba
}

func inkBounds(rendered *image.RGBA, target color.RGBA) image.Rectangle {
	bounds := image.Rectangle{}
	for y := 50; y < 82; y++ {
		for x := 72; x < 700; x++ {
			pixel := color.RGBAModel.Convert(rendered.At(x, y)).(color.RGBA)
			if pixel != target {
				continue
			}
			if bounds.Empty() {
				bounds = image.Rect(x, y, x+1, y+1)
			} else {
				bounds = bounds.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return bounds
}
