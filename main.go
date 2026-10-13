package main

import (
	"context"
	"flag"
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gobrowser:", err)
		os.Exit(1)
	}
}

func run() error {
	sourceURL := flag.String("url", "https://news.ycombinator.com/", "Hacker News page URL")
	outputPath := flag.String("out", "hackernews.png", "PNG output path")
	width := flag.Int("width", 1000, "output image width in pixels (minimum 640)")
	storyLimit := flag.Int("stories", 30, "maximum number of stories to render (1-30)")
	flag.Parse()

	if *storyLimit < 1 || *storyLimit > 30 {
		return fmt.Errorf("-stories must be between 1 and 30")
	}
	if *width < 640 {
		return fmt.Errorf("-width must be at least 640")
	}

	page, err := fetchPage(context.Background(), *sourceURL)
	if err != nil {
		return err
	}

	stories, err := parseStories(page)
	if err != nil {
		return err
	}
	if len(stories) > *storyLimit {
		stories = stories[:*storyLimit]
	}

	output, err := os.Create(*outputPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", *outputPath, err)
	}
	defer output.Close()

	if err := renderPNG(output, stories, *width); err != nil {
		return fmt.Errorf("render PNG: %w", err)
	}
	return nil
}
