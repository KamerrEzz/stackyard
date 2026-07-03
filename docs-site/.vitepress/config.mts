import { defineConfig } from "vitepress";

export default defineConfig({
  title: "Stackyard",
  description:
    "A local Docker-based database environment manager and multi-engine DB client.",
  base: "/stackyard/",
  cleanUrls: true,
  themeConfig: {
    nav: [
      { text: "Home", link: "/" },
      { text: "Getting Started", link: "/getting-started" },
      {
        text: "Features",
        items: [
          { text: "Environment Manager", link: "/features/environment-manager" },
          { text: "DB Client", link: "/features/db-client" },
          { text: "Schema Diagram", link: "/features/schema-diagram" },
          { text: "Import / Export", link: "/features/import-export" },
          { text: "Migrations", link: "/features/migrations" },
        ],
      },
      { text: "About", link: "/about" },
      { text: "Contributing with AI", link: "/contributing-with-ai" },
    ],
    sidebar: [
      {
        text: "Introduction",
        items: [
          { text: "Home", link: "/" },
          { text: "Getting Started", link: "/getting-started" },
        ],
      },
      {
        text: "Features",
        items: [
          { text: "Environment Manager", link: "/features/environment-manager" },
          { text: "DB Client", link: "/features/db-client" },
          { text: "Schema Diagram", link: "/features/schema-diagram" },
          { text: "Import / Export", link: "/features/import-export" },
          { text: "Migrations", link: "/features/migrations" },
        ],
      },
      {
        text: "Project",
        items: [
          { text: "About", link: "/about" },
          { text: "Contributing with AI", link: "/contributing-with-ai" },
        ],
      },
    ],
    socialLinks: [
      { icon: "github", link: "https://github.com/KamerrEzz/stackyard" },
    ],
    footer: {
      message: "Released under the PolyForm Noncommercial License 1.0.0.",
      copyright: "Copyright © 2026 Kamerr Ezz",
    },
    search: {
      provider: "local",
    },
  },
});
