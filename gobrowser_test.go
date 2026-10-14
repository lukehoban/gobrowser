package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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

func TestFetchPageWithStylesResolvesBaseAndPreservesCascade(t *testing.T) {
	var stylesheetRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/page":
			_, _ = response.Write([]byte(`<!doctype html><html><head>
				<base href="/assets/">
				<link rel="preload stylesheet" href="css/site.css">
				<style>.titleline a { color: blue; }</style>
			</head><body><table>
				<tr class="athing"><td><span class="titleline"><a href="/story">Linked style story</a></span></td></tr>
				<tr><td class="subtext"><span class="score">1 point</span></td></tr>
			</table></body></html>`))
		case "/assets/css/site.css":
			stylesheetRequests.Add(1)
			_, _ = response.Write([]byte(".titleline a { color: red; font-size: 24px; }"))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	page, stylesheets, err := fetchPageWithStyles(t.Context(), server.URL+"/page")
	if err != nil {
		t.Fatal(err)
	}
	stories, err := parseStoriesWithStyles(page, stylesheets)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := stylesheetRequests.Load(), int32(1); got != want {
		t.Fatalf("stylesheet requests = %d, want %d", got, want)
	}
	if got, want := stories[0].Style.color, (color.RGBA{B: 255, A: 255}); got != want {
		t.Errorf("title color = %#v, want inline blue %#v", got, want)
	}
	if got, want := stories[0].Style.fontSize, float64(24); got != want {
		t.Errorf("title font size = %v, want linked stylesheet size %v", got, want)
	}
}

func TestFetchLinkedStylesheetsRestrictsOriginAndRedirects(t *testing.T) {
	var externalRequests atomic.Int32
	external := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		externalRequests.Add(1)
		_, _ = response.Write([]byte(".titleline a { color: red; }"))
	}))
	defer external.Close()

	var sameOriginRequests atomic.Int32
	var unexpectedRequests atomic.Int32
	sameOrigin := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/redirect.css":
			http.Redirect(response, request, external.URL+"/style.css", http.StatusFound)
		case "/safe.css":
			sameOriginRequests.Add(1)
			_, _ = response.Write([]byte(".titleline a { color: blue; }"))
		default:
			unexpectedRequests.Add(1)
		}
	}))
	defer sameOrigin.Close()
	pageURL, _ := url.Parse(sameOrigin.URL + "/page")
	page := []byte(`<html><head>
		<link rel="stylesheet" href="https://example.invalid/cross.css">
		<link rel="stylesheet" href="/redirect.css">
		<link rel="stylesheet" href="/safe.css">
		<link rel="stylesheet" media="print" href="/print.css">
		<link rel="stylesheet" disabled href="/disabled.css">
		<link rel="stylesheet" type="text/less" href="/less.css">
	</head></html>`)
	stylesheets, err := fetchLinkedStylesheets(t.Context(), page, pageURL)
	if err != nil {
		t.Fatal(err)
	}
	if got := externalRequests.Load(); got != 0 {
		t.Errorf("cross-origin stylesheet requests = %d, want 0", got)
	}
	if got := sameOriginRequests.Load(); got != 1 {
		t.Errorf("same-origin stylesheet requests = %d, want 1", got)
	}
	if got := unexpectedRequests.Load(); got != 0 {
		t.Errorf("requests for unsupported stylesheet links = %d, want 0", got)
	}
	if _, ok := stylesheets["/safe.css"]; !ok {
		t.Errorf("same-origin stylesheet missing from result: %v", stylesheets)
	}
	if _, ok := stylesheets["/redirect.css"]; ok {
		t.Error("stylesheet redirect to a different origin was accepted")
	}
}

func TestFetchLinkedStylesheetsEnforcesCountAndByteLimits(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.URL.Path == "/1.css" {
			_, _ = response.Write([]byte(strings.Repeat("x", maxStylesheetSize+1)))
			return
		}
		_, _ = response.Write([]byte(".titleline a { color: red; }"))
	}))
	defer server.Close()
	pageURL, _ := url.Parse(server.URL + "/page")

	var markup strings.Builder
	markup.WriteString("<html><head>")
	for index := 1; index <= maxStylesheetCount+1; index++ {
		fmt.Fprintf(&markup, `<link rel="stylesheet" href="/%d.css">`, index)
	}
	markup.WriteString("</head></html>")
	stylesheets, err := fetchLinkedStylesheets(t.Context(), []byte(markup.String()), pageURL)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := requests.Load(), int32(maxStylesheetCount); got != want {
		t.Errorf("stylesheet requests = %d, want bounded count %d", got, want)
	}
	if _, ok := stylesheets["/1.css"]; ok {
		t.Error("oversized stylesheet was accepted")
	}
	if len(stylesheets) != maxStylesheetCount-1 {
		t.Errorf("accepted stylesheet count = %d, want %d", len(stylesheets), maxStylesheetCount-1)
	}
}

func TestFetchLinkedStylesheetsEnforcesAggregateByteLimit(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		_, _ = response.Write([]byte(strings.Repeat("a", maxStylesheetSize)))
	}))
	defer server.Close()
	pageURL, _ := url.Parse(server.URL + "/page")

	var markup strings.Builder
	markup.WriteString("<html><head>")
	for index := 0; index < maxStylesheetCount; index++ {
		fmt.Fprintf(&markup, `<link rel="stylesheet" href="/%d.css">`, index)
	}
	markup.WriteString("</head></html>")
	stylesheets, err := fetchLinkedStylesheets(t.Context(), []byte(markup.String()), pageURL)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := requests.Load(), int32(maxStylesheetBytes/maxStylesheetSize); got != want {
		t.Errorf("stylesheet requests = %d, want aggregate-byte-bounded count %d", got, want)
	}
	if len(stylesheets) != maxStylesheetBytes/maxStylesheetSize {
		t.Errorf("accepted stylesheet count = %d, want %d", len(stylesheets), maxStylesheetBytes/maxStylesheetSize)
	}
}

func TestFetchLinkedStylesheetsHonorsOverallTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/slow.css" {
			<-request.Context().Done()
			return
		}
		_, _ = response.Write([]byte(`<html><head><link rel="stylesheet" href="/slow.css"></head></html>`))
	}))
	defer server.Close()
	pageURL, _ := url.Parse(server.URL + "/page")
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()

	started := time.Now()
	stylesheets, err := fetchLinkedStylesheets(ctx,
		[]byte(`<html><head><link rel="stylesheet" href="/slow.css"></head></html>`), pageURL)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Errorf("stylesheet fetching took %s after context deadline", elapsed)
	}
	if len(stylesheets) != 0 {
		t.Errorf("stylesheets = %v, want no sheet after request timeout", stylesheets)
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

	styledInk := colorBounds(renderedStyled, color.RGBA{R: 255, A: 255}, image.Rect(72, 50, 700, renderedStyled.Bounds().Dy()))
	defaultInk := colorBounds(renderedDefault, textColor, image.Rect(72, 50, 700, renderedDefault.Bounds().Dy()))
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

func TestRenderPNGConvertsCSSPointsToPixels(t *testing.T) {
	page, err := os.ReadFile("testdata/hackernews-css.html")
	if err != nil {
		t.Fatal(err)
	}

	renderWithFontSize := func(fontSize string) *image.RGBA {
		t.Helper()
		styledPage := strings.Replace(string(page), "font-size: 24px", "font-size: "+fontSize, 1)
		if styledPage == string(page) {
			t.Fatal("fixture font size was not replaced")
		}
		stories, err := parseStories([]byte(styledPage))
		if err != nil {
			t.Fatal(err)
		}
		if got, want := stories[0].Style.fontSize, float64(16); got != want {
			t.Fatalf("font size %q resolved to %v pixels, want %v", fontSize, got, want)
		}
		return renderStoryForTest(t, stories[0])
	}

	pointSize := renderWithFontSize("12pt")
	pixelSize := renderWithFontSize("16px")
	if pointSize.Bounds() != pixelSize.Bounds() || !bytes.Equal(pointSize.Pix, pixelSize.Pix) {
		t.Error("12pt rendering differs from equivalent 16px rendering")
	}
}

func TestRenderPNGSeparatesLargeCSSStories(t *testing.T) {
	stories := []Story{
		{
			Rank:  "1",
			Title: "Large CSS title",
			Score: "10 points",
			Style: textStyle{color: color.RGBA{R: 255, A: 255}, hasColor: true, fontSize: maxTitleFontSize},
		},
		{Rank: "2", Title: "Next story", Score: "5 points"},
	}
	var output bytes.Buffer
	if err := renderPNG(&output, stories, 800); err != nil {
		t.Fatal(err)
	}
	renderedImage, err := png.Decode(&output)
	if err != nil {
		t.Fatal(err)
	}
	rendered, ok := renderedImage.(*image.RGBA)
	if !ok {
		t.Fatalf("decoded PNG has type %T, want *image.RGBA", renderedImage)
	}

	const pageVerticalSpace = 64 + 18
	if got, minimum := rendered.Bounds().Dy(), pageVerticalSpace+2*rowHeight; got <= minimum {
		t.Fatalf("image height = %d, want more than %d for a large title", got, minimum)
	}
	firstRowHeight := rendered.Bounds().Dy() - pageVerticalSpace - rowHeight
	secondRowTop := 14 + headerHeight + 12 + firstRowHeight

	title := colorBounds(rendered, color.RGBA{R: 255, A: 255}, image.Rect(72, 60, 700, secondRowTop))
	metadata := grayInkBounds(rendered, image.Rect(72, 60, 700, secondRowTop))
	nextTitle := colorBounds(rendered, textColor, image.Rect(72, secondRowTop, 700, rendered.Bounds().Dy()))
	if title.Empty() {
		t.Fatal("large story title was not rendered")
	}
	if metadata.Empty() {
		t.Fatalf("large story metadata was not rendered (image height %d, first row height %d, second row top %d)", rendered.Bounds().Dy(), firstRowHeight, secondRowTop)
	}
	if nextTitle.Empty() {
		t.Fatal("following story title was not rendered")
	}
	if title.Max.Y >= metadata.Min.Y {
		t.Errorf("large story title ends at y=%d, overlapping metadata starting at y=%d", title.Max.Y, metadata.Min.Y)
	}
	if metadata.Max.Y >= nextTitle.Min.Y {
		t.Errorf("large story metadata ends at y=%d, overlapping following title starting at y=%d", metadata.Max.Y, nextTitle.Min.Y)
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

func colorBounds(rendered *image.RGBA, target color.RGBA, area image.Rectangle) image.Rectangle {
	area = area.Intersect(rendered.Bounds())
	bounds := image.Rectangle{}
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
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

func grayInkBounds(rendered *image.RGBA, area image.Rectangle) image.Rectangle {
	area = area.Intersect(rendered.Bounds())
	bounds := image.Rectangle{}
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			pixel := color.RGBAModel.Convert(rendered.At(x, y)).(color.RGBA)
			redGreen := int(pixel.R) - int(pixel.G)
			greenBlue := int(pixel.G) - int(pixel.B)
			if pixel.R < 80 || pixel.R > 200 || redGreen < -5 || redGreen > 5 || greenBlue < -5 || greenBlue > 5 {
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
