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
	inputPath := flag.String("input", "", "read a saved Hacker News HTML page instead of fetching the URL")
	outputPath := flag.String("out", "hackernews.png", "PNG output path")
	width := flag.Int("width", 1000, "output image width in pixels (640-4096)")
	storyLimit := flag.Int("stories", 30, "maximum number of stories to render (1-30)")
	flag.Parse()

	if *storyLimit < 1 || *storyLimit > 30 {
		return fmt.Errorf("-stories must be between 1 and 30")
	}
	if *width < 640 || *width > 4096 {
		return fmt.Errorf("-width must be between 640 and 4096")
	}

	var (
		page []byte
		err  error
	)
	if *inputPath == "" {
		page, err = fetchPage(context.Background(), *sourceURL)
		if err != nil {
			return err
		}
	} else {
		input, err := os.Open(*inputPath)
		if err != nil {
			return fmt.Errorf("open %s: %w", *inputPath, err)
		}
		page, err = io.ReadAll(io.LimitReader(input, maxPageSize+1))
		closeErr := input.Close()
		if err != nil {
			return fmt.Errorf("read %s: %w", *inputPath, err)
		}
		if closeErr != nil {
			return fmt.Errorf("close %s: %w", *inputPath, closeErr)
		}
		if len(page) > maxPageSize {
			return fmt.Errorf("input exceeds %d bytes", maxPageSize)
		}
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
