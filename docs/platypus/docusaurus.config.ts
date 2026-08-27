import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

const config: Config = {
  title: 'Platypus ',
  tagline: 'A Modern Multi-Session Reverse Shell Manager',
  favicon: 'images/favicon.ico',

  // Set the production url of your site here
  url: 'https://platypus-reverse-shell.vercel.app',
  // Set the /<baseUrl>/ pathname under which your site is served
  // For GitHub pages deployment, it is often '/<projectName>/'
  baseUrl: '/',

  // GitHub pages deployment config.
  // If you aren't using GitHub pages, you don't need these.
  organizationName: 'Platypus', // Usually your GitHub org/user name.
  projectName: 'platypus', // Usually your repo name.

  onBrokenLinks: 'throw',

  // onBrokenMarkdownLinks moved under markdown.hooks in Docusaurus 3.9
  // and the top-level form is removed in v4.
  markdown: {
    hooks: {
      onBrokenMarkdownLinks: 'warn',
    },
  },

  // Even if you don't use internationalization, you can use this field to set
  // useful metadata like html lang. For example, if your site is Chinese, you
  // may want to replace "en" with "zh-Hans".
  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      {
        docs: {
          sidebarPath: './sidebars.ts',
          // Please change this to your repo.
          // Remove this to remove the "edit this page" links.
          editUrl:
            'https://github.com/WangYihang/Platypus/tree/main/docs/platypus/',
        },
        blog: {
          showReadingTime: true,
          feedOptions: {
            type: ['rss', 'atom'],
            xslt: true,
          },
          // Please change this to your repo.
          // Remove this to remove the "edit this page" links.
          editUrl:
            'https://github.com/WangYihang/Platypus/tree/main/docs/platypus/',
          // Useful options to enforce blogging best practices
          onInlineTags: 'warn',
          onInlineAuthors: 'warn',
          onUntruncatedBlogPosts: 'warn',
        },
        theme: {
          customCss: './src/css/custom.css',
        },
      } satisfies Preset.Options,
    ],
  ],

  themeConfig: {
    // Replace with your project's social card
    image: 'images/docusaurus-social-card.jpg',
    navbar: {
      title: 'Platypus',
      logo: {
        alt: 'Platypus Logo',
        src: 'images/logo.svg',
      },
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'tutorialSidebar',
          position: 'left',
          label: 'Documents',
        },
        // /download, /internals, /roadmap, /changelogs and /about were
        // navbar entries for pages that were never written — src/pages/
        // only holds index.tsx. Docusaurus counted all five as broken
        // links on every generated page and failed the build. The three
        // that have a real destination now point at it; Roadmap and
        // About are dropped rather than stubbed, since inventing their
        // content is not a build fix.
        {to: '/docs/dev-guide/overview', label: 'Internals', position: 'left'},
        {
          href: 'https://github.com/WangYihang/Platypus/releases',
          label: 'Download',
          position: 'left',
        },
        {
          href: 'https://github.com/WangYihang/Platypus/blob/main/CHANGELOG.md',
          label: 'Change Logs',
          position: 'left',
        },
        {
          href: 'https://github.com/wangyihang/platypus',
          label: 'GitHub',
          position: 'right',
        },
      ],
    },
    footer: {
      style: 'dark',
      links: [
        {
          title: 'Docs',
          items: [
            // Was /docs/intro, which does not exist: the docs root is
            // docs/index.md and the tutorial is docs/getting-started.md.
            {
              label: 'Getting Started',
              to: '/docs/getting-started',
            },
            {
              label: 'User Guide',
              to: '/docs/user-guide/overview',
            },
            {
              label: 'Dev Guide',
              to: '/docs/dev-guide/overview',
            },
          ],
        },
        {
          title: 'Community',
          items: [
            // These were the scaffold's own channels — Docusaurus's
            // Stack Overflow tag, Discord and Twitter — presented as if
            // they were this project's. Point at the places that
            // actually take Platypus questions instead.
            {
              label: 'Issues',
              href: 'https://github.com/WangYihang/Platypus/issues',
            },
            {
              label: 'Security Policy',
              href: 'https://github.com/WangYihang/Platypus/blob/main/SECURITY.md',
            },
          ],
        },
        {
          title: 'More',
          items: [
            {
              label: 'Blog',
              to: '/blog',
            },
            {
              // Was facebook/docusaurus.
              label: 'GitHub',
              href: 'https://github.com/WangYihang/Platypus',
            },
            {
              label: 'Releases',
              href: 'https://github.com/WangYihang/Platypus/releases',
            },
          ],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} Platypus. Built with Docusaurus.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
