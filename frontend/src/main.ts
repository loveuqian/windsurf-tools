import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import './style.css'
import './utils/theme'

const app = createApp(App)
const pinia = createPinia()

// 把 Vue 内部异常 / unhandled promise / window error 集中渲染到 #app-runtime-error
// 横幅，避免某个子组件抛错时主区被静默清空（用户只看到空白）。
const showRuntimeError = (label: string, err: unknown) => {
  const detail =
    err && typeof err === 'object' && 'stack' in (err as any)
      ? String((err as any).stack)
      : String(err)
  const text = `[${label}] ${detail}`
  // eslint-disable-next-line no-console
  console.error(text)
  let host = document.getElementById('app-runtime-error')
  if (!host) {
    host = document.createElement('pre')
    host.id = 'app-runtime-error'
    host.style.cssText = [
      'position:fixed',
      'left:12px',
      'right:12px',
      'bottom:12px',
      'max-height:40vh',
      'overflow:auto',
      'z-index:99999',
      'padding:12px 14px',
      'border-radius:12px',
      'background:rgba(220,38,38,0.96)',
      'color:#fff',
      'font-family:ui-monospace,Menlo,Consolas,monospace',
      'font-size:12px',
      'line-height:1.45',
      'white-space:pre-wrap',
      'box-shadow:0 12px 32px rgba(0,0,0,0.35)',
    ].join(';')
    host.addEventListener('dblclick', () => host?.remove())
    document.body.appendChild(host)
  }
  host.textContent = `${host.textContent ?? ''}${text}\n\n`
  ;(window as any).__lastError = err
}

app.config.errorHandler = (err, _instance, info) => {
  showRuntimeError(`vue:${info}`, err)
}
window.addEventListener('error', (event) => {
  showRuntimeError('window.error', event.error ?? event.message)
})
window.addEventListener('unhandledrejection', (event) => {
  showRuntimeError('promise', event.reason)
})

app.use(pinia)
app.mount('#app')
