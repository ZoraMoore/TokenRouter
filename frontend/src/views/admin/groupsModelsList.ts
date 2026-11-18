export interface ModelsListConfig {
  enabled: boolean
  models: string[]
}

export interface ModelsListItem {
  id: string
  selected: boolean
}

export interface ModelsListState {
  enabled: boolean
  savedModels: string[]
  candidateModels: string[]
  items: ModelsListItem[]
}

export const createModelsListState = (
  config?: Partial<ModelsListConfig> | null,
): ModelsListState => ({
  enabled: config?.enabled ?? false,
  savedModels: normalizeModels(config?.models ?? []),
  candidateModels: [],
  items: [],
})

export const hydrateModelsListState = (
  config: Partial<ModelsListConfig> | null | undefined,
  candidates: string[],
): ModelsListState => {
  const state = createModelsListState(config)
  setModelsListCandidates(state, candidates)
  return state
}

export const setModelsListCandidates = (
  state: ModelsListState,
  candidates: string[],
) => {
  const normalizedCandidates = normalizeModels(candidates)
  state.candidateModels = normalizedCandidates
  const currentSelected = new Set(
    state.items.filter(item => item.selected).map(item => item.id),
  )
  const currentKnown = new Set(state.items.map(item => item.id))
  const savedSelected = new Set(state.savedModels)
  const hasExistingItems = state.items.length > 0
  const selectionOrder = normalizeModels([
    ...state.items.map(item => item.id),
    ...state.savedModels,
    ...normalizedCandidates,
  ])

  state.items = selectionOrder.map(id => {
    const selected = hasExistingItems
      ? currentSelected.has(id)
      : state.savedModels.length > 0
        ? savedSelected.has(id)
        : normalizedCandidates.includes(id)

    return {
      id,
      selected: selected && (currentKnown.has(id) || savedSelected.has(id) || state.savedModels.length === 0),
    }
  })
}

export const toggleModelsListItem = (state: ModelsListState, modelID: string) => {
  const item = state.items.find(item => item.id === modelID)
  if (item) {
    item.selected = !item.selected
  }
}

export const selectAllModelsListItems = (state: ModelsListState) => {
  state.items.forEach(item => {
    item.selected = true
  })
}

export const invertModelsListSelection = (state: ModelsListState) => {
  state.items.forEach(item => {
    item.selected = !item.selected
  })
}

export const moveModelsListItem = (
  state: ModelsListState,
  fromIndex: number,
  toIndex: number,
) => {
  if (
    fromIndex === toIndex ||
    fromIndex < 0 ||
    toIndex < 0 ||
    fromIndex >= state.items.length ||
    toIndex >= state.items.length
  ) {
    return
  }
  const [item] = state.items.splice(fromIndex, 1)
  state.items.splice(toIndex, 0, item)
}

export const buildModelsListConfig = (state: ModelsListState): ModelsListConfig => ({
  enabled: state.enabled,
  models: state.items.length > 0
    ? state.items.filter(item => item.selected).map(item => item.id)
    : [...state.savedModels],
})

export const getAvailabilityProbeCandidateModels = (state: ModelsListState): string[] => {
  // 后端候选已应用分组映射和白名单，历史自定义列表不能重新加入不可请求的模型。
  if (state.enabled) {
    const available = new Set(state.candidateModels)
    const selected = state.items.length > 0
      ? state.items.filter(item => item.selected).map(item => item.id)
      : state.savedModels
    return normalizeModels(selected).filter(model => available.has(model))
  }

  // 自定义列表关闭时，候选列表取本次后端返回值。
  return [...state.candidateModels]
}

const normalizeModels = (models: string[]): string[] => {
  const seen = new Set<string>()
  const out: string[] = []
  for (const raw of models) {
    const model = raw.trim()
    if (!model || seen.has(model)) {
      continue
    }
    seen.add(model)
    out.push(model)
  }
  return out
}
