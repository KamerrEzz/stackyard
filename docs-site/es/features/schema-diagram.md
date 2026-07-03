# Diagrama de esquema

Un diagrama entidad-relación generado directamente a partir de
introspección de esquema en vivo — sin modelado manual, sin herramientas
externas.

![Diagrama ER en vivo con una relación de clave foránea](/screenshots/schema-diagram.png)

## Relacional (PostgreSQL, MySQL)

El diagrama se construye a partir de introspección en vivo del esquema
activo — tablas, columnas, tipos, claves primarias y claves foráneas — y
se renderiza dentro de la app usando la sintaxis `erDiagram` de Mermaid,
por lo que no se requiere ningún archivo externo para visualizarlo.
Admite zoom y desplazamiento para esquemas grandes, manteniéndose legible
incluso proyectado en la pantalla de un aula. Un botón **Regenerate**
actualiza el diagrama bajo demanda; intencionalmente no es una vista en
vivo ni de actualización automática. Los diagramas se exportan como PNG,
SVG, o texto Mermaid copiable sin procesar, de modo que el diagrama pueda
pegarse directamente en notas o en un README.

## MongoDB (estructura inferida)

Dado que las colecciones de MongoDB no tienen claves foráneas reales, el
diagrama infiere la forma de cada colección a partir de una muestra de
documentos (tamaño de muestra configurable, con un valor por defecto
razonable) y se etiqueta explícitamente como **"estructura inferida, no
una relación forzada"** — reforzando la diferencia real entre el
modelado relacional y el de documentos en lugar de disimularla. Comparte
las mismas capacidades de exportación que el diagrama relacional.
