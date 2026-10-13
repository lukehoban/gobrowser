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

func TestParseStoriesAppliesTitleCSS(t *testing.T) {
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
	if !stories[0].HasTitleColor || stories[0].TitleColor != (color.RGBA{R: 255, A: 255}) {
		t.Errorf("title color = %#v (set %t), want red", stories[0].TitleColor, stories[0].HasTitleColor)
	}
	if stories[0].TitleFontSize != 24 {
		t.Errorf("title font size = %d, want 24", stories[0].TitleFontSize)
	}
}

func TestInlineTitleCSSOverridesStylesheet(t *testing.T) {
	page := []byte(`<style>a { color: blue; font-size: 12px; } .titleline a { color: red; font-size: 24px; }</style>
<table>
<tr class="athing"><td class="title"><span class="titleline"><a>A story</a></span></td></tr>
<tr class="athing"><td class="title"><span class="titleline"><a style="color: green; font-size: 18px">Another story</a></span></td></tr>
</table>`)
	stories, err := parseStories(page)
	if err != nil {
		t.Fatal(err)
	}
	if len(stories) != 2 {
		t.Fatalf("got %d stories, want 2", len(stories))
	}
	if got, want := stories[0].TitleColor, (color.RGBA{R: 255, A: 255}); got != want {
		t.Errorf("stylesheet title color = %#v, want %#v", got, want)
	}
	if stories[0].TitleFontSize != 24 {
		t.Errorf("stylesheet title font size = %d, want 24", stories[0].TitleFontSize)
	}
	if got, want := stories[1].TitleColor, (color.RGBA{G: 128, A: 255}); got != want {
		t.Errorf("inline title color = %#v, want %#v", got, want)
	}
	if stories[1].TitleFontSize != 18 {
		t.Errorf("inline title font size = %d, want 18", stories[1].TitleFontSize)
	}
}

func TestParseCSSValues(t *testing.T) {
	for _, test := range []struct {
		input string
		want  color.RGBA
	}{
		{input: "red", want: color.RGBA{R: 255, A: 255}},
		{input: "#0f0", want: color.RGBA{G: 255, A: 255}},
		{input: "rgb(1, 2, 3)", want: color.RGBA{R: 1, G: 2, B: 3, A: 255}},
	} {
		got, ok := parseCSSColor(test.input)
		if !ok || got != test.want {
			t.Errorf("parseCSSColor(%q) = %#v, %t; want %#v, true", test.input, got, ok, test.want)
		}
	}
	if got, ok := parseCSSFontSize("12pt"); !ok || got != 16 {
		t.Errorf("parseCSSFontSize(12pt) = %d, %t; want 16, true", got, ok)
	}
	if _, ok := parseCSSFontSize("48px"); ok {
		t.Error("parseCSSFontSize(48px) accepted a size over the supported limit")
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

func TestRenderPNGAppliesTitleCSS(t *testing.T) {
	page, err := os.ReadFile("testdata/hackernews-styled.html")
	if err != nil {
		t.Fatal(err)
	}
	stories, err := parseStories(page)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := renderPNG(&output, stories, 800); err != nil {
		t.Fatal(err)
	}
	rendered, err := png.Decode(&output)
	if err != nil {
		t.Fatal(err)
	}

	redPixels := 0
	for y := 0; y < rendered.Bounds().Dy(); y++ {
		for x := 0; x < rendered.Bounds().Dx(); x++ {
			r, g, b, _ := rendered.At(x, y).RGBA()
			if r > 12_000 && r > g*2 && r > b*2 {
				redPixels++
			}
		}
	}
	if redPixels == 0 {
		t.Fatal("rendered image contains no red title pixels")
	}

	var defaultOutput bytes.Buffer
	if err := renderPNG(&defaultOutput, []Story{{Rank: "1", Title: stories[0].Title}}, 800); err != nil {
		t.Fatal(err)
	}
	defaultImage, err := png.Decode(&defaultOutput)
	if err != nil {
		t.Fatal(err)
	}
	if rendered.Bounds().Dy() <= defaultImage.Bounds().Dy() {
		t.Errorf("styled image height = %d, want taller than default %d for 24px title", rendered.Bounds().Dy(), defaultImage.Bounds().Dy())
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
