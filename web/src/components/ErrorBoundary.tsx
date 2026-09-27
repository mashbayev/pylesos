// Файл: web/src/components/ErrorBoundary.tsx
// Белый экран запрещён: при сбое рендера — сообщение (один из двух текстов, живущих во фронте).
import { Component, type ReactNode } from 'react'

export class ErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false }

  static getDerivedStateFromError() {
    return { failed: true }
  }

  render() {
    if (this.state.failed) {
      return <p className="p-6">Окно программы дало сбой. Закройте его и запустите Пылесос снова.</p>
    }
    return this.props.children
  }
}
