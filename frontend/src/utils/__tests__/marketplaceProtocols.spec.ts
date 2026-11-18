import { describe, expect, it } from 'vitest'
import fixture from '@/__tests__/fixtures/protocol-catalog.json'
import { MARKETPLACE_PROTOCOLS, marketplaceProtocols } from '../marketplaceProtocols'
import { resolveProviderBrandKey } from '../providerBrand'
import zhMisc from '@/i18n/locales/zh/misc'
import enMisc from '@/i18n/locales/en/misc'

describe('marketplaceProtocols', () => {
  it('端点和顺序与后端协议目录的客户端协议一致', () => {
    const clientProtocols = fixture.protocols
      .filter(protocol => !protocol.upstream_only)
      .map(protocol => ({ id: protocol.id, endpoint: protocol.endpoint }))
    expect(MARKETPLACE_PROTOCOLS.map(({ id, endpoint }) => ({ id, endpoint }))).toEqual(clientProtocols)
  })

  it('每个协议都有中英文短名，品牌能解析出图标', () => {
    for (const protocol of MARKETPLACE_PROTOCOLS) {
      expect(zhMisc.marketplace.protocolNames).toHaveProperty(protocol.id)
      expect(enMisc.marketplace.protocolNames).toHaveProperty(protocol.id)
      expect(resolveProviderBrandKey(protocol.brand)).toBe(protocol.brand)
    }
  })

  it('按目录顺序返回，跳过上游专用和未知的协议', () => {
    expect(marketplaceProtocols(['openai_chat_completions', 'qoder_chat', 'anthropic_messages']).map(protocol => protocol.id))
      .toEqual(['anthropic_messages', 'openai_chat_completions'])
    expect(marketplaceProtocols(undefined)).toEqual([])
  })
})
