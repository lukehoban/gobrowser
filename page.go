package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

const maxPageSize = 8 << 20

type Story struct {
	Rank     string
	Title    string
	Domain   string
	Score    string
	Age      string
	Comments string
}

func fetchPage(ctx context.Context, rawURL string) ([]byte, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("URL must use http or https")
	}
	if parsedURL.Host == "" {
		return nil, fmt.Errorf("URL must include a host")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("User-Agent", "gobrowser/0.1 (+https://github.com/lukehoban/gobrowser)")

	client := &http.Client{Timeout: 20_000_000_000}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch page: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch page: HTTP %s", response.Status)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxPageSize+1))
	if err != nil {
		return nil, fmt.Errorf("read page: %w", err)
	}
	if len(body) > maxPageSize {
		return nil, fmt.Errorf("page exceeds %d bytes", maxPageSize)
	}
	return body, nil
}

func parseStories(page []byte) ([]Story, error) {
	document, err := html.Parse(strings.NewReader(string(page)))
	if err != nil {
		return nil, fmt.Errorf("parse HTML: %w", err)
	}

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
			Domain: strings.Trim(strings.TrimSpace(normalizedText(findClass(titleLine, "sitebit"))), "()"),
		}
		if story.Rank == "" {
			story.Rank = strconv.Itoa(len(stories) + 1)
		}
		if story.Domain == "" && titleLink != nil {
			story.Domain = domainFromHref(attribute(titleLink, "href"))
		}

		subtext := findClass(node.NextSibling, "subtext")
		story.Score = normalizedText(findClass(subtext, "score"))
		story.Age = normalizedText(findClass(subtext, "age"))
		story.Comments = findCommentsLink(subtext)
		stories = append(stories, story)
	}
	if len(stories) == 0 {
		return nil, fmt.Errorf("no Hacker News story rows found")
	}
	return stories, nil
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

func findClass(node *html.Node, class string) *html.Node {
	for current := node; current != nil; current = nextNodeWithin(node, current) {
		if hasClass(current, class) {
			return current
		}
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
	if node == nil {
		return ""
	}
	for _, attribute := range node.Attr {
		if attribute.Key == name {
			return attribute.Val
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
