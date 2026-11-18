interface ProviderIDRow {
  id: number
}

interface ProviderListPage {
  items: ProviderIDRow[]
  total: number
  pages?: number
}

type ProviderPageFetcher = (
  page: number,
  pageSize: number,
  filters: Record<string, unknown>
) => Promise<ProviderListPage>

const SELECT_ALL_PAGE_SIZE = 1000

// 按同一筛选快照拉取全部轻量提供商页；结果不完整时拒绝替换现有选择。
export async function fetchAllProviderIds(
  fetchPage: ProviderPageFetcher,
  filters: Record<string, unknown>
): Promise<number[]> {
  const requestFilters = {
    ...filters,
    lite: '1',
    include_scheduler_score: '0'
  }
  const firstPage = await fetchPage(1, SELECT_ALL_PAGE_SIZE, requestFilters)
  const pageCount = Math.max(
    firstPage.pages ?? 0,
    Math.ceil(firstPage.total / SELECT_ALL_PAGE_SIZE)
  )
  const ids = firstPage.items.map(provider => provider.id)

  for (let page = 2; page <= pageCount; page++) {
    const result = await fetchPage(page, SELECT_ALL_PAGE_SIZE, requestFilters)
    ids.push(...result.items.map(provider => provider.id))
  }

  const uniqueIDs = Array.from(new Set(ids))
  if (uniqueIDs.length !== firstPage.total) {
    throw new Error('提供商列表结果不完整')
  }
  return uniqueIDs
}
