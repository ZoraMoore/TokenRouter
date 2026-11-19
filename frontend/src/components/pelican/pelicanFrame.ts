export interface PelicanCanvas {
  width: number
  height: number
}

export type PelicanPreviewMode = 'fit' | 'actual'

const defaultViewport: PelicanCanvas = { width: 1024, height: 768 }
const maxDimension = 8192
export const pelicanPreviewUpdateLimit = 120
const noncePattern = /^[A-Za-z0-9+/_=-]{1,256}$/
const offlinePolicy = "default-src 'none'; frame-src 'none'; connect-src 'none'; object-src 'none'; form-action 'none'; base-uri 'none'"

export function pelicanSvgCanvas(svg: Element | null): PelicanCanvas {
  if (!svg) return defaultViewport
  function pixels(value: string | null) {
    const match = /^\s*(\d*\.?\d+(?:e[+-]?\d+)?)(?:px)?\s*$/i.exec(value || '')
    return Number(match?.[1])
  }

  let width = pixels(svg.getAttribute('width'))
  let height = pixels(svg.getAttribute('height'))
  const box = svg.getAttribute('viewBox')?.trim().split(/[\s,]+/).map(Number)
  if (box?.length === 4 && box.every(Number.isFinite) && validDimension(box[2]) && validDimension(box[3])) {
    const ratio = box[2] / box[3]
    if (!validDimension(width)) width = validDimension(height) ? height * ratio : box[2]
    if (!validDimension(height)) height = width / ratio
  }
  return validDimension(width) && validDimension(height) ? { width, height } : defaultViewport
}

export function fitPelicanCanvas(canvas: PelicanCanvas, available: PelicanCanvas) {
  return Math.max(0, Math.min(1, available.width / canvas.width, available.height / canvas.height))
}

function validDimension(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 && value <= maxDimension
}

export function readPelicanCanvas(event: MessageEvent, frame: Window | undefined, channel: string): PelicanCanvas | null {
  if (!frame || !channel || event.source !== frame || event.origin !== 'null' || !event.data || typeof event.data !== 'object') return null
  const { type, token, width, height } = event.data
  if (type !== 'pelican-preview:size' || token !== channel || !validDimension(width) || !validDimension(height)) return null
  return { width, height }
}

// 外层 CSP 限制内部作品框架的跳转。作品框架对本站使用不透明源，动画脚本在其中执行。
export function createPelicanFrame(markup: string, interactive = false) {
  const channel = Array.from(crypto.getRandomValues(new Uint8Array(16)), value => value.toString(16).padStart(2, '0')).join('')
  const pageNonce = document.head.querySelector<HTMLScriptElement>('script[nonce]')?.nonce || ''
  const nonce = noncePattern.test(pageNonce) ? pageNonce : channel
  const doc = new DOMParser().parseFromString(markup, 'text/html')
  // DOMParser 使用独立文档。作品节点只进入沙盒文档，当前页面用它们读取视口尺寸。
  for (const node of doc.querySelectorAll('base, meta[http-equiv], link, iframe, frame, object, embed')) node.remove()
  for (const script of doc.querySelectorAll('script')) {
    if (script.hasAttribute('src') || script.hasAttribute('href') || script.hasAttribute('xlink:href') ||
      script.getAttribute('type')?.trim().toLowerCase() === 'speculationrules') {
      script.remove()
    } else {
      script.setAttribute('nonce', nonce)
    }
  }
  const visualElements = Array.from(doc.body.children).filter(node => !['style', 'script', 'meta', 'template'].includes(node.localName))
  const svg = visualElements.length === 1 && visualElements[0].localName === 'svg' &&
    !Array.from(doc.body.childNodes).some(node => node.nodeType === Node.TEXT_NODE && node.textContent?.trim())
    ? visualElements[0]
    : null
  const viewport = pelicanSvgCanvas(svg)

  if (/^<svg(?=[\s/>])/i.test(markup)) {
    const style = doc.createElement('style')
    style.textContent = ':where(html,body){height:100%;margin:0}:where(body){display:grid;place-items:center}:where(body>svg){display:block;max-width:100%;max-height:100%}'
    doc.head.prepend(style)
  }

  const measurement = doc.createElement('script')
  measurement.setAttribute('nonce', nonce)
  measurement.textContent = `(${measurePelicanDocument.toString()})(window,${JSON.stringify(channel)},${JSON.stringify(viewport)},${maxDimension},${pelicanPreviewUpdateLimit})`
  doc.head.prepend(measurement)
  const policy = doc.createElement('meta')
  policy.httpEquiv = 'Content-Security-Policy'
  policy.content = `${offlinePolicy}; script-src 'nonce-${nonce}'; style-src 'unsafe-inline'; img-src data: blob:; media-src data: blob:; font-src data:`
  doc.head.prepend(policy)

  const artwork = document.createElement('iframe')
  artwork.srcdoc = '<!doctype html>' + doc.documentElement.outerHTML
  artwork.setAttribute('sandbox', interactive ? 'allow-scripts allow-pointer-lock' : 'allow-scripts')
  artwork.referrerPolicy = 'no-referrer'
  artwork.setAttribute('scrolling', 'no')
  if (!interactive) artwork.tabIndex = -1
  artwork.width = String(viewport.width)
  artwork.height = String(viewport.height)
  artwork.style.cssText = 'display:block;border:0'
  const framePolicy = policy.content
  const bridge = `(${mountPelicanArtwork.toString()})(${JSON.stringify(channel)})`
  const srcdoc = `<!doctype html><html><head><meta http-equiv="Content-Security-Policy" content="${framePolicy}"></head><body style="height:100vh;margin:0"><template>${artwork.outerHTML}</template><script nonce="${nonce}">${bridge}</script></body></html>`
  return { srcdoc, viewport, channel }
}

// 隔离外壳在布局后挂载作品，确保作答脚本首读的视口已有尺寸。
function mountPelicanArtwork(token: string) {
  const template = document.querySelector('template')
  const artwork = template?.content.querySelector('iframe')
  if (!artwork) return
  window.addEventListener('message', (event: MessageEvent) => {
    if (event.origin !== 'null' || event.source !== artwork.contentWindow || event.data?.type !== 'pelican-preview:size' || event.data.token !== token) return
    const { width, height } = event.data
    window.parent.postMessage({ type: 'pelican-preview:size', token, width, height }, '*')
  })
  requestAnimationFrame(() => {
    document.body.append(artwork)
    template?.remove()
  })
}

// 此函数序列化后在作品框架中运行。它通过参数接收视口与上限，并按同一像素基准测量全部显示模式。
function measurePelicanDocument(target: Window & typeof globalThis, token: string, viewport: PelicanCanvas, dimensionLimit: number, updateLimit: number) {
  const doc = target.document
  const root = doc.documentElement
  const observed = new WeakSet<Element>()
  const sampleTimers: number[] = []
  let timer = 0
  let updates = 0
  let stopped = false
  let originX = 0
  let originY = 0
  let innerScale = 1
  let authorZoom = 1
  let translateX = '0px'
  let translateY = '0px'
  let lastWidth = 0
  let lastHeight = 0
  let resizeObserver: ResizeObserver | undefined
  let mutationObserver: MutationObserver | undefined

  function start() {
    if (!doc.body || stopped) return
    freezeStyleRules()
    const style = target.getComputedStyle(root)
    const zoom = style.getPropertyValue('zoom')
    const numericZoom = parseFloat(zoom)
    if (Number.isFinite(numericZoom) && numericZoom > 0) authorZoom = zoom.endsWith('%') ? numericZoom / 100 : numericZoom
    const translation = style.translate?.match(/(?:\([^)]*\)|[^\s(])+/g)
    if (translation && style.translate !== 'none') {
      translateX = translation[0]
      translateY = translation[1] || '0px'
    }
    // 固定百分比的计算基准，使压缩后的画布测量收敛。
    setRootPixels('width', style.width, viewport.width)
    setRootPixels('height', style.height, viewport.height)
    if (target.ResizeObserver) resizeObserver = new target.ResizeObserver(schedule)
    if (target.MutationObserver) {
      mutationObserver = new target.MutationObserver(schedule)
      mutationObserver.observe(root, { subtree: true, childList: true, attributes: true, characterData: true })
    }
    doc.addEventListener('load', schedule, true)
    target.addEventListener('resize', schedule)
    doc.fonts?.ready.then(schedule).catch(() => {})
    measure()
    for (const delay of [100, 500, 1500, 5000]) sampleTimers.push(target.setTimeout(schedule, delay))
  }

  function setRootPixels(property: string, value: string, fallback: number) {
    const pixels = parseFloat(value)
    const size = value.endsWith('px') && pixels > 0 ? Math.min(dimensionLimit, pixels) : fallback
    root.style.setProperty(property, `${size}px`, 'important')
  }

  function measure() {
    timer = 0
    if (stopped || updates >= updateLimit) return
    freezeStyleRules()
    let minX = 0
    let minY = 0
    let maxX = Math.max(viewport.width, doc.body.scrollWidth * authorZoom)
    let maxY = Math.max(viewport.height, doc.body.scrollHeight * authorZoom)
    const elements = doc.querySelectorAll<HTMLElement | SVGElement>('body, body *')
    for (let i = 0; i < Math.min(elements.length, 2000); i++) {
      const element = elements[i]
      if (element.style) freezeUnits(element.style)
      const svg = element.closest('svg')
      if (svg && svg !== element) continue
      if (!observed.has(element)) {
        resizeObserver?.observe(element)
        observed.add(element)
      }
      const rect = element.getBoundingClientRect()
      if (!rect.width && !rect.height) continue
      minX = Math.min(minX, (rect.left + target.scrollX) / innerScale - originX)
      minY = Math.min(minY, (rect.top + target.scrollY) / innerScale - originY)
      maxX = Math.max(maxX, (rect.right + target.scrollX) / innerScale - originX)
      maxY = Math.max(maxY, (rect.bottom + target.scrollY) / innerScale - originY)
    }
    const nextOriginX = Math.min(dimensionLimit, Math.max(originX, Math.ceil(-minX)))
    const nextOriginY = Math.min(dimensionLimit, Math.max(originY, Math.ceil(-minY)))
    if (nextOriginX !== originX || nextOriginY !== originY) {
      originX = nextOriginX
      originY = nextOriginY
      root.style.setProperty(
        'translate',
        `calc(${translateX} + ${originX / authorZoom}px) calc(${translateY} + ${originY / authorZoom}px)`,
        'important'
      )
    }

    // 每次测量保留已发现的绘制范围，动画移动时画廊尺寸保持稳定。
    const width = Math.min(dimensionLimit, Math.max(lastWidth, Math.ceil(maxX + originX)))
    const height = Math.min(dimensionLimit, Math.max(lastHeight, Math.ceil(maxY + originY)))
    if (width === lastWidth && height === lastHeight) return
    lastWidth = width
    lastHeight = height
    innerScale = Math.min(1, viewport.width / width, viewport.height / height)
    root.style.setProperty('zoom', String(authorZoom * innerScale), 'important')
    target.parent.postMessage({ type: 'pelican-preview:size', token, width, height }, '*')
    updates++
  }

  function freezeStyleRules() {
    // 视口单位按作品的初始视口计算。作者写入的媒体查询仍由固定尺寸的 iframe 求值。
    freezeUnits(root.style)
    for (const sheet of doc.styleSheets) {
      try {
        freezeRules(sheet.cssRules)
      } catch {
        // 浏览器可以限制部分样式表的读取权限。
      }
    }
  }

  function freezeRules(rules: CSSRuleList) {
    for (const rule of rules) {
      if ('style' in rule) freezeUnits((rule as CSSStyleRule).style)
      if ('cssRules' in rule) freezeRules((rule as CSSGroupingRule).cssRules)
    }
  }

  function freezeUnits(style: CSSStyleDeclaration) {
    for (let i = 0; i < style.length; i++) {
      const property = style[i]
      const value = style.getPropertyValue(property)
      const resolved = value.replace(
        /("(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|url\([^)]*\))|(-?\d*\.?\d+)([sld]?v[wh]|vmin|vmax)\b/gi,
        (original, literal: string | undefined, quantity: string, units: string) => {
          if (literal) return original
          const unit = units.toLowerCase()
          const pixels = unit === 'vmin'
            ? Math.min(viewport.width, viewport.height)
            : unit === 'vmax'
              ? Math.max(viewport.width, viewport.height)
              : unit.endsWith('w') ? viewport.width : viewport.height
          return `${Number(quantity) * pixels / 100}px`
        }
      )
      if (resolved !== value) style.setProperty(property, resolved, style.getPropertyPriority(property))
    }
  }

  function schedule() {
    if (!stopped && !timer && updates < updateLimit) timer = target.setTimeout(measure, 80)
  }

  function stop() {
    stopped = true
    target.clearTimeout(timer)
    for (const id of sampleTimers) target.clearTimeout(id)
    resizeObserver?.disconnect()
    mutationObserver?.disconnect()
    doc.removeEventListener('load', schedule, true)
    doc.removeEventListener('DOMContentLoaded', start)
    target.removeEventListener('resize', schedule)
  }

  target.addEventListener('pagehide', stop, { once: true })
  if (doc.readyState === 'loading') doc.addEventListener('DOMContentLoaded', start, { once: true })
  else start()
}
