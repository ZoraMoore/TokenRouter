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
  modified: boolean
  selectionModified: boolean
  hasSavedSelection: boolean
  savedModels: string[]
  candidateModels: string[]
  items: ModelsListItem[]
}

export const createModelsListState = (
  config?: Partial<ModelsListConfig> | null,
): ModelsListState => ({
  enabled: config?.enabled ?? false,
  modified: false,
  selectionModified: false,
  hasSavedSelection: Boolean(config?.enabled || config?.models?.length),
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

// 候选合并保持手选、保存顺序和暂时不可请求的型号。
export const setModelsListCandidates = (state: ModelsListState, candidates: string[]) => {
  const normalizedCandidates = normalizeModels(candidates)
  const available = new Set(normalizedCandidates)
  const previouslySelected = new Map(state.items.map(item => [item.id, item.selected]))
  const savedSelected = new Set(state.savedModels)
  const selectDefaults = state.items.length === 0 && !state.hasSavedSelection
  state.candidateModels = normalizedCandidates
  const order = new Set([...previouslySelected.keys(), ...savedSelected, ...available])

  state.items = Array.from(order, id => ({
    id,
    selected: previouslySelected.get(id) ?? (savedSelected.has(id) || (selectDefaults && available.has(id))),
  }))
}

// 初始化候选可以预选型号。表单只记录用户对开关或清单的操作。
export const setModelsListEnabled = (state: ModelsListState, enabled: boolean) => {
  if (state.enabled === enabled) return
  state.enabled = enabled
  state.modified = true
}

export const toggleModelsListItem = (state: ModelsListState, modelID: string, value?: boolean) => {
  const item = state.items.find(item => item.id === modelID)
  if (!item) return
  const selected = value ?? !item.selected
  if (item.selected === selected) return
  item.selected = selected
  state.modified = true
  state.selectionModified = true
}

export const selectAllModelsListItems = (state: ModelsListState) => {
  state.items.forEach(item => {
    item.selected = true
  })
  state.modified = true
  state.selectionModified = true
}

export const invertModelsListSelection = (state: ModelsListState) => {
  state.items.forEach(item => {
    item.selected = !item.selected
  })
  state.modified = true
  state.selectionModified = true
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
  const items = [...state.items]
  const [item] = items.splice(fromIndex, 1)
  items.splice(toIndex, 0, item)
  state.items = items
  state.modified = true
  state.selectionModified = true
}

export const buildModelsListConfig = (state: ModelsListState): ModelsListConfig => ({
  enabled: state.enabled,
  models: state.items.length > 0
    ? state.items.filter(item => item.selected).map(item => item.id)
    : [...state.savedModels],
})

// 新建关闭时发送空清单。编辑时省略未触碰的配置，存储中已有的顺序由服务端保管。
export const buildCreateModelsListConfig = (state: ModelsListState): ModelsListConfig =>
  state.enabled ? buildModelsListConfig(state) : { enabled: false, models: [] }

export const buildUpdateModelsListConfig = (state: ModelsListState): ModelsListConfig | undefined => {
  if (!state.modified) return undefined
  if (!state.enabled && !state.selectionModified) {
    return { enabled: false, models: [...state.savedModels] }
  }
  return buildModelsListConfig(state)
}

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
