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

var svgCSSURL = regexp.MustCompile(`(?i)url\s*\(([^)]*)\)`)

// pelicanSVG 提取可作为独立图像显示的 SVG。HTML 中的脚本、样式和说明文字留在包装页内。
func pelicanSVG(output string) string {
	tokens := html.NewTokenizer(strings.NewReader(output))
	var artwork string
	var depth, offset, start int
	for {
		token := tokens.Next()
		position := offset
		offset += len(tokens.Raw())
		if token == html.ErrorToken {
			if tokens.Err() != io.EOF || depth != 0 {
				return ""
			}
			return artwork
		}
		name, _ := tokens.TagName()
		if string(name) != "svg" {
			continue
		}
		switch token {
		case html.StartTagToken:
			if depth == 0 {
				if artwork != "" {
					return ""
				}
				start = position
			}
			depth++
		case html.EndTagToken:
			if depth == 0 {
				return ""
			}
			depth--
			if depth == 0 {
				artwork = standaloneSVG(output[start:offset])
				if artwork == "" {
					return ""
				}
			}
		case html.SelfClosingTagToken:
			if depth == 0 {
				return ""
			}
		}
	}
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
