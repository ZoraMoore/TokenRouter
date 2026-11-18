import { describe, expect, it } from 'vitest'
import {
  buildCcSwitchImportDeeplink,
  buildCcSwitchUsageScript
} from '@/utils/ccswitchImport'

function paramsFromDeeplink(deeplink: string): URLSearchParams {
  const query = deeplink.split('?')[1] || ''
  return new URLSearchParams(query)
}

function decodeBase64Utf8(value: string): string {
  return new TextDecoder().decode(Uint8Array.from(atob(value), char => char.charCodeAt(0)))
}

describe('ccswitchImport utils', () => {
  it.each([
    'https://api.example.com',
    'https://api.example.com/',
    'https://api.example.com/v1',
    'https://api.example.com/v1/',
    'https://api.example.com/api/v1'
  ])('pins the usage script to /v1/usage for base URL %s', (baseUrl) => {
    const usageScript = buildCcSwitchUsageScript(baseUrl, 'USD')

    expect(usageScript).toContain('url: "https://api.example.com/v1/usage"')
    expect(usageScript).not.toContain('{{baseUrl}}')
    expect(usageScript).toContain('"Authorization": "Bearer {{apiKey}}"')
  })

  const baseInput = {
    baseUrl: 'https://api.example.com',
    providerName: 'TokenRouter',
    apiKey: 'sk-test',
    model: 'configured-model',
    usageScript: 'return true'
  }

  it('adds the Codex model parameter for OpenAI imports', () => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        clientType: 'codex'
      })
    )

    expect(params.get('resource')).toBe('provider')
    expect(params.get('app')).toBe('codex')
    expect(params.get('endpoint')).toBe(`${baseInput.baseUrl}/v1`)
    expect(params.get('model')).toBe('configured-model')
    expect(decodeBase64Utf8(params.get('usageScript') || '')).toBe(baseInput.usageScript)
  })

  it.each([
    'https://api.example.com',
    'https://api.example.com/',
    'https://api.example.com/v1',
    'https://api.example.com/v1/'
  ])('imports Grok Build with one /v1 suffix for base URL %s', (baseUrl) => {
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        baseUrl,
        clientType: 'grok'
      })
    )

    expect(params.get('app')).toBe('grokbuild')
    expect(params.get('endpoint')).toBe('https://api.example.com/v1')
    expect(params.get('model')).toBe('configured-model')
  })

  it.each(['claude', 'gemini'] as const)('imports %s with the selected model and common endpoint', (clientType) => {
    const params = paramsFromDeeplink(buildCcSwitchImportDeeplink({ ...baseInput, clientType }))
    expect(params.get('app')).toBe(clientType)
    expect(params.get('endpoint')).toBe(baseInput.baseUrl)
    expect(params.get('model')).toBe('configured-model')
  })

  it('uses the Anthropic root endpoint and preserves UTF-8 usage scripts', () => {
    const usageScript = 'return "余额"'
    const params = paramsFromDeeplink(
      buildCcSwitchImportDeeplink({
        ...baseInput,
        baseUrl: 'https://api.example.com/v1',
        clientType: 'claude',
        usageScript
      })
    )

    expect(params.get('endpoint')).toBe('https://api.example.com')
    expect(decodeBase64Utf8(params.get('usageScript') || '')).toBe(usageScript)
  })
})
