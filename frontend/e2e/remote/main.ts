import { createApp, h, ref } from 'vue'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import VNCTabContent from '../../src/components/VNCTabContent.vue'
import SPICETabContent from '../../src/components/SPICETabContent.vue'
const type = new URLSearchParams(location.search).get('type') || 'vnc'
const config = { host: type === 'vnc' ? '127.0.0.1' : 'tcp://127.0.0.1', port: type === 'vnc' ? 5909 : 33925, password: '', vncShared: true }
const visible = ref(true)
window.addEventListener('e2e:toggle-tab', () => { visible.value = !visible.value })
createApp({ render: () => visible.value ? h(type === 'vnc' ? VNCTabContent : SPICETabContent, { panelId: 'protocol-e2e', sessionId: 'desktop-session-must-not-be-adopted', config: config as any }) : h('p', 'Another tab') }).use(ElementPlus).mount('#app')
document.head.insertAdjacentHTML('beforeend', '<style>html,body,#app{margin:0;width:100%;height:100%;background:#111;color:#ddd} :root{--text-primary:#eee;--text-muted:#aaa;--bg-base:#111;--bg-elevated:#333;--success:#0a0;--error:#f55}</style>')
