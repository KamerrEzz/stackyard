# Contribuir con IA

Para quienes usen un asistente de IA para programar y quieran extender o
modificar Stackyard, este proyecto ya tiene el contexto que esa
herramienta necesita — cargar estos archivos, en este orden, antes de
hacer cambios:

1. **[`CLAUDE.md`](https://github.com/KamerrEzz/stackyard/blob/main/CLAUDE.md)**
   — convenciones del proyecto: estilo de comentarios, qué va en el código
   frente a qué va en la documentación, y referencias a los demás
   archivos listados abajo. Empezar por acá; es corto e indica dónde vive
   todo lo demás.
2. **[`spec.md`](https://github.com/KamerrEzz/stackyard/blob/main/spec.md)**
   — la especificación funcional: planteamiento del problema, objetivos y
   criterios de aceptación para cada funcionalidad, módulo por módulo.
3. **[`plan.md`](https://github.com/KamerrEzz/stackyard/blob/main/plan.md)**
   — la arquitectura y el diseño técnico detrás de esa especificación:
   cómo están estructurados el backend en Go y el frontend en React, el
   esquema de almacenamiento, y las decisiones técnicas clave detrás de
   ellos.
4. **[`tasks.md`](https://github.com/KamerrEzz/stackyard/blob/main/tasks.md)**
   — el historial completo de construcción fase por fase, como checklist.
   Útil para entender qué existe ya y en qué orden fue construido.
5. **[`docs/STATE.md`](https://github.com/KamerrEzz/stackyard/blob/main/docs/STATE.md)**
   — un registro largo y detallado de cada sesión de trabajo: decisiones
   tomadas, su justificación, y detalles descubiertos en el camino. Es la
   fuente más rica sobre el **por qué** de las cosas en todo el
   repositorio — cuando `tasks.md` o el código en sí no explican una
   decisión, `STATE.md` casi con certeza sí lo hace.

Para una pasada rápida, `CLAUDE.md` + `docs/STATE.md` + `tasks.md` por sí
solos llevan a un asistente de IA (o a una persona) la mayor parte del
camino hacia el contexto completo; agregar `spec.md`/`plan.md` cuando el
cambio toca directamente el alcance del producto o la arquitectura.

`docs/STATE.md` es el registro interno de desarrollo de este proyecto,
distinto de este sitio de documentación pública — no está pensado como
lectura pulida, pero es el registro más honesto de la historia del
proyecto.
