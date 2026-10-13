package main

import (
	"image/color"
	"math"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/html"
)

type titleStyle struct {
	Color    color.RGBA
	FontSize float64
}

type cssDeclarations struct {
	color       color.RGBA
	hasColor    bool
	fontSize    float64
	hasFontSize bool
}

type cssRule struct {
	selector     string
	declarations cssDeclarations
	specificity  int
	order        int
}

type cssRank struct {
	specificity int
	order       int
}

func defaultTitleStyle() titleStyle {
	return titleStyle{
		Color:    color.RGBA{A: 255},
		FontSize: 15,
	}
}

func parsePageStyles(document *html.Node) []cssRule {
	var rules []cssRule
	order := 0
	for node := document; node != nil; node = nextNodeWithin(document, node) {
		if node.Type != html.ElementNode || node.Data != "style" {
			continue
		}
		rules = append(rules, parseCSSRules(textContent(node), &order)...)
	}
	return rules
}

func textContent(node *html.Node) string {
	var text strings.Builder
	for current := node; current != nil; current = nextNodeWithin(node, current) {
		if current.Type == html.TextNode {
			text.WriteString(current.Data)
		}
	}
	return text.String()
}

func parseCSSRules(source string, order *int) []cssRule {
	source = removeCSSComments(source)
	var rules []cssRule
	for {
		open := strings.IndexByte(source, '{')
		if open < 0 {
			break
		}
		close := strings.IndexByte(source[open+1:], '}')
		if close < 0 {
			break
		}
		close += open + 1
		declarations := parseCSSDeclarations(source[open+1 : close])
		for _, selector := range strings.Split(source[:open], ",") {
			selector = strings.TrimSpace(selector)
			_, specificity, ok := parseSelector(selector)
			if !ok {
				continue
			}
			*order = *order + 1
			rules = append(rules, cssRule{
				selector:     selector,
				declarations: declarations,
				specificity:  specificity,
				order:        *order,
			})
		}
		source = source[close+1:]
	}
	return rules
}

func removeCSSComments(source string) string {
	for {
		start := strings.Index(source, "/*")
		if start < 0 {
			return source
		}
		end := strings.Index(source[start+2:], "*/")
		if end < 0 {
			return source[:start]
		}
		source = source[:start] + source[start+2+end+2:]
	}
}

func parseCSSDeclarations(source string) cssDeclarations {
	var declarations cssDeclarations
	for _, declaration := range strings.Split(source, ";") {
		propertyValue := strings.SplitN(declaration, ":", 2)
		if len(propertyValue) != 2 {
			continue
		}
		property := strings.ToLower(strings.TrimSpace(propertyValue[0]))
		value := strings.TrimSpace(propertyValue[1])
		switch property {
		case "color":
			if parsedColor, ok := parseCSSColor(value); ok {
				declarations.color = parsedColor
				declarations.hasColor = true
			}
		case "font-size":
			if size, ok := parseCSSFontSize(value); ok {
				declarations.fontSize = size
				declarations.hasFontSize = true
			}
		}
	}
	return declarations
}

func parseCSSColor(value string) (color.RGBA, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	namedColors := map[string]color.RGBA{
		"black":  {A: 255},
		"blue":   {B: 255, A: 255},
		"gray":   {R: 128, G: 128, B: 128, A: 255},
		"green":  {G: 128, A: 255},
		"grey":   {R: 128, G: 128, B: 128, A: 255},
		"orange": {R: 255, G: 165, A: 255},
		"purple": {R: 128, B: 128, A: 255},
		"red":    {R: 255, A: 255},
		"white":  {R: 255, G: 255, B: 255, A: 255},
		"yellow": {R: 255, G: 255, A: 255},
	}
	if parsed, ok := namedColors[value]; ok {
		return parsed, true
	}
	if strings.HasPrefix(value, "#") {
		hex := value[1:]
		if len(hex) == 3 {
			hex = strings.Join([]string{hex[:1], hex[:1], hex[1:2], hex[1:2], hex[2:3], hex[2:3]}, "")
		}
		if len(hex) != 6 {
			return color.RGBA{}, false
		}
		parsed, err := strconv.ParseUint(hex, 16, 24)
		if err != nil {
			return color.RGBA{}, false
		}
		return color.RGBA{
			R: uint8(parsed >> 16),
			G: uint8(parsed >> 8),
			B: uint8(parsed),
			A: 255,
		}, true
	}
	if strings.HasPrefix(value, "rgb(") && strings.HasSuffix(value, ")") {
		channels := strings.Split(value[4:len(value)-1], ",")
		if len(channels) != 3 {
			return color.RGBA{}, false
		}
		rgb := [3]uint8{}
		for index, channel := range channels {
			value, err := strconv.Atoi(strings.TrimSpace(channel))
			if err != nil || value < 0 || value > 255 {
				return color.RGBA{}, false
			}
			rgb[index] = uint8(value)
		}
		return color.RGBA{R: rgb[0], G: rgb[1], B: rgb[2], A: 255}, true
	}
	return color.RGBA{}, false
}

func parseCSSFontSize(value string) (float64, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	multiplier := 1.0
	switch {
	case strings.HasSuffix(value, "px"):
		value = strings.TrimSpace(strings.TrimSuffix(value, "px"))
	case strings.HasSuffix(value, "pt"):
		value = strings.TrimSpace(strings.TrimSuffix(value, "pt"))
		multiplier = 96.0 / 72.0
	default:
		return 0, false
	}
	size, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, false
	}
	size *= multiplier
	if math.IsNaN(size) || math.IsInf(size, 0) || size < 6 || size > 32 {
		return 0, false
	}
	return size, true
}

func parseSelector(selector string) ([]string, int, bool) {
	parts := strings.Fields(selector)
	if len(parts) == 0 {
		return nil, 0, false
	}
	specificity := 0
	for _, part := range parts {
		if part == ">" || strings.ContainsAny(part, "+~:[]") {
			return nil, 0, false
		}
		_, partSpecificity, ok := parseCompoundSelector(part)
		if !ok {
			return nil, 0, false
		}
		specificity += partSpecificity
	}
	return parts, specificity, true
}

func parseCompoundSelector(selector string) (simpleSelector, int, bool) {
	var parsed simpleSelector
	specificity := 0
	for index := 0; index < len(selector); {
		switch selector[index] {
		case '.':
			value, next := selectorIdentifier(selector, index+1)
			if next == index+1 {
				return simpleSelector{}, 0, false
			}
			parsed.classes = append(parsed.classes, value)
			specificity += 10
			index = next
		case '#':
			value, next := selectorIdentifier(selector, index+1)
			if next == index+1 {
				return simpleSelector{}, 0, false
			}
			parsed.id = value
			specificity += 100
			index = next
		default:
			if parsed.tag != "" {
				return simpleSelector{}, 0, false
			}
			value, next := selectorIdentifier(selector, index)
			if next == index {
				return simpleSelector{}, 0, false
			}
			if value != "*" {
				parsed.tag = strings.ToLower(value)
				specificity++
			}
			index = next
		}
	}
	return parsed, specificity, specificity > 0 || selector == "*"
}

type simpleSelector struct {
	tag     string
	id      string
	classes []string
}

func selectorIdentifier(selector string, start int) (string, int) {
	end := start
	for end < len(selector) {
		character := rune(selector[end])
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) && character != '-' && character != '_' && character != '*' {
			break
		}
		end++
	}
	return selector[start:end], end
}

func matchesSelector(node *html.Node, selector string) bool {
	parts, _, ok := parseSelector(selector)
	if !ok || !matchesCompoundSelector(node, parts[len(parts)-1]) {
		return false
	}
	ancestor := node.Parent
	for index := len(parts) - 2; index >= 0; index-- {
		for ancestor != nil && !matchesCompoundSelector(ancestor, parts[index]) {
			ancestor = ancestor.Parent
		}
		if ancestor == nil {
			return false
		}
		ancestor = ancestor.Parent
	}
	return true
}

func matchesCompoundSelector(node *html.Node, selector string) bool {
	if node == nil || node.Type != html.ElementNode {
		return false
	}
	parsed, _, ok := parseCompoundSelector(selector)
	if !ok {
		return false
	}
	if parsed.tag != "" && node.Data != parsed.tag {
		return false
	}
	if parsed.id != "" && attribute(node, "id") != parsed.id {
		return false
	}
	for _, class := range parsed.classes {
		if !hasClass(node, class) {
			return false
		}
	}
	return true
}

func computedTitleStyle(node *html.Node, rules []cssRule) titleStyle {
	style := defaultTitleStyle()
	colorRank := cssRank{specificity: -1, order: -1}
	sizeRank := cssRank{specificity: -1, order: -1}
	for _, rule := range rules {
		if !matchesSelector(node, rule.selector) {
			continue
		}
		rank := cssRank{specificity: rule.specificity, order: rule.order}
		if rule.declarations.hasColor && rankAtLeast(rank, colorRank) {
			style.Color = rule.declarations.color
			colorRank = rank
		}
		if rule.declarations.hasFontSize && rankAtLeast(rank, sizeRank) {
			style.FontSize = rule.declarations.fontSize
			sizeRank = rank
		}
	}
	if inline := parseCSSDeclarations(attribute(node, "style")); inline.hasColor || inline.hasFontSize {
		if inline.hasColor {
			style.Color = inline.color
		}
		if inline.hasFontSize {
			style.FontSize = inline.fontSize
		}
	}
	return style
}

func rankAtLeast(candidate, current cssRank) bool {
	if candidate.specificity != current.specificity {
		return candidate.specificity > current.specificity
	}
	return candidate.order >= current.order
}
