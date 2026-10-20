package main

import (
	"context"
	"flag"
	"fmt"
	"io"
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
	inputPath := flag.String("input", "", "read saved HTML instead of fetching the URL")
	outputPath := flag.String("out", "hackernews.png", "PNG output path")
	width := flag.Int("width", defaultViewportWidth, "output image width in pixels (640-4096)")
	storyLimit := flag.Int("stories", 30, "maximum number of stories to render (1-30)")
	flag.Parse()

	if *storyLimit < 1 || *storyLimit > 30 {
		return fmt.Errorf("-stories must be between 1 and 30")
	}
	if *width < 640 || *width > 4096 {
		return fmt.Errorf("-width must be between 640 and 4096")
	}

	var (
		page         []byte
		linkedStyles map[string]string
		err          error
	)
	if *inputPath == "" {
		page, linkedStyles, err = fetchPageWithStyles(context.Background(), *sourceURL)
	} else {
		page, err = readPage(context.Background(), *sourceURL, *inputPath)
	}
	if err != nil {
		return err
	}
	stories, err := parseStoriesForViewport(page, linkedStyles, *width)
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
	if err := renderPNG(output, stories, *width); err != nil {
		output.Close()
		return fmt.Errorf("render PNG: %w", err)
	}
	if err := output.Close(); err != nil {
		return fmt.Errorf("close %s: %w", *outputPath, err)
	}
	return nil
}

func readPage(ctx context.Context, sourceURL, inputPath string) ([]byte, error) {
	if inputPath == "" {
		return fetchPage(ctx, sourceURL)
	}

	input, err := os.Open(inputPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", inputPath, err)
	}
	defer input.Close()

	page, err := io.ReadAll(io.LimitReader(input, maxPageSize+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", inputPath, err)
	}
	if len(page) > maxPageSize {
		return nil, fmt.Errorf("input exceeds %d bytes", maxPageSize)
	}
	return page, nil
}
