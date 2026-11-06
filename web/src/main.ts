import { createApp } from 'vue'
import App from './App.vue'
import './style.css'

// Prevent browser-level zoom while keeping ordinary scrolling available.
for (const eventName of ['gesturestart', 'gesturechange', 'gestureend']) {
  document.addEventListener(eventName, (event) => event.preventDefault(), { passive: false, capture: true })
}
document.addEventListener('touchmove', (event) => {
  if (event.touches.length > 1) event.preventDefault()
}, { passive: false, capture: true })
document.addEventListener('wheel', (event) => {
  if (event.ctrlKey || event.metaKey) {
    event.preventDefault()
    event.stopImmediatePropagation()
  }
}, { passive: false, capture: true })
document.addEventListener('keydown', (event) => {
  if ((event.ctrlKey || event.metaKey) && ['+', '-', '=', '0'].includes(event.key)) {
    event.preventDefault()
    event.stopImmediatePropagation()
  }
}, { capture: true })

createApp(App).mount('#app')

