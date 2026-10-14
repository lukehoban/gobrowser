package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	headerHeight = 34
	rowHeight    = 48
	rowPadding   = 3
	rowGap       = 2
	rowBottom    = 7
)

type storyLayout struct {
	height           int
	titleLines       []string
	titleBaseline    int
	titleLineHeight  int
	domainInline     bool
	domainBaseline   int
	metadataBaseline int
}

var (
	pageColor   = color.RGBA{R: 246, G: 246, B: 239, A: 255}
	orangeColor = color.RGBA{R: 255, G: 102, B: 0, A: 255}
	textColor   = color.RGBA{R: 0, G: 0, B: 0, A: 255}
	mutedColor  = color.RGBA{R: 130, G: 130, B: 130, A: 255}
)

func renderPNG(output io.Writer, stories []Story, width int) error {
	if width < 640 || width > 4096 {
		return fmt.Errorf("width must be between 640 and 4096 pixels")
	}
	if len(stories) == 0 {
		return fmt.Errorf("at least one story is required")
	}

	parsedFont, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return fmt.Errorf("parse embedded font: %w", err)
	}
	titleFaces := make(map[float64]font.Face)
	defer func() {
		for _, face := range titleFaces {
			face.Close()
		}
	}()
	titleFaceForSize := func(size float64) (font.Face, error) {
		if size == 0 {
			size = defaultTitleFontSize
		}
		if face, ok := titleFaces[size]; ok {
			return face, nil
		}
		face, err := opentype.NewFace(parsedFont, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
		if err != nil {
			return nil, fmt.Errorf("create title font: %w", err)
		}
		titleFaces[size] = face
		return face, nil
	}
	titleFace, err := titleFaceForSize(defaultTitleFontSize)
	if err != nil {
		return err
	}
	metaFace, err := opentype.NewFace(parsedFont, &opentype.FaceOptions{Size: 11, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return fmt.Errorf("create metadata font: %w", err)
	}
	defer metaFace.Close()
	defer metaFace.Close()

	contentWidth := min(width-32, 960)
	left := (width - contentWidth) / 2
	layouts := make([]storyLayout, len(stories))
	height := 64 + 18
	for index, story := range stories {
		titleFace, err := titleFaceForSize(story.Style.fontSize)
		if err != nil {
			return err
		}
		titleWidth := contentWidth - 52 - 8
		layouts[index] = calculateStoryLayout(titleFace, metaFace, story, titleWidth)
		height += layouts[index].height
	}
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(pageColor), image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(left, 14, left+contentWidth, 14+headerHeight), image.NewUniform(orangeColor), image.Point{}, draw.Src)
	drawHeader(canvas, left, contentWidth, titleFace, metaFace)

	rowTop := 14 + headerHeight + 12
	for index, story := range stories {
		titleFace, err := titleFaceForSize(story.Style.fontSize)
		if err != nil {
			return err
		}
		drawStory(canvas, left, contentWidth, rowTop, story, titleFace, metaFace, layouts[index])
		rowTop += layouts[index].height
	}
	return png.Encode(output, canvas)
}

func calculateStoryLayout(titleFace, metaFace font.Face, story Story, titleWidth int) storyLayout {
	titleMetrics := titleFace.Metrics()
	metadataMetrics := metaFace.Metrics()
	titleBaseline := rowPadding + titleMetrics.Ascent.Ceil()
	lineHeight := titleMetrics.Height.Ceil()
	if lineHeight < titleMetrics.Ascent.Ceil()+titleMetrics.Descent.Ceil() {
		lineHeight = titleMetrics.Ascent.Ceil() + titleMetrics.Descent.Ceil()
	}

	domain := ""
	if story.Domain != "" {
		domain = "(" + story.Domain + ")"
	}
	domainInline := domain != ""
	domainWidth := textWidth(metaFace, domain) + 6
	if domainInline && titleWidth-domainWidth < minRuneWidth(titleFace, story.Title) {
		domainInline = false
	}
	if domainInline {
		titleWidth -= domainWidth
	}
	titleLines := wrapText(titleFace, story.Title, titleWidth)
	lastTitleBaseline := titleBaseline + (len(titleLines)-1)*lineHeight

	metadataBaseline := lastTitleBaseline + titleMetrics.Descent.Ceil() + rowGap + metadataMetrics.Ascent.Ceil()
	domainBaseline := lastTitleBaseline
	if domain != "" && !domainInline {
		domainBaseline = lastTitleBaseline + titleMetrics.Descent.Ceil() + rowGap + metadataMetrics.Ascent.Ceil()
		metadataBaseline = domainBaseline + metadataMetrics.Descent.Ceil() + rowGap + metadataMetrics.Ascent.Ceil()
	}
	height := metadataBaseline + metadataMetrics.Descent.Ceil() + rowBottom
	if height < rowHeight {
		height = rowHeight
	}
	return storyLayout{
		height:           height,
		titleLines:       titleLines,
		titleBaseline:    titleBaseline,
		titleLineHeight:  lineHeight,
		domainInline:     domainInline,
		domainBaseline:   domainBaseline,
		metadataBaseline: metadataBaseline,
	}
}

func drawHeader(canvas *image.RGBA, left, width int, titleFace, metaFace font.Face) {
	draw.Draw(canvas, image.Rect(left+8, 21, left+30, 43), image.NewUniform(color.White), image.Point{}, draw.Src)
	drawText(canvas, titleFace, left+13, 38, "Y", textColor)
	drawText(canvas, titleFace, left+36, 37, "Hacker News", textColor)

	menuX := left + 142
	for _, item := range []string{"new", "past", "comments", "ask", "show", "jobs", "submit"} {
		drawText(canvas, metaFace, menuX, 35, item, textColor)
		menuX += textWidth(metaFace, item) + 12
	}
	login := "login"
	drawText(canvas, metaFace, left+width-textWidth(metaFace, login)-10, 35, login, textColor)
}

func drawStory(canvas *image.RGBA, left, width, top int, story Story, titleFace, metaFace font.Face, layout storyLayout) {
	rank := story.Rank + "."
	rankWidth := textWidth(metaFace, rank)
	drawText(canvas, metaFace, left+23-rankWidth, top+16, rank, mutedColor)
	drawTriangle(canvas, left+31, top+7)

	titleX := left + 52
	domain := ""
	if story.Domain != "" {
		domain = "(" + story.Domain + ")"
	}
	titleColor := textColor
	if story.Style.hasColor {
		titleColor = story.Style.color
	}
	for index, line := range layout.titleLines {
		drawText(canvas, titleFace, titleX, top+layout.titleBaseline+index*layout.titleLineHeight, line, titleColor)
	}
	if domain != "" {
		domainX := titleX
		domainBaseline := layout.domainBaseline
		if layout.domainInline {
			lastTitle := layout.titleLines[len(layout.titleLines)-1]
			domainX += textWidth(titleFace, lastTitle) + 6
		}
		drawText(canvas, metaFace, domainX, top+domainBaseline, fitText(metaFace, domain, left+width-domainX-8), mutedColor)
	}

	metadata := make([]string, 0, 4)
	if story.Score != "" {
		metadata = append(metadata, story.Score)
	}
	if story.User != "" {
		metadata = append(metadata, "by "+story.User)
	}
	if story.Age != "" {
		metadata = append(metadata, story.Age)
	}
	if story.Comments != "" {
		metadata = append(metadata, story.Comments)
	}
	drawText(canvas, metaFace, titleX, top+layout.metadataBaseline, strings.Join(metadata, " | "), mutedColor)
}

func drawText(canvas *image.RGBA, face font.Face, x, baseline int, text string, foreground color.Color) {
	drawer := font.Drawer{
		Dst:  canvas,
		Src:  image.NewUniform(foreground),
		Face: face,
		Dot:  fixed.P(x, baseline),
	}
	drawer.DrawString(text)
}

func textWidth(face font.Face, text string) int {
	return font.MeasureString(face, text).Ceil()
}

func fitText(face font.Face, text string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if textWidth(face, text) <= maxWidth {
		return text
	}
	const ellipsis = "…"
	if textWidth(face, ellipsis) > maxWidth {
		return ""
	}
	runes := []rune(text)
	for len(runes) > 0 && textWidth(face, string(runes)+ellipsis) > maxWidth {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + ellipsis
}

func wrapText(face font.Face, text string, maxWidth int) []string {
	if maxWidth <= 0 {
		return []string{""}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}

	lines := make([]string, 0, len(words))
	line := ""
	for _, word := range words {
		if textWidth(face, word) > maxWidth {
			if line != "" {
				lines = append(lines, line)
				line = ""
			}
			chunks := splitTextToFit(face, word, maxWidth)
			lines = append(lines, chunks[:len(chunks)-1]...)
			line = chunks[len(chunks)-1]
			continue
		}

		candidate := word
		if line != "" {
			candidate = line + " " + word
		}
		if line != "" && textWidth(face, candidate) > maxWidth {
			lines = append(lines, line)
			line = word
		} else {
			line = candidate
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

func splitTextToFit(face font.Face, text string, maxWidth int) []string {
	chunks := make([]string, 0, 2)
	chunk := ""
	for _, r := range text {
		candidate := chunk + string(r)
		if chunk != "" && textWidth(face, candidate) > maxWidth {
			chunks = append(chunks, chunk)
			chunk = string(r)
		} else {
			chunk = candidate
		}
	}
	if chunk != "" {
		chunks = append(chunks, chunk)
	}
	return chunks
}

func minRuneWidth(face font.Face, text string) int {
	for _, r := range text {
		return textWidth(face, string(r))
	}
	return 0
}

func drawTriangle(canvas *image.RGBA, x, y int) {
	for row := 0; row < 7; row++ {
		draw.Draw(canvas, image.Rect(x, y+row, x+row+1, y+row+1), image.NewUniform(mutedColor), image.Point{}, draw.Src)
	}
}
