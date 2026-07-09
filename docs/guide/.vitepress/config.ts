import { defineConfig } from 'vitepress'
import { fileURLToPath } from 'url'
import { dirname, resolve } from 'path'

const __dirname = dirname(fileURLToPath(import.meta.url))

const sidebarZh = [
  {
    text: '软件介绍',
    items: [
      { text: '关于 uniTerm', link: '/zh/introduction' },
      { text: '支持协议', link: '/zh/protocols' }
    ]
  },
  {
    text: '快速开始',
    items: [
      { text: '安装与首次连接', link: '/zh/getting-started' },
      { text: '开始页', link: '/zh/start-page' }
    ]
  },
  {
    text: '连接协议',
    items: [
      { text: '远程终端', link: '/zh/connections/remote-terminal' },
      { text: '本地和串口连接', link: '/zh/connections/local' },
      { text: '文件传输', link: '/zh/connections/file-transfer' },
      { text: '远程桌面', link: '/zh/connections/remote-desktop' },
      { text: '服务器监控', link: '/zh/connections/server-monitor' },
      { text: '数据库', link: '/zh/connections/databases' }
    ]
  },
  {
    text: '功能指南',
    items: [
      { text: 'AI 助理', link: '/zh/features/ai-assistant' },
      { text: '标签和工作区', link: '/zh/features/workspace' },
      { text: '云同步', link: '/zh/features/sync' },
      { text: '智能提示', link: '/zh/features/smart-suggest' },
      { text: '个性化', link: '/zh/features/personalization' }
    ]
  },
  {
    text: '常见问题',
    items: [
      { text: '常见问题与排错', link: '/zh/faq' },
      { text: '问题反馈与社区', link: '/zh/community' }
    ]
  }
]

const sidebarEn = [
  {
    text: 'Introduction',
    items: [
      { text: 'About uniTerm', link: '/en/introduction' },
      { text: 'Supported Protocols', link: '/en/protocols' }
    ]
  },
  {
    text: 'Getting Started',
    items: [
      { text: 'Installation & First Connection', link: '/en/getting-started' }
    ]
  },
  {
    text: 'Connections',
    items: [
      { text: 'Remote Terminal', link: '/en/connections/remote-terminal' },
      { text: 'Local', link: '/en/connections/local' },
      { text: 'File Transfer', link: '/en/connections/file-transfer' },
      { text: 'Remote Desktop', link: '/en/connections/remote-desktop' },
      { text: 'Server Monitor', link: '/en/connections/server-monitor' },
      { text: 'Databases', link: '/en/connections/databases' }
    ]
  },
  {
    text: 'Features',
    items: [
      { text: 'AI Assistant', link: '/en/features/ai-assistant' },
      { text: 'Workspace', link: '/en/features/workspace' },
      { text: 'Cloud Sync', link: '/en/features/sync' },
      { text: 'Smart Suggestions', link: '/en/features/smart-suggest' },
      { text: 'Personalization', link: '/en/features/personalization' }
    ]
  },
  {
    text: 'FAQ',
    items: [
      { text: 'FAQ & Troubleshooting', link: '/en/faq' },
      { text: 'Feedback & Community', link: '/en/community' }
    ]
  }
]

export default defineConfig({
  base: '/guide/',
  title: 'uniTerm User Guide',
  description: 'uniTerm — All-in-One Terminal User Manual',
  head: [
    ['link', { rel: 'icon', href: '/imgs/appicon.png' }],
    ['meta', { name: 'baidu-site-verification', content: 'codeva-xKnncJaXe8' }],
    ['script', {}, `var _hmt = _hmt || [];
(function() {
  var hm = document.createElement("script");
  hm.src = "https://hm.baidu.com/hm.js?c199b0d963bc266f256b10c20d2045bb";
  var s = document.getElementsByTagName("script")[0];
  s.parentNode.insertBefore(hm, s);
})();`]
  ],
  locales: {
    zh: {
      label: '简体中文',
      lang: 'zh-CN',
      link: '/zh/',
      title: 'uniTerm 用户手册',
      description: 'uniTerm 全能终端用户手册',
      themeConfig: {
        nav: [
          { text: '← uniTerm 首页', link: 'https://uniterm.net' },
          { text: '软件介绍', link: '/zh/introduction' },
          { text: '快速开始', link: '/zh/getting-started' },
          { text: '连接协议', link: '/zh/connections/remote-terminal' },
          { text: '功能指南', link: '/zh/features/ai-assistant' },
          { text: '常见问题', link: '/zh/faq' }
        ],
        sidebar: sidebarZh,
        outline: { label: '本页内容' },
        docFooter: { prev: '上一页', next: '下一页' },
        darkModeSwitchLabel: '主题',
        sidebarMenuLabel: '菜单',
        returnToTopLabel: '回到顶部',
        lastUpdated: { text: '最后更新' }
      }
    },
    en: {
      label: 'English',
      lang: 'en-US',
      link: '/en/',
      title: 'uniTerm User Guide',
      description: 'uniTerm All-in-One Terminal User Manual',
      themeConfig: {
        nav: [
          { text: '← uniTerm Home', link: 'https://uniterm.net' },
          { text: 'Introduction', link: '/en/introduction' },
          { text: 'Getting Started', link: '/en/getting-started' },
          { text: 'Connections', link: '/en/connections/remote-terminal' },
          { text: 'Features', link: '/en/features/ai-assistant' },
          { text: 'FAQ', link: '/en/faq' }
        ],
        sidebar: sidebarEn
      }
    }
  },
  themeConfig: {
    search: {
      provider: 'local'
    }
  },
  vite: {
    resolve: {
      alias: {
        '/imgs': resolve(__dirname, '../../imgs')
      }
    }
  }
})
