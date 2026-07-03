# Primeros pasos

Stackyard es una aplicación de escritorio hecha con [Wails](https://wails.io):
un backend en Go junto a un frontend en React/TypeScript, empaquetados
como un único binario nativo. Ejecutarla desde el código fuente requiere
las mismas herramientas que cualquier app de Wails, más Docker para los
entornos que gestiona.

## Requisitos previos

- **Go** 1.25 o superior
- **Node.js** y **pnpm** (el gestor de paquetes del frontend)
- **Wails CLI** — `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- **Docker Desktop** (o un Docker Engine local) — en ejecución, antes de
  iniciar cualquier entorno desde Stackyard

## Ejecutar en modo desarrollo

```sh
git clone https://github.com/KamerrEzz/stackyard.git
cd stackyard
wails dev
```

`wails dev` compila el backend en Go, inicia un servidor de desarrollo de
Vite para el frontend, y abre la ventana de la app con recarga en caliente
habilitada. Para quienes prefieran desarrollar desde un navegador e
inspeccionar directamente los métodos de Go expuestos, también hay un
servidor de desarrollo disponible en `http://localhost:34115`.

## Compilar un binario de producción

```sh
wails build
```

Esto genera un binario nativo redistribuible (ver `wails.json` para el
nombre exacto de salida y la plataforma de destino).

## Usando la app

1. Abrir **Environments**, nombrar un perfil, elegir uno o más motores
   (PostgreSQL, MySQL, MongoDB, Redis), y hacer clic en **Create & Start**.
2. Copiar la cadena de conexión generada, o abrir **DB Client** y pegarla
   directamente — Stackyard interpreta la URL y completa el formulario de
   conexión.
3. Navegar el árbol del esquema, editar datos en la grilla estilo hoja de
   cálculo, escribir y guardar consultas, o generar un diagrama de esquema —
   todo desde la misma ventana, sin necesidad de un cliente GUI separado.

Ver la sección de [Funcionalidades](/es/features/environment-manager) para
un recorrido completo de cada módulo.
