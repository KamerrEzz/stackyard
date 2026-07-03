# Importar / Exportar

Mueve datos hacia dentro y hacia fuera de una conexión sin salir de
Stackyard.

## Exportar

El alcance de la exportación es explícito: una tabla completa, o el
conjunto de resultados de la consulta actualmente ejecutada.

- Las exportaciones en **CSV** y **JSON** preservan los tipos de columna
  con la fidelidad suficiente para poder reimportarse — fechas, números y
  valores nulos permanecen distinguibles de una cadena vacía.
- La exportación como **dump SQL** genera sentencias `CREATE TABLE` +
  `INSERT` válidas, importables en una instancia nueva del mismo motor
  (PostgreSQL/MySQL).
- Las exportaciones de solo esquema también están disponibles como un
  archivo real `schema.prisma` o un archivo `schema.ts` de Drizzle,
  generados a partir de la misma introspección de tablas usada en el
  resto del Cliente de BD.

## Importar

La importación de CSV/JSON valida el archivo contra las columnas de la
tabla destino **antes** de confirmar los cambios — las discrepancias se
reportan por adelantado, y no se escribe nada si la validación falla.
