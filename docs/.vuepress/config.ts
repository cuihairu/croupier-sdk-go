import { defineUserConfig } from 'vuepress'
import { defaultTheme } from '@vuepress/theme-default'
import { viteBundler } from '@vuepress/bundler-vite'
import { searchPlugin } from '@vuepress/plugin-search'

export default defineUserConfig({
  lang: 'zh-CN',
  title: 'Croupier Go SDK',
  description: '高性能 Go SDK，用于 Croupier 游戏函数注册与执行系统',
  head: [
    ['meta', { name: 'viewport', content: 'width=device-width,initial-scale=1' }],
    ['meta', { name: 'keywords', content: 'croupier,go,sdk,gRPC,游戏开发' }],
    ['meta', { name: 'theme-color', content: '#00ADD8' }],
    ['meta', { property: 'og:type', content: 'website' }],
    ['meta', { property: 'og:locale', content: 'zh-CN' }],
    ['meta', { property: 'og:title', content: 'Croupier Go SDK' }],
    ['meta', { property: 'og:site_name', content: 'Croupier Go SDK' }],
  ],
  base: '/croupier-sdk-go/',
  bundler: viteBundler({
    viteOptions: {
      build: {
        chunkSizeWarningLimit: 1200,
      },
    },
  }),
  theme: defaultTheme({
    repo: 'cuihairu/croupier-sdk-go',
    repoLabel: 'GitHub',
    editLinkText: '在 GitHub 上编辑此页',
    lastUpdated: true,
    lastUpdatedText: '最后更新',
    contributors: false,
    logo: '/logo.png',

    navbar: [
      {
        text: '指南',
        link: '/guide/',
      },
      {
        text: 'API 参考',
        link: '/api/',
      },
      {
        text: '示例',
        link: '/examples/',
      },
      {
        text: 'Croupier 主项目',
        link: 'https://cuihairu.github.io/croupier/',
      },
      {
        text: '其他 SDK',
        children: [
          {
            text: 'C++ SDK',
            link: 'https://cuihairu.github.io/croupier-sdk-cpp/',
          },
          {
            text: 'Java SDK',
            link: 'https://cuihairu.github.io/croupier-sdk-java/',
          },
          {
            text: 'JavaScript SDK',
            link: 'https://cuihairu.github.io/croupier-sdk-js/',
          },
          {
            text: 'Python SDK',
            link: 'https://cuihairu.github.io/croupier-sdk-python/',
          },
        ],
      },
    ],

    sidebar: {
      '/guide/': [
        {
          text: '入门指南',
          collapsable: false,
          children: [
            '/guide/README.md',
            '/guide/installation.md',
            '/guide/quick-start.md',
          ],
        },
        {
          text: '核心概念',
          children: [
            '/guide/architecture.md',
            '/guide/function-descriptor.md',
            '/guide/build-modes.md',
          ],
        },
      ],

      '/api/': [
        {
          text: 'API 参考',
          collapsable: false,
          children: [
            '/api/README.md',
            '/api/client.md',
            '/api/config.md',
          ],
        },
      ],

      '/examples/': [
        {
          text: '使用示例',
          collapsable: false,
          children: [
            '/examples/README.md',
            '/examples/basic.md',
            '/examples/comprehensive.md',
          ],
        },
      ],

      '/': [
        {
          text: '概览',
          collapsible: false,
          children: [
            '/README.md',
          ],
        },
        {
          text: '指南',
          collapsible: false,
          children: [
            '/guide/README.md',
            '/guide/installation.md',
          ],
        },
        {
          text: 'API',
          collapsible: false,
          children: [
            '/api/README.md',
          ],
        },
      ],
    },

    themePlugins: {
      git: true,
      gitContributors: false,
      prismHighlighter: true,
      nprogress: true,
      backToTop: true,
    },
  }),

  plugins: [
    searchPlugin({
      locales: {
        '/': {
          placeholder: '搜索文档',
          hotKeys: ['k', '/'],
        },
      },
      hotSearchOnlyFocus: true,
      maxSuggestions: 10,
    }),
  ],
})
