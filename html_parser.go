package main

import (
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/jomei/notionapi"
)

// ParseHTMLToNotionBlocks parses HTML content and converts it to Notion blocks
type HTMLParser struct{}

// NewHTMLParser creates a new HTML parser instance
func NewHTMLParser() *HTMLParser {
	return &HTMLParser{}
}

// Parse converts HTML string to Notion blocks
func (p *HTMLParser) Parse(htmlContent string) []notionapi.Block {
	if htmlContent == "" {
		return []notionapi.Block{}
	}

	// Pre-process HTML to handle multiple content parts
	content := p.preprocessHTML(htmlContent)
	
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(content))
	if err != nil {
		// If parsing fails, return the content as a simple paragraph
		return p.createFallbackBlock(content)
	}

	var blocks []notionapi.Block
	
	// Process the body or the root element
	body := doc.Find("body")
	if body.Length() == 0 {
		// No body tag, process the document root
		body = doc.Selection
	}

	// Process each top-level element
	body.Children().Each(func(i int, s *goquery.Selection) {
		if newBlocks := p.parseElementToBlocks(s); len(newBlocks) > 0 {
			blocks = append(blocks, newBlocks...)
		}
	})

	return blocks
}

// preprocessHTML cleans and prepares HTML for parsing
func (p *HTMLParser) preprocessHTML(htmlContent string) string {
	// Remove excessive whitespace
	re := regexp.MustCompile(`\s+`)
	content := re.ReplaceAllString(htmlContent, " ")
	
	// Handle multiple content parts separated by newlines
	content = strings.ReplaceAll(content, "\n\n", "</p><p>")
	content = strings.ReplaceAll(content, "\n", "<br>")
	
	// Wrap in basic HTML structure if needed
	if !strings.Contains(content, "<") {
		content = "<p>" + content + "</p>"
	}
	
	return content
}

// createFallbackBlock creates a simple paragraph block when parsing fails
func (p *HTMLParser) createFallbackBlock(content string) []notionapi.Block {
	// Limit content length
	if len(content) > 2000 {
		content = content[:2000] + "..."
	}
	
	return []notionapi.Block{
		notionapi.ParagraphBlock{
			RichText: []notionapi.RichText{
				{
					Type: notionapi.ObjectTypeText,
					Text: notionapi.Text{
						Content: content,
					},
					PlainText: content,
				},
			},
		},
	}
}

// parseElementToBlocks recursively parses a DOM element and converts it to Notion blocks
// Returns multiple blocks to handle elements like lists that need multiple blocks
func (p *HTMLParser) parseElementToBlocks(s *goquery.Selection) []notionapi.Block {
	tagName := goquery.NodeName(s)
	
	switch tagName {
	case "p":
		if block := p.parseParagraph(s); block != nil {
			return []notionapi.Block{block}
		}
	case "h1", "h2", "h3", "h4", "h5", "h6":
		if block := p.parseHeading(s, tagName); block != nil {
			return []notionapi.Block{block}
		}
	case "ul":
		return p.parseUnorderedList(s)
	case "ol":
		return p.parseOrderedList(s)
	case "blockquote":
		if block := p.parseQuote(s); block != nil {
			return []notionapi.Block{block}
		}
	case "pre":
		if block := p.parseCodeBlock(s); block != nil {
			return []notionapi.Block{block}
		}
	case "hr":
		return []notionapi.Block{notionapi.DividerBlock{}}
	case "img":
		if block := p.parseImage(s); block != nil {
			return []notionapi.Block{block}
		}
	case "br":
		// Line break within text, handled at rich text level
		return nil
	case "div", "section", "article", "main", "span":
		// For container elements, process children
		var blocks []notionapi.Block
		s.Children().Each(func(i int, child *goquery.Selection) {
			if childBlocks := p.parseElementToBlocks(child); len(childBlocks) > 0 {
				blocks = append(blocks, childBlocks...)
			}
		})
		return blocks
	default:
		// For unknown elements, try to extract text content
		if text := strings.TrimSpace(s.Text()); text != "" {
			return []notionapi.Block{
				notionapi.ParagraphBlock{
					RichText: []notionapi.RichText{
						{
							Type: notionapi.ObjectTypeText,
							Text: notionapi.Text{
								Content: text,
							},
							PlainText: text,
						},
					},
				},
			}
		}
	}
	
	return nil
}

// parseParagraph converts a paragraph element to Notion paragraph block
func (p *HTMLParser) parseParagraph(s *goquery.Selection) notionapi.Block {
	richText := p.extractRichText(s)
	if len(richText) == 0 {
		return nil
	}
	return notionapi.ParagraphBlock{
		RichText: richText,
	}
}

// parseHeading converts heading elements to Notion heading blocks
func (p *HTMLParser) parseHeading(s *goquery.Selection, tagName string) notionapi.Block {
	richText := p.extractRichText(s)
	if len(richText) == 0 {
		return nil
	}

	// Determine heading level
	level := 1
	switch tagName {
	case "h1":
		level = 1
	case "h2":
		level = 2
	case "h3":
		level = 3
	default:
		level = 3 // h4, h5, h6 are mapped to heading_3
	}

	return notionapi.HeadingBlock{
		RichText: richText,
		Level:    level,
	}
}

// parseUnorderedList converts UL elements to Notion bulleted list blocks
func (p *HTMLParser) parseUnorderedList(s *goquery.Selection) []notionapi.Block {
	var blocks []notionapi.Block
	
	s.Find("li").Each(func(i int, li *goquery.Selection) {
		richText := p.extractRichText(li)
		if len(richText) > 0 {
			// Check for nested lists
			if li.Find("ul, ol").Length() > 0 {
				// Handle nested lists by adding the list item text first
				blocks = append(blocks, notionapi.BulletedListItemBlock{
					RichText: richText,
				})
				// Process nested lists
				li.Children().Each(func(j int, child *goquery.Selection) {
					if goquery.NodeName(child) == "ul" || goquery.NodeName(child) == "ol" {
						if nestedBlocks := p.parseElementToBlocks(child); len(nestedBlocks) > 0 {
							blocks = append(blocks, nestedBlocks...)
						}
					}
				})
			} else {
				blocks = append(blocks, notionapi.BulletedListItemBlock{
					RichText: richText,
				})
			}
		}
	})

	return blocks
}

// parseOrderedList converts OL elements to Notion numbered list blocks
func (p *HTMLParser) parseOrderedList(s *goquery.Selection) []notionapi.Block {
	var blocks []notionapi.Block
	
	s.Find("li").Each(func(i int, li *goquery.Selection) {
		richText := p.extractRichText(li)
		if len(richText) > 0 {
			blocks = append(blocks, notionapi.NumberedListItemBlock{
				RichText: richText,
			})
		}
	})

	return blocks
}

// parseQuote converts blockquote to Notion quote block
func (p *HTMLParser) parseQuote(s *goquery.Selection) notionapi.Block {
	richText := p.extractRichText(s)
	if len(richText) == 0 {
		return nil
	}
	return notionapi.QuoteBlock{
		RichText: richText,
	}
}

// parseCodeBlock converts pre/code elements to Notion code block
func (p *HTMLParser) parseCodeBlock(s *goquery.Selection) notionapi.Block {
	// Try to find code element inside pre
	code := s.Find("code")
	var content string
	if code.Length() > 0 {
		content = code.Text()
	} else {
		content = s.Text()
	}
	
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}

	// Try to detect language from class
	lang := "plain text"
	if code.Length() > 0 {
		class, exists := code.Attr("class")
		if exists && strings.Contains(class, "language-") {
			parts := strings.Split(class, "language-")
			if len(parts) > 1 {
				lang = strings.TrimSpace(parts[1])
				// Notion only supports specific languages, default to plain text
				if !isSupportedLanguage(lang) {
					lang = "plain text"
				}
			}
		}
	}

	return notionapi.CodeBlock{
		RichText: []notionapi.RichText{
			{
				Type: notionapi.ObjectTypeText,
				Text: notionapi.Text{
					Content: content,
				},
				PlainText: content,
			},
		},
		Language: lang,
	}
}

// parseImage converts img element to Notion image block
func (p *HTMLParser) parseImage(s *goquery.Selection) notionapi.Block {
	src, exists := s.Attr("src")
	if !exists || src == "" {
		return nil
	}

	// Skip if not HTTP URL
	if !strings.HasPrefix(src, "http") {
		return nil
	}

	alt, _ := s.Attr("alt")
	var caption []notionapi.RichText
	if alt != "" {
		caption = []notionapi.RichText{
			{
				Type: notionapi.ObjectTypeText,
				Text: notionapi.Text{
					Content: alt,
				},
				PlainText: alt,
			},
		}
	}

	return notionapi.ImageBlock{
		Type: "external",
		External: &notionapi.FileObject{
			URL: src,
		},
		Caption: caption,
	}
}

// extractRichText extracts rich text with formatting from an element
func (p *HTMLParser) extractRichText(s *goquery.Selection) []notionapi.RichText {
	var richTexts []notionapi.RichText
	
	// Process child nodes
	s.Contents().Each(func(i int, node *goquery.Selection) {
		if richText := p.processNode(node); richText != nil {
			richTexts = append(richTexts, *richText)
		}
	})

	return richTexts
}

// processNode processes individual text nodes and elements
func (p *HTMLParser) processNode(node *goquery.Selection) *notionapi.RichText {
	nodeType := node.Get(0).Type
	
	// Text node
	if nodeType == 3 { // TextNode = 3
		text := strings.TrimSpace(node.Text())
		if text == "" {
			return nil
		}
		return &notionapi.RichText{
			Type: notionapi.ObjectTypeText,
			Text: notionapi.Text{
				Content: text,
			},
			PlainText: text,
		}
	}
	
	// Element node
	if nodeType == 1 { // ElementNode = 1
		tagName := goquery.NodeName(node)
		switch tagName {
		case "a":
			return p.processLink(node)
		case "strong", "b":
			return p.processBold(node)
		case "em", "i":
			return p.processItalic(node)
		case "code":
			return p.processCode(node)
		case "br":
			return &notionapi.RichText{
				Type: notionapi.ObjectTypeText,
				Text: notionapi.Text{
					Content: "\n",
				},
				PlainText: "\n",
			}
		case "span", "div":
			// Process children for span/div
			var combinedText string
			var richTexts []notionapi.RichText
			
			node.Contents().Each(func(i int, child *goquery.Selection) {
				if rt := p.processNode(child); rt != nil {
					richTexts = append(richTexts, *rt)
					combinedText += rt.PlainText
				}
			})
			
			if combinedText != "" {
				return &notionapi.RichText{
					Type:      notionapi.ObjectTypeText,
					Text:      notionapi.Text{Content: combinedText},
					PlainText: combinedText,
				}
			}
			return nil
		default:
			// For other elements, extract text content
			text := strings.TrimSpace(node.Text())
			if text == "" {
				return nil
			}
			return &notionapi.RichText{
				Type: notionapi.ObjectTypeText,
				Text: notionapi.Text{
					Content: text,
				},
				PlainText: text,
			}
		}
	}
	
	return nil
}

// processLink creates a rich text with link annotation
func (p *HTMLParser) processLink(s *goquery.Selection) *notionapi.RichText {
	text := strings.TrimSpace(s.Text())
	if text == "" {
		return nil
	}

	href, exists := s.Attr("href")
	if !exists || href == "" {
		// No href, treat as normal text
		return &notionapi.RichText{
			Type: notionapi.ObjectTypeText,
			Text: notionapi.Text{
				Content: text,
			},
			PlainText: text,
		}
	}

	return &notionapi.RichText{
		Type: notionapi.ObjectTypeText,
		Text: notionapi.Text{
			Content: text,
			Link: &notionapi.Link{
				URL: href,
			},
		},
		PlainText: text,
		Href:      href,
		Annotations: &notionapi.Annotations{
			Underline: true,
		},
	}
}

// processBold creates bold rich text
func (p *HTMLParser) processBold(s *goquery.Selection) *notionapi.RichText {
	text := strings.TrimSpace(s.Text())
	if text == "" {
		return nil
	}

	return &notionapi.RichText{
		Type: notionapi.ObjectTypeText,
		Text: notionapi.Text{
			Content: text,
		},
		PlainText: text,
		Annotations: &notionapi.Annotations{
			Bold: true,
		},
	}
}

// processItalic creates italic rich text
func (p *HTMLParser) processItalic(s *goquery.Selection) *notionapi.RichText {
	text := strings.TrimSpace(s.Text())
	if text == "" {
		return nil
	}

	return &notionapi.RichText{
		Type: notionapi.ObjectTypeText,
		Text: notionapi.Text{
			Content: text,
		},
		PlainText: text,
		Annotations: &notionapi.Annotations{
			Italic: true,
		},
	}
}

// processCode creates inline code rich text
func (p *HTMLParser) processCode(s *goquery.Selection) *notionapi.RichText {
	text := strings.TrimSpace(s.Text())
	if text == "" {
		return nil
	}

	return &notionapi.RichText{
		Type: notionapi.ObjectTypeText,
		Text: notionapi.Text{
			Content: text,
		},
		PlainText: text,
		Annotations: &notionapi.Annotations{
			Code: true,
		},
	}
}

// isSupportedLanguage checks if the language is supported by Notion
func isSupportedLanguage(lang string) bool {
	supportedLanguages := map[string]bool{
		"abap": true, "arduino": true, "bash": true, "basic": true, "c": true,
		"clojure": true, "coffeescript": true, "c++": true, "c#": true, "css": true,
		"dart": true, "diff": true, "docker": true, "elixir": true, "elm": true,
		"erlang": true, "flow": true, "fortran": true, "f#": true, "gherkin": true,
		"glsl": true, "go": true, "graphql": true, "groovy": true, "haskell": true,
		"html": true, "java": true, "javascript": true, "json": true, "julia": true,
		"kotlin": true, "latex": true, "less": true, "lisp": true, "livescript": true,
		"lua": true, "makefile": true, "markdown": true, "markup": true, "matlab": true,
		"mercury": true, "nix": true, "objective-c": true, "ocaml": true, "pascal": true,
		"perl": true, "php": true, "plain text": true, "powershell": true, "prolog": true,
		"protobuf": true, "python": true, "r": true, "reason": true, "ruby": true,
		"rust": true, "sass": true, "scala": true, "scheme": true, "scss": true,
		"shell": true, "sql": true, "swift": true, "typescript": true, "vb.net": true,
		"verilog": true, "vhdl": true, "visual basic": true, "webassembly": true, "xml": true,
		"yaml": true,
	}
	return supportedLanguages[strings.ToLower(lang)]
}
