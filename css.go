package main

import (
	"image/color"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

const (
	defaultTitleFontSize = 15
	maxTitleFontSize     = 64
	maxCSSRules          = 1024
	maxSelectorParts     = 8
	maxCSSBlockDepth     = 4
	defaultViewportWidth = 1000
)

type textStyle struct {
	color    color.RGBA
	hasColor bool
	fontSize float64
}

type cssRule struct {
	selector     cssSelector
	declarations map[string]cssDeclaration
	order        int
}

type cssDeclaration struct {
	value     string
	important bool
}

type cssSelector struct {
	parts       []cssCompound
	combinators []rune
	specificity specificity
}

type cssCompound struct {
	tag     string
	id      string
	classes []string
}

type specificity struct {
	ids     int
	classes int
	tags    int
}

type winningDeclaration struct {
	declaration cssDeclaration
	specificity specificity
	order       int
	inline      bool
	set         bool
}

func parseCSSRules(document *html.Node) []cssRule {
	return parseCSSRulesWithLinkedStyles(document, nil)
}

func parseCSSRulesWithLinkedStyles(document *html.Node, linkedStyles map[string]string) []cssRule {
	return parseCSSRulesForViewport(document, linkedStyles, defaultViewportWidth)
}

// parseCSSRulesForViewport collects style rules from embedded and linked
// stylesheets, applying @media blocks only when they match viewportWidth.
func parseCSSRulesForViewport(document *html.Node, linkedStyles map[string]string, viewportWidth int) []cssRule {
	var rules []cssRule
	order := 0
	for node := document; node != nil; node = nextNode(node) {
		if node.Type != html.ElementNode {
			continue
		}
		switch node.Data {
		case "style":
			rules = appendCSSRules(rules, textContent(node), viewportWidth, &order)
		case "link":
			if isStylesheetLink(node) {
				rules = appendCSSRules(rules, linkedStyles[strings.TrimSpace(attribute(node, "href"))], viewportWidth, &order)
			}
		}
		if len(rules) >= maxCSSRules {
			return rules[:maxCSSRules]
		}
	}
	return rules
}

func appendCSSRules(rules []cssRule, stylesheet string, viewportWidth int, order *int) []cssRule {
	return appendCSSBlock(rules, stripCSSComments(stylesheet), viewportWidth, 0, order)
}

// appendCSSBlock parses a sequence of style rules and at-rules. Each block is
// consumed as a unit, so unsupported at-rules are skipped whole and @media
// contents are only parsed when the query matches the viewport.
func appendCSSBlock(rules []cssRule, source string, viewportWidth, depth int, order *int) []cssRule {
	for len(rules) < maxCSSRules {
		boundary := strings.IndexAny(source, "{;}")
		if boundary < 0 {
			break
		}
		prelude := strings.TrimSpace(source[:boundary])
		if source[boundary] != '{' {
			// Statement at-rules such as @import end at ';'; stray '}' are ignored.
			source = source[boundary+1:]
			continue
		}
		var body string
		body, source = cssBlockBody(source[boundary+1:])
		if strings.HasPrefix(prelude, "@") {
			if query, ok := mediaAtRuleQuery(prelude); ok && depth < maxCSSBlockDepth && mediaQueryListMatches(query, viewportWidth) {
				rules = appendCSSBlock(rules, body, viewportWidth, depth+1, order)
			}
			continue
		}
		rules = appendStyleRule(rules, prelude, body, order)
	}
	return rules
}

// cssBlockBody returns the contents of a block whose opening brace has already
// been consumed, and the source following its matching closing brace. An
// unterminated block extends to the end of the source.
func cssBlockBody(source string) (string, string) {
	depth := 0
	for index := 0; index < len(source); index++ {
		switch source[index] {
		case '{':
			depth++
		case '}':
			if depth == 0 {
				return source[:index], source[index+1:]
			}
			depth--
		}
	}
	return source, ""
}

func appendStyleRule(rules []cssRule, selectorList, body string, order *int) []cssRule {
	declarations := parseCSSDeclarations(body)
	if len(declarations) == 0 {
		return rules
	}
	for _, selectorText := range strings.Split(selectorList, ",") {
		selector, ok := parseCSSSelector(strings.TrimSpace(selectorText))
		if !ok {
			continue
		}
		*order++
		rules = append(rules, cssRule{selector: selector, declarations: declarations, order: *order})
		if len(rules) >= maxCSSRules {
			return rules
		}
	}
	return rules
}

func mediaAtRuleQuery(prelude string) (string, bool) {
	const keyword = "@media"
	if len(prelude) < len(keyword) || !strings.EqualFold(prelude[:len(keyword)], keyword) {
		return "", false
	}
	query := prelude[len(keyword):]
	if query != "" && query[0] != '(' && !isCSSWhitespace(query[0]) {
		return "", false
	}
	return query, true
}

// mediaQueryListMatches reports whether any comma-separated media query
// matches a screen of the given width. An empty list matches all media.
func mediaQueryListMatches(list string, viewportWidth int) bool {
	if strings.TrimSpace(list) == "" {
		return true
	}
	for _, query := range strings.Split(list, ",") {
		if mediaQueryMatches(query, viewportWidth) {
			return true
		}
	}
	return false
}

// mediaQueryMatches evaluates "[not|only] type [and (feature)]*" and
// "[not] (feature) [and (feature)]*" queries. Queries with unsupported features
// or syntax never match, even when negated.
func mediaQueryMatches(query string, viewportWidth int) bool {
	tokens, ok := mediaQueryTokens(strings.ToLower(query))
	if !ok || len(tokens) == 0 {
		return false
	}
	negated := false
	switch tokens[0] {
	case "not":
		negated = true
		tokens = tokens[1:]
	case "only":
		tokens = tokens[1:]
		if len(tokens) == 0 || tokens[0][0] == '(' {
			return false
		}
	}
	if len(tokens)%2 == 0 {
		return false
	}
	matched := true
	for index, token := range tokens {
		switch {
		case index%2 == 1:
			if token != "and" {
				return false
			}
		case token[0] == '(':
			featureMatched, known := mediaFeatureMatches(token[1:len(token)-1], viewportWidth)
			if !known {
				return false
			}
			matched = matched && featureMatched
		case index == 0 && token != "and" && token != "not" && token != "only":
			matched = matched && (token == "all" || token == "screen")
		default:
			return false
		}
	}
	return matched != negated
}

func mediaQueryTokens(query string) ([]string, bool) {
	var tokens []string
	for {
		query = strings.TrimLeft(query, " \t\r\n\f")
		if query == "" {
			return tokens, true
		}
		switch query[0] {
		case '(':
			end := strings.IndexByte(query, ')')
			if end < 0 {
				return nil, false
			}
			tokens = append(tokens, query[:end+1])
			query = query[end+1:]
		case ')':
			return nil, false
		default:
			end := strings.IndexAny(query, " \t\r\n\f()")
			if end < 0 {
				end = len(query)
			}
			tokens = append(tokens, query[:end])
			query = query[end:]
		}
	}
}

// mediaFeatureMatches evaluates width, min-width, and max-width features. The
// second result is false for unsupported features or values.
func mediaFeatureMatches(feature string, viewportWidth int) (bool, bool) {
	name, value, hasValue := strings.Cut(feature, ":")
	name = strings.TrimSpace(name)
	if !hasValue {
		return viewportWidth > 0, name == "width"
	}
	length, ok := parseMediaLength(value)
	if !ok {
		return false, false
	}
	width := float64(viewportWidth)
	switch name {
	case "width":
		return width == length, true
	case "min-width":
		return width >= length, true
	case "max-width":
		return width <= length, true
	}
	return false, false
}

func parseMediaLength(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	multiplier := float64(1)
	switch {
	case strings.HasSuffix(value, "px"):
		value = value[:len(value)-2]
	case strings.HasSuffix(value, "rem"):
		value = value[:len(value)-3]
		multiplier = 16
	case strings.HasSuffix(value, "em"):
		value = value[:len(value)-2]
		multiplier = 16
	case value != "0":
		return 0, false
	}
	length, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || length < 0 {
		return 0, false
	}
	return length * multiplier, true
}

func isCSSWhitespace(character byte) bool {
	return character == ' ' || character == '\t' || character == '\n' || character == '\r' || character == '\f'
}

func stripCSSComments(source string) string {
	var cleaned strings.Builder
	for len(source) > 0 {
		start := strings.Index(source, "/*")
		if start < 0 {
			cleaned.WriteString(source)
			break
		}
		cleaned.WriteString(source[:start])
		source = source[start+2:]
		end := strings.Index(source, "*/")
		if end < 0 {
			break
		}
		source = source[end+2:]
	}
	return cleaned.String()
}

func textContent(node *html.Node) string {
	var content strings.Builder
	for current := node; current != nil; current = nextNodeWithin(node, current) {
		if current.Type == html.TextNode {
			content.WriteString(current.Data)
		}
	}
	return content.String()
}

func parseCSSDeclarations(source string) map[string]cssDeclaration {
	declarations := make(map[string]cssDeclaration)
	for _, part := range strings.Split(source, ";") {
		colon := strings.IndexByte(part, ':')
		if colon < 0 {
			continue
		}
		property := strings.ToLower(strings.TrimSpace(part[:colon]))
		value := strings.TrimSpace(part[colon+1:])
		if property != "color" && property != "font-size" {
			continue
		}
		important := false
		if strings.HasSuffix(strings.ToLower(value), "!important") {
			important = true
			value = strings.TrimSpace(value[:len(value)-len("!important")])
		}
		if property == "color" {
			if _, ok := parseCSSColor(value); !ok {
				continue
			}
		} else if !validFontSize(value) {
			continue
		}
		previous, exists := declarations[property]
		if !exists || important || !previous.important {
			declarations[property] = cssDeclaration{value: value, important: important}
		}
	}
	return declarations
}

func parseCSSSelector(source string) (cssSelector, bool) {
	if source == "" {
		return cssSelector{}, false
	}
	fields := strings.Fields(strings.ReplaceAll(source, ">", " > "))
	var selector cssSelector
	var pendingCombinator rune
	for _, field := range fields {
		if field == ">" {
			if len(selector.parts) == 0 || pendingCombinator != 0 {
				return cssSelector{}, false
			}
			pendingCombinator = '>'
			continue
		}
		compound, specificity, ok := parseCSSCompound(field)
		if !ok || len(selector.parts) >= maxSelectorParts {
			return cssSelector{}, false
		}
		if len(selector.parts) > 0 {
			combinator := pendingCombinator
			if combinator == 0 {
				combinator = ' '
			}
			selector.combinators = append(selector.combinators, combinator)
		}
		pendingCombinator = 0
		selector.parts = append(selector.parts, compound)
		selector.specificity.ids += specificity.ids
		selector.specificity.classes += specificity.classes
		selector.specificity.tags += specificity.tags
	}
	return selector, len(selector.parts) > 0 && pendingCombinator == 0
}

func parseCSSCompound(source string) (cssCompound, specificity, bool) {
	var compound cssCompound
	var score specificity
	for index := 0; index < len(source); {
		switch source[index] {
		case '*':
			if index != 0 || compound.tag != "" {
				return cssCompound{}, specificity{}, false
			}
			index++
			if index < len(source) && source[index] != '.' && source[index] != '#' {
				return cssCompound{}, specificity{}, false
			}
		case '.', '#':
			kind := source[index]
			index++
			start := index
			for index < len(source) && isCSSIdentifier(source[index]) {
				index++
			}
			if start == index {
				return cssCompound{}, specificity{}, false
			}
			name := source[start:index]
			if kind == '.' {
				compound.classes = append(compound.classes, name)
				score.classes++
			} else {
				if compound.id != "" {
					return cssCompound{}, specificity{}, false
				}
				compound.id = name
				score.ids++
			}
		default:
			start := index
			for index < len(source) && isCSSIdentifier(source[index]) {
				index++
			}
			if start == index || compound.tag != "" {
				return cssCompound{}, specificity{}, false
			}
			compound.tag = strings.ToLower(source[start:index])
			score.tags++
		}
	}
	return compound, score, true
}

func isCSSIdentifier(character byte) bool {
	return character == '-' || character == '_' || character >= 'a' && character <= 'z' ||
		character >= 'A' && character <= 'Z' || character >= '0' && character <= '9'
}

func (selector cssSelector) matches(node *html.Node) bool {
	index := len(selector.parts) - 1
	if index < 0 || !selector.parts[index].matches(node) {
		return false
	}
	for index > 0 {
		parent := node.Parent
		if selector.combinators[index-1] == '>' {
			if parent == nil || !selector.parts[index-1].matches(parent) {
				return false
			}
			node = parent
			index--
			continue
		}
		for parent != nil && !selector.parts[index-1].matches(parent) {
			parent = parent.Parent
		}
		if parent == nil {
			return false
		}
		node = parent
		index--
	}
	return true
}

func (compound cssCompound) matches(node *html.Node) bool {
	if node == nil || node.Type != html.ElementNode {
		return false
	}
	if compound.tag != "" && node.Data != compound.tag {
		return false
	}
	if compound.id != "" && attribute(node, "id") != compound.id {
		return false
	}
	for _, class := range compound.classes {
		if !hasClass(node, class) {
			return false
		}
	}
	return true
}

func resolveTitleStyle(node *html.Node, rules []cssRule) textStyle {
	var style textStyle
	fontSize := float64(defaultTitleFontSize)
	var ancestors []*html.Node
	for current := node; current != nil; current = current.Parent {
		ancestors = append(ancestors, current)
	}

	for index := len(ancestors) - 1; index >= 0; index-- {
		current := ancestors[index]
		var colorWinner, fontSizeWinner winningDeclaration
		for _, rule := range rules {
			if !rule.selector.matches(current) {
				continue
			}
			for property, declaration := range rule.declarations {
				winner := &colorWinner
				if property == "font-size" {
					winner = &fontSizeWinner
				}
				candidate := winningDeclaration{
					declaration: declaration,
					specificity: rule.selector.specificity,
					order:       rule.order,
					set:         true,
				}
				if outranks(candidate, *winner) {
					*winner = candidate
				}
			}
		}

		inline := parseCSSDeclarations(attribute(current, "style"))
		for property, declaration := range inline {
			winner := &colorWinner
			if property == "font-size" {
				winner = &fontSizeWinner
			}
			candidate := winningDeclaration{declaration: declaration, inline: true, set: true}
			if outranks(candidate, *winner) {
				*winner = candidate
			}
		}

		if colorWinner.set {
			if parsedColor, ok := parseCSSColor(colorWinner.declaration.value); ok {
				style.color = parsedColor
				style.hasColor = true
			}
		}
		if fontSizeWinner.set {
			if parsedSize, ok := resolveFontSize(fontSizeWinner.declaration.value, fontSize); ok {
				fontSize = parsedSize
				style.fontSize = parsedSize
			}
		}
	}
	return style
}

func outranks(candidate, current winningDeclaration) bool {
	if !current.set {
		return true
	}
	if candidate.declaration.important != current.declaration.important {
		return candidate.declaration.important
	}
	if candidate.inline != current.inline {
		return candidate.inline
	}
	if candidate.specificity != current.specificity {
		return compareSpecificity(candidate.specificity, current.specificity) > 0
	}
	return candidate.order >= current.order
}

func compareSpecificity(left, right specificity) int {
	if left.ids != right.ids {
		if left.ids > right.ids {
			return 1
		}
		return -1
	}
	if left.classes != right.classes {
		if left.classes > right.classes {
			return 1
		}
		return -1
	}
	if left.tags > right.tags {
		return 1
	}
	if left.tags < right.tags {
		return -1
	}
	return 0
}

func validFontSize(value string) bool {
	_, ok := resolveFontSize(value, defaultTitleFontSize)
	return ok
}

func resolveFontSize(value string, inherited float64) (float64, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	multiplier := float64(1)
	switch {
	case strings.HasSuffix(value, "px"):
		value = strings.TrimSpace(value[:len(value)-2])
	case strings.HasSuffix(value, "pt"):
		value = strings.TrimSpace(value[:len(value)-2])
		multiplier = 96.0 / 72
	case strings.HasSuffix(value, "rem"):
		value = strings.TrimSpace(value[:len(value)-3])
		multiplier = defaultTitleFontSize
	case strings.HasSuffix(value, "em"):
		value = strings.TrimSpace(value[:len(value)-2])
		multiplier = inherited
	case strings.HasSuffix(value, "%"):
		value = strings.TrimSpace(value[:len(value)-1])
		multiplier = inherited / 100
	default:
		return 0, false
	}
	size, err := strconv.ParseFloat(value, 64)
	if err != nil || size <= 0 {
		return 0, false
	}
	size *= multiplier
	return size, size >= 1 && size <= maxTitleFontSize
}

func parseCSSColor(value string) (color.RGBA, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if strings.HasPrefix(value, "#") {
		hex := value[1:]
		switch len(hex) {
		case 3:
			red, redErr := strconv.ParseUint(strings.Repeat(string(hex[0]), 2), 16, 8)
			green, greenErr := strconv.ParseUint(strings.Repeat(string(hex[1]), 2), 16, 8)
			blue, blueErr := strconv.ParseUint(strings.Repeat(string(hex[2]), 2), 16, 8)
			if redErr == nil && greenErr == nil && blueErr == nil {
				return color.RGBA{R: uint8(red), G: uint8(green), B: uint8(blue), A: 255}, true
			}
		case 6:
			parsed, err := strconv.ParseUint(hex, 16, 24)
			if err == nil {
				return color.RGBA{R: uint8(parsed >> 16), G: uint8(parsed >> 8), B: uint8(parsed), A: 255}, true
			}
		}
		return color.RGBA{}, false
	}

	if strings.HasPrefix(value, "rgb(") && strings.HasSuffix(value, ")") {
		channels := strings.Split(value[4:len(value)-1], ",")
		if len(channels) != 3 {
			return color.RGBA{}, false
		}
		var rgb [3]uint8
		for index, channel := range channels {
			number, err := strconv.Atoi(strings.TrimSpace(channel))
			if err != nil || number < 0 || number > 255 {
				return color.RGBA{}, false
			}
			rgb[index] = uint8(number)
		}
		return color.RGBA{R: rgb[0], G: rgb[1], B: rgb[2], A: 255}, true
	}

	switch value {
	case "black":
		return color.RGBA{A: 255}, true
	case "white":
		return color.RGBA{R: 255, G: 255, B: 255, A: 255}, true
	case "red":
		return color.RGBA{R: 255, A: 255}, true
	case "green":
		return color.RGBA{G: 128, A: 255}, true
	case "blue":
		return color.RGBA{B: 255, A: 255}, true
	case "gray", "grey":
		return color.RGBA{R: 128, G: 128, B: 128, A: 255}, true
	case "orange":
		return color.RGBA{R: 255, G: 165, A: 255}, true
	}
	return color.RGBA{}, false
}
