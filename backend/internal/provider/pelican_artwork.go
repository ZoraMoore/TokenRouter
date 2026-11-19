package provider

import (
	"encoding/xml"
	"io"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

const (
	svgNamespace   = "http://www.w3.org/2000/svg"
	xlinkNamespace = "http://www.w3.org/1999/xlink"
)

var (
	svgCSSURL         = regexp.MustCompile(`(?i)url\s*\(([^)]*)\)`)
	pelicanBareMarkup = regexp.MustCompile(`(?im)^[\t ]*<(?:!doctype\s+html|html|svg|canvas)(?:[\s/>]|$)`)
	pelicanMarkupTag  = regexp.MustCompile(`(?i)<(?:!doctype\s+html|html|svg|canvas)(?:[\s/>]|$)`)
)

type pelicanFigure struct {
	length          int
	graphic, script bool
}

// pelicanPage 记录回答中当前一份 HTML 文档或 SVG 片段的绘图内容。
type pelicanPage struct {
	document, closed, body, canvas bool
	hasCanvas, hasBody, hasScript  bool
	hasGraphic, scriptedSVG        bool
	rawElement                     string
	figures                        int
	image                          string
}

// pelicanAnswerMarkup 选取最后一段绘图代码。末段缺少围栏时仍按其中的 HTML 检查完整性。
func pelicanAnswerMarkup(source string) string {
	var chosen string
	var start, offset int
	var inFence bool
	candidate := pelicanBareMarkup
	for line := range strings.SplitAfterSeq(source, "\n") {
		heading := strings.TrimSpace(line)
		if strings.HasPrefix(heading, "```") && (!inFence || heading == "```") {
			if block := source[start:offset]; candidate != nil && candidate.MatchString(block) {
				chosen = block
			}
			inFence = !inFence
			candidate = pelicanBareMarkup
			if inFence {
				switch strings.ToLower(heading[3:]) {
				case "", "html", "xml", "svg":
					candidate = pelicanMarkupTag
				default:
					candidate = nil
				}
			}
			start = offset + len(line)
		}
		offset += len(line)
	}
	if start == 0 {
		return source
	}
	if block := source[start:]; candidate != nil && candidate.MatchString(block) {
		return block
	}
	return chosen
}

// pelicanArtwork 检查整份绘图回答。完整网页交给沙盒预览，单幅 SVG 还可作为图片导出。
func pelicanArtwork(source string) (string, bool) {
	source = pelicanAnswerMarkup(source)
	tokens := html.NewTokenizer(strings.NewReader(source))
	var page pelicanPage
	var offset int
	for {
		token := tokens.Next()
		start := offset
		offset += len(tokens.Raw())
		switch token {
		case html.ErrorToken:
			// 残缺的起始标签也会返回 EOF，此时 Raw 仍包含未读完的标签。
			if tokens.Err() != io.EOF || offset != start || page.rawElement != "" {
				return "", false
			}
			if page.document {
				hasDrawing := page.hasGraphic || page.hasCanvas || (page.hasBody && page.hasScript)
				if !page.closed || !hasDrawing {
					return "", false
				}
				return page.image, true
			}
			return page.image, !page.body && page.figures == 1 && (page.image != "" || page.scriptedSVG)
		case html.DoctypeToken:
			fields := strings.Fields(string(tokens.Text()))
			if len(fields) > 0 && strings.EqualFold(fields[0], "html") {
				page = pelicanPage{document: true}
			}
		case html.StartTagToken, html.SelfClosingTagToken, html.EndTagToken:
			tag, _ := tokens.TagName()
			switch name := string(tag); name {
			case "html":
				switch token {
				case html.StartTagToken:
					page = pelicanPage{document: true}
				case html.EndTagToken:
					page.closed = true
				}
			case "body":
				page.body = token == html.StartTagToken
				page.hasBody = page.hasBody || page.body
			case "canvas":
				if token == html.EndTagToken {
					page.hasCanvas = page.hasCanvas || page.canvas
				}
				page.canvas = page.body && token == html.StartTagToken
			case "script", "style":
				if token == html.StartTagToken {
					page.rawElement = name
				} else if token == html.EndTagToken && page.rawElement == name {
					page.hasScript = page.hasScript || name == "script"
					page.rawElement = ""
				}
			case "svg":
				if token == html.EndTagToken {
					return "", false
				}
				page.figures++
				if token == html.StartTagToken {
					figure, valid := readPelicanSVG(tokens)
					if !valid {
						return "", false
					}
					offset += figure.length
					page.hasGraphic = page.hasGraphic || figure.graphic || figure.script
					page.scriptedSVG = page.scriptedSVG || figure.script
				}
				if page.figures == 1 {
					page.image = standaloneSVG(source[start:offset])
				} else {
					// 多幅 SVG 共享页面布局和样式，需要显示整页。
					page.image = ""
				}
			}
		}
	}
}

// readPelicanSVG 读到配对的结束标签，区分可见图形与 defs 中的图形定义。
func readPelicanSVG(tokens *html.Tokenizer) (pelicanFigure, bool) {
	var figure pelicanFigure
	stack := []string{"svg"}
	var definitionDepth, foreignDepth int
	for len(stack) > 0 {
		token := tokens.Next()
		figure.length += len(tokens.Raw())
		if token == html.ErrorToken {
			return figure, false
		}
		if token != html.StartTagToken && token != html.SelfClosingTagToken && token != html.EndTagToken {
			continue
		}
		rawName, _ := tokens.TagName()
		name := string(rawName)
		// foreignObject 的子元素按 HTML 解析，img、br 等标签可以省略结束标签。
		if foreignDepth > 0 && name != "foreignobject" {
			continue
		}
		if token == html.EndTagToken {
			depth := len(stack)
			if stack[depth-1] != name {
				return figure, false
			}
			if definitionDepth == depth {
				definitionDepth = 0
			}
			switch name {
			case "foreignobject":
				foreignDepth--
			case "script":
				figure.script = true
			}
			stack = stack[:depth-1]
			continue
		}
		switch name {
		case "path", "rect", "circle", "ellipse", "line", "polyline", "polygon", "text", "use", "image", "foreignobject":
			figure.graphic = figure.graphic || definitionDepth == 0
		}
		if token == html.StartTagToken {
			stack = append(stack, name)
			switch name {
			case "defs", "symbol", "clippath", "mask", "pattern", "marker", "metadata":
				if definitionDepth == 0 {
					definitionDepth = len(stack)
				}
			case "foreignobject":
				foreignDepth++
			}
		}
	}
	return figure, true
}

// standaloneSVG 用 XML 规则检查标签配对，并给 HTML 内联图形补上图像命名空间。
func standaloneSVG(source string) string {
	decoder := xml.NewDecoder(strings.NewReader(source))
	var depth, definitionDepth int
	var graphic, needsNamespace, needsXLink bool
	for {
		token, err := decoder.Token()
		if err == io.EOF && depth == 0 && graphic {
			if needsNamespace || needsXLink {
				bindings := ""
				if needsNamespace {
					bindings = ` xmlns="` + svgNamespace + `"`
				}
				if needsXLink {
					bindings += ` xmlns:xlink="` + xlinkNamespace + `"`
				}
				source = "<svg" + bindings + source[len("<svg"):]
			}
			if len(source) > PelicanDrawingLimit {
				return ""
			}
			return source
		}
		if err != nil {
			return ""
		}
		switch value := token.(type) {
		case xml.StartElement:
			if value.Name.Space != "" && !strings.Contains(value.Name.Space, ":") {
				return ""
			}
			if depth == 0 {
				if value.Name.Local != "svg" || (value.Name.Space != "" && value.Name.Space != svgNamespace) {
					return ""
				}
				needsNamespace = value.Name.Space == ""
			}
			depth++
			valid, missingXLink := svgAttributes(value.Attr, depth == 1)
			if !valid {
				return ""
			}
			needsXLink = needsXLink || missingXLink
			switch strings.ToLower(value.Name.Local) {
			case "script", "foreignobject":
				return ""
			case "defs", "symbol", "clippath", "mask", "pattern", "marker", "metadata":
				if definitionDepth == 0 {
					definitionDepth = depth
				}
			case "path", "rect", "circle", "ellipse", "line", "polyline", "polygon", "text", "use", "image":
				graphic = graphic || definitionDepth == 0
			}
		case xml.EndElement:
			if depth == definitionDepth {
				definitionDepth = 0
			}
			depth--
		case xml.CharData:
			if !localSVGReferences(string(value)) {
				return ""
			}
		case xml.Directive, xml.ProcInst:
			return ""
		}
	}
}

// svgAttributes 检查 XML 属性并识别缺少声明的 HTML xlink 引用。
func svgAttributes(attrs []xml.Attr, root bool) (valid, missingXLink bool) {
	seen := make(map[xml.Name]bool, len(attrs))
	for _, attr := range attrs {
		name := strings.ToLower(attr.Name.Local)
		if seen[attr.Name] || strings.HasPrefix(name, "on") || !localSVGReferences(attr.Value) {
			return false, false
		}
		seen[attr.Name] = true
		switch attr.Name.Space {
		case "":
		case "xmlns":
			if name == "xlink" && attr.Value != xlinkNamespace {
				return false, false
			}
		case "xlink":
			missingXLink = true
		default:
			// encoding/xml 会把没有绑定到命名空间的前缀原样放进 Space。
			if !strings.Contains(attr.Name.Space, ":") {
				return false, false
			}
		}
		switch name {
		case "href", "src":
			if !strings.HasPrefix(strings.TrimSpace(attr.Value), "#") {
				return false, false
			}
		case "base":
			if attr.Name.Space == "http://www.w3.org/XML/1998/namespace" {
				return false, false
			}
		case "xmlns":
			if root && attr.Value != svgNamespace {
				return false, false
			}
		}
	}
	return true, missingXLink
}

// localSVGReferences 允许图形和动画通过片段 ID 引用同一幅 SVG 的定义。
func localSVGReferences(text string) bool {
	if strings.Contains(strings.ToLower(text), "@import") {
		return false
	}
	for _, match := range svgCSSURL.FindAllStringSubmatch(text, -1) {
		target := strings.Trim(strings.TrimSpace(match[1]), "'\"")
		if !strings.HasPrefix(target, "#") {
			return false
		}
	}
	return true
}
