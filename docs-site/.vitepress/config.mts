import { defineConfig } from "vitepress";

export default defineConfig({
  title: "Stackyard",
  description:
    "A local Docker-based database environment manager and multi-engine DB client.",
  base: "/stackyard/",
  cleanUrls: true,
  locales: {
    root: {
      label: "English",
      lang: "en",
      description:
        "A local Docker-based database environment manager and multi-engine DB client.",
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
        footer: {
          message: "Released under the PolyForm Noncommercial License 1.0.0.",
          copyright: "Copyright © 2026 Kamerr Ezz",
        },
      },
    },
    es: {
      label: "Español",
      lang: "es",
      link: "/es/",
      description:
        "Un gestor de entornos de bases de datos local basado en Docker y cliente de BD multi-motor.",
      themeConfig: {
        nav: [
          { text: "Inicio", link: "/es/" },
          { text: "Primeros pasos", link: "/es/getting-started" },
          {
            text: "Funcionalidades",
            items: [
              { text: "Gestor de entornos", link: "/es/features/environment-manager" },
              { text: "Cliente de BD", link: "/es/features/db-client" },
              { text: "Diagrama de esquema", link: "/es/features/schema-diagram" },
              { text: "Importar / Exportar", link: "/es/features/import-export" },
              { text: "Migraciones", link: "/es/features/migrations" },
            ],
          },
          { text: "Acerca de", link: "/es/about" },
          { text: "Contribuir con IA", link: "/es/contributing-with-ai" },
        ],
        sidebar: [
          {
            text: "Introducción",
            items: [
              { text: "Inicio", link: "/es/" },
              { text: "Primeros pasos", link: "/es/getting-started" },
            ],
          },
          {
            text: "Funcionalidades",
            items: [
              { text: "Gestor de entornos", link: "/es/features/environment-manager" },
              { text: "Cliente de BD", link: "/es/features/db-client" },
              { text: "Diagrama de esquema", link: "/es/features/schema-diagram" },
              { text: "Importar / Exportar", link: "/es/features/import-export" },
              { text: "Migraciones", link: "/es/features/migrations" },
            ],
          },
          {
            text: "Proyecto",
            items: [
              { text: "Acerca de", link: "/es/about" },
              { text: "Contribuir con IA", link: "/es/contributing-with-ai" },
            ],
          },
        ],
        footer: {
          message: "Distribuido bajo la PolyForm Noncommercial License 1.0.0.",
          copyright: "Copyright © 2026 Kamerr Ezz",
        },
      },
    },
  },
  themeConfig: {
    socialLinks: [
      { icon: "github", link: "https://github.com/KamerrEzz/stackyard" },
    ],
    search: {
      provider: "local",
    },
  },
});
