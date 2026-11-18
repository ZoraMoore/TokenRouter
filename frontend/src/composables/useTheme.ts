import { ref } from 'vue'

// 主题模式：light/dark 为显式选择，system 跟随操作系统配色。
export type ThemeMode = 'light' | 'dark' | 'system'

const themeStorageKey = 'theme'
const themeSwitchingClass = 'theme-switching'
const systemDarkQuery = '(prefers-color-scheme: dark)'
const isDark = ref(document.documentElement.classList.contains('dark')) // check-ui-allow: 响应式主题原语本身
const themeMode = ref<ThemeMode>('system')
let themeSwitchFrame: number | undefined
let systemThemeMedia: MediaQueryList | undefined

// 本地只保存显式选择，没有记录时视为跟随系统。
function readSavedThemeMode(): ThemeMode {
  const savedTheme = localStorage.getItem(themeStorageKey)
  return savedTheme === 'dark' || savedTheme === 'light' ? savedTheme : 'system'
}

function systemPrefersDark(): boolean {
  return typeof window.matchMedia === 'function' && window.matchMedia(systemDarkQuery).matches
}

function resolveIsDark(mode: ThemeMode): boolean {
  return mode === 'system' ? systemPrefersDark() : mode === 'dark'
}

function applyTheme(nextIsDark: boolean) {
  isDark.value = nextIsDark
  document.documentElement.classList.toggle('dark', nextIsDark)
}

// 跟随系统时，操作系统切换配色后页面同步切换；监听只注册一次。
function watchSystemTheme() {
  if (systemThemeMedia || typeof window.matchMedia !== 'function') return
  systemThemeMedia = window.matchMedia(systemDarkQuery)
  systemThemeMedia.addEventListener('change', (event) => {
    if (themeMode.value !== 'system' || event.matches === isDark.value) return
    suspendThemeTransitions()
    applyTheme(event.matches)
  })
}

export function initTheme() {
  themeMode.value = readSavedThemeMode()
  applyTheme(resolveIsDark(themeMode.value))
  watchSystemTheme()
}

function suspendThemeTransitions() {
  const root = document.documentElement
  root.classList.add(themeSwitchingClass)

  if (themeSwitchFrame !== undefined) {
    window.cancelAnimationFrame(themeSwitchFrame)
  }

  // 保留两个渲染帧，确保 Vue 更新和浏览器绘制期间都不会触发零散的颜色过渡。
  themeSwitchFrame = window.requestAnimationFrame(() => {
    themeSwitchFrame = window.requestAnimationFrame(() => {
      root.classList.remove(themeSwitchingClass)
      themeSwitchFrame = undefined
    })
  })
}

export function setThemeMode(mode: ThemeMode) {
  themeMode.value = mode
  if (mode === 'system') {
    localStorage.removeItem(themeStorageKey)
  } else {
    localStorage.setItem(themeStorageKey, mode)
  }

  const nextIsDark = resolveIsDark(mode)
  if (nextIsDark === isDark.value) return
  suspendThemeTransitions()
  // 整体直接切换主题，使各组件同时更新颜色。
  applyTheme(nextIsDark)
}

export function setTheme(nextIsDark: boolean) {
  setThemeMode(nextIsDark ? 'dark' : 'light')
}

export function useTheme() {
  return {
    isDark,
    themeMode,
    setTheme,
    setThemeMode,
    toggleTheme: () => setTheme(!isDark.value),
  }
}
