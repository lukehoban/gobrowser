package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	maxPageSize                = 8 << 20
	maxStylesheetCount         = 8
	maxStylesheetSize          = 1 << 20
	maxStylesheetBytes         = 4 << 20
	maxStylesheetFetchDuration = 20 * time.Second
	maxHTTPRedirects           = 10
)

type Story struct {
	Rank     string
	Title    string
	Domain   string
	Score    string
	User     string
	Age      string
	Comments string
	Style    textStyle
}

func fetchPage(ctx context.Context, rawURL string) ([]byte, error) {
	page, _, err := fetchPageDocument(ctx, rawURL)
	return page, err
}

func fetchPageWithStyles(ctx context.Context, rawURL string) ([]byte, map[string]string, error) {
	page, pageURL, err := fetchPageDocument(ctx, rawURL)
	if err != nil {
		return nil, nil, err
	}
	stylesheets, err := fetchLinkedStylesheets(ctx, page, pageURL)
	if err != nil {
		return nil, nil, err
	}
	return page, stylesheets, nil
}

func fetchPageDocument(ctx context.Context, rawURL string) ([]byte, *url.URL, error) {
	parsedURL, err := parseHTTPURL(rawURL)
	if err != nil {
		return nil, nil, fmt.Errorf("parse URL: %w", err)
	}

	client := &http.Client{Timeout: maxStylesheetFetchDuration}
	page, finalURL, err := fetchHTTPResource(ctx, client, parsedURL.String(), maxPageSize, "page")
	if err != nil {
		return nil, nil, err
	}
	return page, finalURL, nil
}

func fetchHTTPResource(ctx context.Context, client *http.Client, rawURL string, maxBytes int, resource string) ([]byte, *url.URL, error) {
	parsedURL, err := parseHTTPURL(rawURL)
	if err != nil {
		return nil, nil, err
	}
	if maxBytes < 0 {
		return nil, nil, fmt.Errorf("invalid %s size limit", resource)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("create %s request: %w", resource, err)
	}
	request.Header.Set("User-Agent", "gobrowser/0.1 (+https://github.com/lukehoban/gobrowser)")

	response, err := client.Do(request)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch %s: %w", resource, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, nil, fmt.Errorf("fetch %s: HTTP %s", resource, response.Status)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, int64(maxBytes)+1))
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", resource, err)
	}
	if len(body) > maxBytes {
		return nil, nil, fmt.Errorf("%s exceeds %d bytes", resource, maxBytes)
	}
	finalURL := response.Request.URL
	if finalURL == nil {
		finalURL = parsedURL
	}
	finalURLCopy := *finalURL
	return body, &finalURLCopy, nil
}

func parseHTTPURL(rawURL string) (*url.URL, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		return nil, fmt.Errorf("URL must be an absolute HTTP or HTTPS URL")
	}
	return parsedURL, nil
}

type linkedStylesheet struct {
	href string
	url  *url.URL
}

func fetchLinkedStylesheets(ctx context.Context, page []byte, pageURL *url.URL) (map[string]string, error) {
	document, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return nil, fmt.Errorf("parse HTML for linked stylesheets: %w", err)
	}
	links := findLinkedStylesheets(document, pageURL)
	stylesheets := make(map[string]string)
	if len(links) == 0 {
		return stylesheets, nil
	}

	fetchContext, cancel := context.WithTimeout(ctx, maxStylesheetFetchDuration)
	defer cancel()
	client := &http.Client{
		Timeout: maxStylesheetFetchDuration,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= maxHTTPRedirects {
				return fmt.Errorf("stopped after %d stylesheet redirects", maxHTTPRedirects)
			}
			if !sameOrigin(pageURL, request.URL) {
				return fmt.Errorf("stylesheet redirect is outside the page origin")
			}
			return nil
		},
	}

	totalBytes := 0
	for _, link := range links {
		if !sameOrigin(pageURL, link.url) {
			continue
		}
		remaining := maxStylesheetBytes - totalBytes
		if remaining <= 0 {
			break
		}
		limit := min(maxStylesheetSize, remaining)
		body, _, err := fetchHTTPResource(fetchContext, client, link.url.String(), limit, "stylesheet")
		if err != nil {
			if fetchContext.Err() != nil {
				break
			}
			continue
		}
		stylesheets[link.href] = string(body)
		totalBytes += len(body)
	}
	return stylesheets, nil
}

func findLinkedStylesheets(document *html.Node, pageURL *url.URL) []linkedStylesheet {
	baseURL := documentBaseURL(document, pageURL)
	var links []linkedStylesheet
	for node := document; node != nil; node = nextNode(node) {
		if node.Type != html.ElementNode || node.Data != "link" || !isStylesheetLink(node) {
			continue
		}
		href := strings.TrimSpace(attribute(node, "href"))
		if href == "" {
			continue
		}
		reference, err := url.Parse(href)
		if err != nil {
			continue
		}
		resolved := baseURL.ResolveReference(reference)
		if _, err := parseHTTPURL(resolved.String()); err != nil || resolved.User != nil {
			continue
		}
		links = append(links, linkedStylesheet{href: href, url: resolved})
		if len(links) == maxStylesheetCount {
			break
		}
	}
	return links
}

func documentBaseURL(document *html.Node, pageURL *url.URL) *url.URL {
	for node := document; node != nil; node = nextNode(node) {
		if node.Type != html.ElementNode || node.Data != "base" {
			continue
		}
		href := strings.TrimSpace(attribute(node, "href"))
		if href == "" {
			continue
		}
		reference, err := url.Parse(href)
		if err != nil {
			continue
		}
		resolved := pageURL.ResolveReference(reference)
		if _, err := parseHTTPURL(resolved.String()); err == nil {
			return resolved
		}
	}
	return pageURL
}

func isStylesheetLink(node *html.Node) bool {
	if attributePresent(node, "disabled") {
		return false
	}
	hasStylesheet := false
	for _, rel := range strings.Fields(attribute(node, "rel")) {
		switch strings.ToLower(rel) {
		case "stylesheet":
			hasStylesheet = true
		case "alternate":
			return false
		}
	}
	if !hasStylesheet {
		return false
	}
	linkType := strings.TrimSpace(strings.SplitN(attribute(node, "type"), ";", 2)[0])
	if linkType != "" && !strings.EqualFold(linkType, "text/css") {
		return false
	}
	media := strings.TrimSpace(strings.ToLower(attribute(node, "media")))
	if media == "" {
		return true
	}
	for _, medium := range strings.Split(media, ",") {
		medium = strings.TrimSpace(medium)
		if medium != "all" && medium != "screen" {
			return false
		}
	}
	return true
}

func sameOrigin(left, right *url.URL) bool {
	if left == nil || right == nil || !strings.EqualFold(left.Scheme, right.Scheme) ||
		!strings.EqualFold(left.Hostname(), right.Hostname()) || left.User != nil || right.User != nil {
		return false
	}
	return effectivePort(left) == effectivePort(right)
}

func effectivePort(parsedURL *url.URL) string {
	if port := parsedURL.Port(); port != "" {
		return port
	}
	if strings.EqualFold(parsedURL.Scheme, "https") {
		return "443"
	}
	return "80"
}

func attributePresent(node *html.Node, name string) bool {
	if node != nil {
		for _, attribute := range node.Attr {
			if attribute.Key == name {
				return true
			}
		}
	}
	return false
}

func parseStories(page []byte) ([]Story, error) {
	return parseStoriesWithStyles(page, nil)
}

func parseStoriesWithStyles(page []byte, linkedStyles map[string]string) ([]Story, error) {
	document, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return nil, fmt.Errorf("parse HTML: %w", err)
	}
	rules := parseCSSRulesWithLinkedStyles(document, linkedStyles)

	var stories []Story
	for node := document.FirstChild; node != nil; node = nextNode(node) {
		if node.Type != html.ElementNode || node.Data != "tr" || !hasClass(node, "athing") {
			continue
		}
		titleLine := findClass(node, "titleline")
		titleLink := findElement(titleLine, "a")
		title := normalizedText(titleLink)
		if title == "" {
			continue
		}

		story := Story{
			Rank:   strings.TrimSuffix(normalizedText(findClass(node, "rank")), "."),
			Title:  title,
			Domain: strings.TrimSpace(strings.Trim(strings.TrimSpace(normalizedText(findClass(titleLine, "sitebit"))), "()")),
			Style:  resolveTitleStyle(titleLink, rules),
		}
		if story.Rank == "" {
			story.Rank = strconv.Itoa(len(stories) + 1)
		}
		if story.Domain == "" && titleLink != nil {
			story.Domain = domainFromHref(attribute(titleLink, "href"))
		}

		if subtextRow := followingSiblingElement(node, "tr"); subtextRow != nil {
			subtext := findClass(subtextRow, "subtext")
			story.Score = normalizedText(findClass(subtext, "score"))
			story.User = normalizedText(findClass(subtext, "hnuser"))
			story.Age = normalizedText(findClass(subtext, "age"))
			story.Comments = findCommentsLink(subtext)
		}
		stories = append(stories, story)
	}
	if len(stories) == 0 {
		return nil, fmt.Errorf("no Hacker News story rows found")
	}
	return stories, nil
}

func followingSiblingElement(node *html.Node, tag string) *html.Node {
	for sibling := node.NextSibling; sibling != nil; sibling = sibling.NextSibling {
		if sibling.Type == html.ElementNode && sibling.Data == tag {
			return sibling
		}
	}
	return nil
}

func nextNode(node *html.Node) *html.Node {
	if node.FirstChild != nil {
		return node.FirstChild
	}
	for node != nil {
		if node.NextSibling != nil {
			return node.NextSibling
		}
		node = node.Parent
	}
	return nil
}

func nextNodeWithin(root, node *html.Node) *html.Node {
	if node.FirstChild != nil {
		return node.FirstChild
	}
	for node != nil && node != root {
		if node.NextSibling != nil {
			return node.NextSibling
		}
		node = node.Parent
	}
	return nil
}

func findClass(node *html.Node, class string) *html.Node {
	for current := node; current != nil; current = nextNodeWithin(node, current) {
		if hasClass(current, class) {
			return current
		}
	}
	return nil
}

func findElement(node *html.Node, tag string) *html.Node {
	for current := node; current != nil; current = nextNodeWithin(node, current) {
		if current.Type == html.ElementNode && current.Data == tag {
			return current
		}
	}
	return nil
}

func hasClass(node *html.Node, class string) bool {
	if node == nil {
		return false
	}
	for _, attribute := range node.Attr {
		if attribute.Key == "class" {
			for _, value := range strings.Fields(attribute.Val) {
				if value == class {
					return true
				}
			}
		}
	}
	return false
}

func attribute(node *html.Node, name string) string {
	if node != nil {
		for _, attribute := range node.Attr {
			if attribute.Key == name {
				return attribute.Val
			}
		}
	}
	return ""
}

func normalizedText(node *html.Node) string {
	if node == nil {
		return ""
	}
	var text strings.Builder
	for current := node; current != nil; current = nextNodeWithin(node, current) {
		if current.Type == html.TextNode {
			text.WriteByte(' ')
			text.WriteString(current.Data)
		}
	}
	return strings.Join(strings.Fields(text.String()), " ")
}

func findCommentsLink(node *html.Node) string {
	for current := node; current != nil; current = nextNodeWithin(node, current) {
		if current.Type != html.ElementNode || current.Data != "a" {
			continue
		}
		text := normalizedText(current)
		lower := strings.ToLower(text)
		if strings.Contains(lower, "comment") || lower == "discuss" {
			return text
		}
	}
	return ""
}

func domainFromHref(href string) string {
	parsedURL, err := url.Parse(href)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(parsedURL.Hostname(), "www.")
}
