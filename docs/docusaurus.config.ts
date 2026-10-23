import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

const repository = 'https://github.com/tosunlab/hisar';

const config: Config = {
  title: 'Hisar',
  tagline: 'End-to-end encrypted password, SSH-key and secrets manager for developers',
  favicon: 'img/favicon.svg',

  future: {
    v4: true,
  },

  url: 'https://docs.tryhisar.com',
  baseUrl: '/',
  // Every page is <path>/index.html and is linked with the slash, so nginx serves it without rewrites or redirects.
  trailingSlash: true,

  organizationName: 'tosunlab',
  projectName: 'hisar',

  onBrokenLinks: 'throw',
  markdown: {
    format: 'detect',
    hooks: {
      onBrokenMarkdownLinks: 'throw',
    },
  },

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      {
        docs: {
          routeBasePath: '/',
          sidebarPath: './sidebars.ts',
          editUrl: `${repository}/edit/main/docs/`,
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      } satisfies Preset.Options,
    ],
  ],

  themeConfig: {
    image: 'img/social-card.png',
    colorMode: {
      respectPrefersColorScheme: true,
    },
    navbar: {
      title: 'Hisar',
      logo: {
        alt: 'Hisar',
        src: 'img/logo.svg',
      },
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'docs',
          position: 'left',
          label: 'Docs',
        },
        {
          href: 'https://tryhisar.com',
          label: 'Website',
          position: 'right',
        },
        {
          href: repository,
          label: 'GitHub',
          position: 'right',
        },
      ],
    },
    footer: {
      style: 'dark',
      links: [
        {
          title: 'Hisar',
          items: [
            {label: 'Website', href: 'https://tryhisar.com'},
            {label: 'Download', href: 'https://tryhisar.com/download'},
          ],
        },
        {
          title: 'Source',
          items: [
            {label: 'Hisar on GitHub', href: repository},
            {label: 'These docs on GitHub', href: `${repository}/tree/main/docs`},
          ],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} Hisar.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
      additionalLanguages: ['bash', 'json', 'yaml', 'go'],
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
