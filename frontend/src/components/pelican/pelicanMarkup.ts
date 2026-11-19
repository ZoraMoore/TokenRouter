// 模型有时先写草稿再给出最终代码。识别最后一份作品，包括还没写完的代码块。
function artworkBlock(answer: string) {
  const fence = /^[ \t]*```([^\s`]*)[ \t]*\r?$/gm
  const artworkTag = /<(?:!doctype\s+html|html|svg|canvas)(?=[\s/>]|$)/i
  const standaloneTag = /^[ \t]*<(?:!doctype\s+html|html|svg|canvas)(?=[\s/>]|$)/im
  let last = ''
  let start = 0
  let language: string | null = null
  function consider(block: string) {
    // 围栏外的说明可以提到 <svg>。另起一行的作品可替换前面的草稿。
    if (language && !/^(?:html|svg|xml)$/i.test(language)) return
    const tag = language === null ? standaloneTag : artworkTag
    if (tag.test(block)) last = block
  }
  for (const match of answer.matchAll(fence)) {
    if (language !== null && match[1]) continue
    consider(answer.slice(start, match.index))
    language = language === null ? match[1] : null
    start = match.index + match[0].length
  }
  if (start === 0) return answer
  consider(answer.slice(start))
  return last
}

// 浏览器会补齐截断的 HTML。先定位原文中的文档或 SVG 结束标签，再把作品交给沙盒。
export function extractPelicanMarkup(answer: string): string {
  const text = artworkBlock(answer)
  const tags = /<!--|<(\/)?(!doctype\s+html|[a-z][\w:-]*)(?=[\s/>])(?:[^"'><]|"[^"]*"|'[^']*')*>|<\/?[a-z][\w:-]*(?=[\s/>]|$)/gi
  let start = -1
  let doctype = -1
  let documentStart = -1
  let documentEnd = -1
  let markupEnd = 0
  let svgDepth = 0
  let hasSVG = false
  let tag: RegExpExecArray | null
  while ((tag = tags.exec(text))) {
    if (tag[0] === '<!--') {
      const end = text.indexOf('-->', tags.lastIndex)
      if (end < 0) return ''
      tags.lastIndex = end + 3
      continue
    }
    if (!tag[2]) return ''
    const name = tag[2].toLowerCase()
    const closing = !!tag[1]
    if (start < 0 && !closing) start = tag.index
    if (name.startsWith('!doctype')) {
      doctype = tag.index
      documentStart = doctype
      documentEnd = -1
      svgDepth = 0
    } else if (name === 'html') {
      if (closing) {
        documentEnd = tags.lastIndex
      } else {
        documentStart = doctype >= 0 ? doctype : tag.index
        documentEnd = -1
        doctype = -1
        svgDepth = 0
      }
    } else if (name === 'svg') {
      hasSVG = true
      if (closing) svgDepth--
      else if (!tag[0].endsWith('/>')) svgDepth++
      if (svgDepth < 0) return ''
    } else if (!closing && /^(script|style|textarea|title)$/.test(name) && !(svgDepth && tag[0].endsWith('/>'))) {
      // 这些元素的正文里可以出现看似文档标签的字符串。
      const end = new RegExp('</' + name + '\\s*>', 'ig')
      end.lastIndex = tags.lastIndex
      if (!end.exec(text)) return ''
      tags.lastIndex = end.lastIndex
    }
    markupEnd = tags.lastIndex
  }
  if (svgDepth !== 0 || start < 0) return ''
  if (documentStart >= 0) return documentEnd > documentStart ? text.slice(documentStart, documentEnd) : ''
  return hasSVG ? text.slice(start, markupEnd) : ''
}
