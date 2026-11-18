import { defineConfig } from 'vitepress'
import type { Plugin } from 'vite'
import { readFile } from 'node:fs/promises'

const apiBase = 'https://api.scienceedu.me'
const publicSettingsUrl = `${apiBase}/api/v1/settings/public`

interface SiteLogoAsset {
  bytes: Buffer
  fileName: string
  mimeType: string
}

const supportedLogoMimeTypes = new Set([
  'image/jpeg',
  'image/png',
  'image/svg+xml',
  'image/webp'
])

function createSiteLogoAsset(bytes: Buffer, mimeType: string): SiteLogoAsset | null {
  const normalizedMimeType = mimeType.toLowerCase().split(';')[0].trim()

  if (!supportedLogoMimeTypes.has(normalizedMimeType) || bytes.length === 0 || bytes.length > 2 * 1024 * 1024) {
    return null
  }

  const embeddedLogo = bytes.toString('base64')
  const wrapper = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1000 1000"><image width="1000" height="1000" preserveAspectRatio="xMidYMid slice" href="data:${normalizedMimeType};base64,${embeddedLogo}"/></svg>`

  return {
    bytes: Buffer.from(wrapper),
    mimeType: 'image/svg+xml',
    fileName: 'site-logo.svg'
  }
}

async function loadSiteLogo(): Promise<SiteLogoAsset | null> {
  try {
    const settingsResponse = await fetch(publicSettingsUrl, {
      signal: AbortSignal.timeout(10_000)
    })

    if (!settingsResponse.ok) {
      throw new Error(`公开设置接口返回 ${settingsResponse.status}`)
    }

    const payload = await settingsResponse.json() as {
      data?: { site_logo?: unknown }
    }
    const siteLogo = payload.data?.site_logo

    if (typeof siteLogo !== 'string' || siteLogo.length === 0) {
      throw new Error('公开设置中未配置站点 Logo')
    }

    const dataUriMatch = siteLogo.match(/^data:([^;,]+);base64,(.+)$/s)
    if (dataUriMatch) {
      const asset = createSiteLogoAsset(Buffer.from(dataUriMatch[2], 'base64'), dataUriMatch[1])
      if (!asset) {
        throw new Error('站点 Logo 的格式或大小不受支持')
      }
      return asset
    }

    const logoUrl = new URL(siteLogo, apiBase)
    if (!['http:', 'https:'].includes(logoUrl.protocol)) {
      throw new Error('站点 Logo URL 协议不受支持')
    }

    const logoResponse = await fetch(logoUrl, {
      signal: AbortSignal.timeout(10_000)
    })
    if (!logoResponse.ok) {
      throw new Error(`站点 Logo 返回 ${logoResponse.status}`)
    }

    const asset = createSiteLogoAsset(
      Buffer.from(await logoResponse.arrayBuffer()),
      logoResponse.headers.get('content-type') ?? ''
    )
    if (!asset) {
      throw new Error('站点 Logo 的格式或大小不受支持')
    }
    return asset
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error)
    console.warn(`[文档站] 无法读取系统站点 Logo，将使用默认图标：${message}`)
    return null
  }
}

function siteLogoPlugin(asset: SiteLogoAsset): Plugin {
  const assetPath = `/${asset.fileName}`

  return {
    name: 'scienceedu-site-logo',
    configureServer(server) {
      server.middlewares.use((request, response, next) => {
        if (request.url?.split('?')[0] !== assetPath) {
          next()
          return
        }

        response.statusCode = 200
        response.setHeader('Content-Type', asset.mimeType)
        response.setHeader('Cache-Control', 'no-cache')
        response.end(asset.bytes)
      })
    },
    generateBundle() {
      this.emitFile({
        type: 'asset',
        fileName: asset.fileName,
        source: asset.bytes
      })
    }
  }
}

const configuredSiteLogo = await loadSiteLogo()
const siteLogoAsset = configuredSiteLogo ?? {
  bytes: await readFile(new URL('../public/favicon.svg', import.meta.url)),
  mimeType: 'image/svg+xml',
  fileName: 'site-logo.svg'
}
const siteLogoPath = `/${siteLogoAsset.fileName}`

export default defineConfig({
  lang: 'zh-CN',
  title: '云桥词元 Docs',
  description: '云桥词元 API 中转服务使用文档：快速开始、接口参考与客户端配置。',
  cleanUrls: true,
  srcExclude: ['README.md'],
  lastUpdated: true,
  sitemap: {
    hostname: 'https://docs.scienceedu.me'
  },
  head: [
    ['link', { rel: 'icon', href: siteLogoPath, type: siteLogoAsset.mimeType }],
    ['meta', { name: 'theme-color', content: '#0f766e' }],
    ['meta', { property: 'og:site_name', content: '云桥词元 Docs' }],
    ['meta', { property: 'og:type', content: 'website' }]
  ],
  themeConfig: {
    logo: siteLogoPath,
    siteTitle: '云桥词元 Docs',
    nav: [
      { text: '首页', link: '/' },
      { text: '快速开始', link: '/guide/quickstart' },
      { text: 'API 参考', link: '/api/endpoints' },
      { text: '客户端', link: '/clients/codex' },
      { text: '返回主站', link: `${apiBase}/home` }
    ],
    sidebar: [
      {
        text: 'API Key',
        items: [
          { text: '快速开始', link: '/guide/quickstart' },
          { text: '创建 API Key', link: '/guide/api-key' },
          { text: 'API 端点', link: '/api/endpoints' },
          { text: '复合 Key', link: '/guide/composite-key' },
          { text: 'Fast 模式', link: '/guide/fast-mode' }
        ]
      },
      {
        text: 'Agents',
        items: [
          { text: 'CC-Switch', link: '/clients/cc-switch' },
          { text: 'Claude Code', link: '/clients/claude-code' },
          { text: 'Codex', link: '/clients/codex' },
          { text: 'Codex++', link: '/clients/codex-plus-plus' },
          { text: 'Hermes', link: '/clients/hermes' },
          { text: 'OpenCode', link: '/clients/opencode' },
          { text: 'WorkBuddy', link: '/clients/workbuddy' }
        ]
      },
      {
        text: 'ChatBot',
        items: [
          { text: 'Cherry Studio', link: '/clients/cherry-studio' },
          { text: 'RikkaHub', link: '/clients/rikkahub' }
        ]
      },
      {
        text: '帮助',
        items: [
          { text: '常见问题', link: '/help/faq' }
        ]
      },
      {
        text: '条款与政策',
        items: [
          { text: '服务条款', link: '/terms/service-terms' },
          { text: '使用政策', link: '/terms/usage-policy' },
          { text: '支持的国家和地区', link: '/terms/supported-countries' }
        ]
      }
    ],
    search: {
      provider: 'local',
      options: {
        translations: {
          button: { buttonText: '搜索文档', buttonAriaLabel: '搜索文档' },
          modal: {
            displayDetails: '显示详细列表',
            resetButtonTitle: '清除查询条件',
            backButtonTitle: '关闭搜索',
            noResultsText: '没有找到相关结果',
            footer: {
              selectText: '选择',
              selectKeyAriaLabel: '回车',
              navigateText: '切换',
              navigateUpKeyAriaLabel: '向上',
              navigateDownKeyAriaLabel: '向下',
              closeText: '关闭',
              closeKeyAriaLabel: 'ESC'
            }
          }
        }
      }
    },
    outline: { level: [2, 3], label: '本页目录' },
    docFooter: { prev: '上一页', next: '下一页' },
    lastUpdated: { text: '最后更新' },
    returnToTopLabel: '返回顶部',
    sidebarMenuLabel: '菜单',
    darkModeSwitchLabel: '深色模式',
    lightModeSwitchTitle: '切换到浅色模式',
    darkModeSwitchTitle: '切换到深色模式',
    footer: {
      message: '云桥词元 API 中转服务使用文档',
      copyright: '请妥善保管 API Key，切勿提交到公开仓库。'
    },
    socialLinks: [
      { icon: 'link', link: apiBase, ariaLabel: '云桥词元主站' }
    ]
  },
  markdown: {
    lineNumbers: true
  },
  vite: {
    plugins: [siteLogoPlugin(siteLogoAsset)]
  }
})
