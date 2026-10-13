package main

import (
	"image/color"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

const defaultTitleFontSize = 15

type cssDeclarations struct {
	color    color.RGBA
	hasColor bool
	fontSize int
	hasSize  bool
}

type cssRule struct {
	selectors    []cssSelector
	declarations cssDeclarations
	order        int
}

type cssSelector struct {
	parts    []string
	classes  int
	elements int
}

type cssPriority struct {
	inline   bool
	classes  int
	elements int
	order    int
}

type cssValue struct {
	color         color.RGBA
	hasColor      bool
	colorPriority cssPriority
	fontSize      int
	hasSize       bool
	sizePriority  cssPriority
}

func parseCSSRules(document *html.Node) []cssRule {
	var rules []cssRule
	order := 0
	for node := document.FirstChild; node != nil; node = nextNode(node) {
		if node.Type != html.ElementNode || node.Data != "style" {
			continue
		}
		source := stripCSSComments(textContent(node))
		for len(source) > 0 {
			open := strings.IndexByte(source, '{')
			if open < 0 {
				break
			}
			close := strings.IndexByte(source[open+1:], '}')
			if close < 0 {
				break
			}
			close += open + 1
			selectorText := strings.TrimSpace(source[:open])
			declarations := parseCSSDeclarations(source[open+1 : close])
			source = source[close+1:]

			selectors := parseCSSSelectors(selectorText)
			if len(selectors) == 0 || (!declarations.hasColor && !declarations.hasSize) {
				continue
			}
			rules = append(rules, cssRule{selectors: selectors, declarations: declarations, order: order})
			order++
		}
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

func stripCSSComments(source string) string {
	for {
		start := strings.Index(source, "/*")
		if start < 0 {
			return source
		}
		end := strings.Index(source[start+2:], "*/")
		if end < 0 {
			return source[:start]
		}
		end += start + 4
		source = source[:start] + source[end:]
	}
}

func parseCSSSelectors(source string) []cssSelector {
	var selectors []cssSelector
	for _, raw := range strings.Split(source, ",") {
		fields := strings.Fields(strings.TrimSpace(raw))
		if len(fields) == 0 {
			continue
		}
		selector := cssSelector{}
		valid := true
		for _, field := range fields {
			part, classCount, elementCount, ok := parseCSSCompound(field)
			if !ok {
				valid = false
				break
			}
			selector.parts = append(selector.parts, part)
			selector.classes += classCount
			selector.elements += elementCount
		}
		if valid {
			selectors = append(selectors, selector)
		}
	}
	return selectors
}

func parseCSSCompound(source string) (string, int, int, bool) {
	if source == "*" {
		return source, 0, 0, true
	}
	parts := strings.Split(source, ".")
	if len(parts) == 0 {
		return "", 0, 0, false
	}
	tag := parts[0]
	if tag != "" && !isCSSIdentifier(tag) {
		return "", 0, 0, false
	}
	for _, class := range parts[1:] {
		if class == "" || !isCSSIdentifier(class) {
			return "", 0, 0, false
		}
	}
	elements := 0
	if tag != "" {
		elements = 1
	}
	return source, len(parts) - 1, elements, true
}

func isCSSIdentifier(value string) bool {
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && char != '-' && char != '_' {
			return false
		}
	}
	return value != ""
}

func parseCSSDeclarations(source string) cssDeclarations {
	var declarations cssDeclarations
	for _, declaration := range strings.Split(source, ";") {
		property, value, found := strings.Cut(declaration, ":")
		if !found {
			continue
		}
		property = strings.ToLower(strings.TrimSpace(property))
		value = strings.TrimSpace(value)
		switch property {
		case "color":
			parsed, ok := parseCSSColor(value)
			if ok {
				declarations.color = parsed
				declarations.hasColor = true
			}
		case "font-size":
			parsed, ok := parseCSSFontSize(value)
			if ok {
				declarations.fontSize = parsed
				declarations.hasSize = true
			}
		}
	}
	return declarations
}

func parseCSSColor(value string) (color.RGBA, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if named, ok := map[string]color.RGBA{
		"black":  {A: 255},
		"blue":   {B: 255, A: 255},
		"gray":   {R: 128, G: 128, B: 128, A: 255},
		"green":  {G: 128, A: 255},
		"grey":   {R: 128, G: 128, B: 128, A: 255},
		"orange": {R: 255, G: 165, A: 255},
		"red":    {R: 255, A: 255},
		"white":  {R: 255, G: 255, B: 255, A: 255},
		"yellow": {R: 255, G: 255, A: 255},
	}[value]; ok {
		return named, true
	}
	if strings.HasPrefix(value, "#") {
		hex := value[1:]
		if len(hex) == 3 {
			r, rErr := strconv.ParseUint(strings.Repeat(string(hex[0]), 2), 16, 8)
			g, gErr := strconv.ParseUint(strings.Repeat(string(hex[1]), 2), 16, 8)
			b, bErr := strconv.ParseUint(strings.Repeat(string(hex[2]), 2), 16, 8)
			if rErr == nil && gErr == nil && bErr == nil {
				return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}, true
			}
		}
		if len(hex) == 6 {
			parsed, err := strconv.ParseUint(hex, 16, 24)
			if err == nil {
				return color.RGBA{R: uint8(parsed >> 16), G: uint8(parsed >> 8), B: uint8(parsed), A: 255}, true
			}
		}
	}
	if strings.HasPrefix(value, "rgb(") && strings.HasSuffix(value, ")") {
		channels := strings.Split(value[4:len(value)-1], ",")
		if len(channels) != 3 {
			return color.RGBA{}, false
		}
		values := [3]uint8{}
		for i, channel := range channels {
			n, err := strconv.Atoi(strings.TrimSpace(channel))
			if err != nil || n < 0 || n > 255 {
				return color.RGBA{}, false
			}
			values[i] = uint8(n)
		}
		return color.RGBA{R: values[0], G: values[1], B: values[2], A: 255}, true
	}
	return color.RGBA{}, false
}

func parseCSSFontSize(value string) (int, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	unit := ""
	for _, candidate := range []string{"px", "pt"} {
		if strings.HasSuffix(value, candidate) {
			unit = candidate
			value = strings.TrimSpace(strings.TrimSuffix(value, candidate))
			break
		}
	}
	if unit == "" {
		return 0, false
	}
	size, err := strconv.ParseFloat(value, 64)
	if err != nil || size <= 0 {
		return 0, false
	}
	if unit == "pt" {
		size *= 4.0 / 3.0
	}
	rounded := int(size + 0.5)
	if rounded < 6 || rounded > 32 {
		return 0, false
	}
	return rounded, true
}

func applyTitleStyles(story *Story, element *html.Node, rules []cssRule) {
	var style cssValue
	for _, rule := range rules {
		var selector *cssSelector
		for i := range rule.selectors {
			if selectorMatches(rule.selectors[i], element) &&
				(selector == nil || moreSpecific(rule.selectors[i], *selector)) {
				selector = &rule.selectors[i]
			}
		}
		if selector == nil {
			continue
		}
		priority := cssPriority{classes: selector.classes, elements: selector.elements, order: rule.order}
		applyCSSDeclarations(&style, rule.declarations, priority)
	}
	inline := parseCSSDeclarations(attribute(element, "style"))
	applyCSSDeclarations(&style, inline, cssPriority{inline: true})

	if style.hasColor {
		story.TitleColor = style.color
		story.HasTitleColor = true
	}
	if style.hasSize {
		story.TitleFontSize = style.fontSize
	}
}

func moreSpecific(a, b cssSelector) bool {
	if a.classes != b.classes {
		return a.classes > b.classes
	}
	return a.elements > b.elements
}

func applyCSSDeclarations(style *cssValue, declarations cssDeclarations, priority cssPriority) {
	if declarations.hasColor && (!style.hasColor || higherPriority(priority, style.colorPriority)) {
		style.color = declarations.color
		style.hasColor = true
		style.colorPriority = priority
	}
	if declarations.hasSize && (!style.hasSize || higherPriority(priority, style.sizePriority)) {
		style.fontSize = declarations.fontSize
		style.hasSize = true
		style.sizePriority = priority
	}
}

func higherPriority(a, b cssPriority) bool {
	if a.inline != b.inline {
		return a.inline
	}
	if a.classes != b.classes {
		return a.classes > b.classes
	}
	if a.elements != b.elements {
		return a.elements > b.elements
	}
	return a.order >= b.order
}

func selectorMatches(selector cssSelector, element *html.Node) bool {
	if element == nil || len(selector.parts) == 0 {
		return false
	}
	current := element
	for i := len(selector.parts) - 1; i >= 0; i-- {
		for current != nil && !compoundMatches(selector.parts[i], current) {
			current = current.Parent
		}
		if current == nil {
			return false
		}
		current = current.Parent
	}
	return true
}

func compoundMatches(selector string, element *html.Node) bool {
	if element.Type != html.ElementNode {
		return false
	}
	if selector == "*" {
		return true
	}
	parts := strings.Split(selector, ".")
	if parts[0] != "" && parts[0] != element.Data {
		return false
	}
	for _, class := range parts[1:] {
		if !hasClass(element, class) {
			return false
		}
	}
	return true
}
