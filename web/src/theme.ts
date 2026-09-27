// Файл: web/src/theme.ts
// Тёмная тема: класс dark на <html>, выбор в localStorage (может быть недоступен — тогда не запоминаем).
const KEY = 'pylesos-theme'

function readTheme(): string | null {
  try {
    return localStorage.getItem(KEY)
  } catch {
    return null
  }
}

export function applyStoredTheme(): void {
  document.documentElement.classList.toggle('dark', readTheme() === 'dark')
}

export function toggleTheme(): void {
  const dark = !document.documentElement.classList.contains('dark')
  document.documentElement.classList.toggle('dark', dark)
  try {
    localStorage.setItem(KEY, dark ? 'dark' : 'light')
  } catch {
    // хранилище недоступно — тема действует до закрытия окна
  }
}
