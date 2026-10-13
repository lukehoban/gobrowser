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
)

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
	titleFace, err := opentype.NewFace(parsedFont, &opentype.FaceOptions{Size: 15, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return fmt.Errorf("create title font: %w", err)
	}
	defer titleFace.Close()
	titleFaces := map[float64]font.Face{15: titleFace}
	metaFace, err := opentype.NewFace(parsedFont, &opentype.FaceOptions{Size: 11, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return fmt.Errorf("create metadata font: %w", err)
	}
	defer metaFace.Close()

	height := 64 + len(stories)*rowHeight + 18
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(pageColor), image.Point{}, draw.Src)

	contentWidth := min(width-32, 960)
	left := (width - contentWidth) / 2
	draw.Draw(canvas, image.Rect(left, 14, left+contentWidth, 14+headerHeight), image.NewUniform(orangeColor), image.Point{}, draw.Src)
	drawHeader(canvas, left, contentWidth, titleFace, metaFace)

	rowTop := 14 + headerHeight + 12
	for index, story := range stories {
		style := story.TitleStyle
		if style.FontSize == 0 {
			style = defaultTitleStyle()
		}
		face := titleFaces[style.FontSize]
		if face == nil {
			face, err = opentype.NewFace(parsedFont, &opentype.FaceOptions{
				Size:    style.FontSize,
				DPI:     72,
				Hinting: font.HintingFull,
			})
			if err != nil {
				return fmt.Errorf("create %gpx title font: %w", style.FontSize, err)
			}
			defer face.Close()
			titleFaces[style.FontSize] = face
		}
		drawStory(canvas, left, contentWidth, rowTop+index*rowHeight, story, face, metaFace, style.Color)
	}
	return png.Encode(output, canvas)
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

func drawStory(canvas *image.RGBA, left, width, top int, story Story, titleFace, metaFace font.Face, titleColor color.Color) {
	rank := story.Rank + "."
	rankWidth := textWidth(metaFace, rank)
	drawText(canvas, metaFace, left+23-rankWidth, top+16, rank, mutedColor)
	drawTriangle(canvas, left+31, top+7)

	titleX := left + 52
	domain := ""
	if story.Domain != "" {
		domain = "(" + story.Domain + ")"
	}
	available := left + width - titleX - 8
	if domain != "" {
		available -= textWidth(metaFace, domain) + 6
	}
	title := fitText(titleFace, story.Title, available)
	drawText(canvas, titleFace, titleX, top+17, title, titleColor)
	if domain != "" {
		domainX := titleX + textWidth(titleFace, title) + 6
		drawText(canvas, metaFace, domainX, top+16, fitText(metaFace, domain, left+width-domainX-8), mutedColor)
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
	drawText(canvas, metaFace, titleX, top+34, strings.Join(metadata, " | "), mutedColor)
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

func drawTriangle(canvas *image.RGBA, x, y int) {
	for row := 0; row < 7; row++ {
		draw.Draw(canvas, image.Rect(x, y+row, x+row+1, y+row+1), image.NewUniform(mutedColor), image.Point{}, draw.Src)
	}
}
