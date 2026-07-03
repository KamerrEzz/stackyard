# Cliente de BD

El Cliente de BD es una interfaz de base de datos multi-motor integrada
en la misma app que el Gestor de entornos — permite conectarse a
PostgreSQL, MySQL, MongoDB o Redis y trabajar con los datos sin cambiar
de herramienta.

![Guía de bienvenida del Cliente de BD](/screenshots/db-client-onboarding.png)

El diseño de 3 paneles mantiene las conexiones y el esquema a la
izquierda, y las pestañas de Query/Data/Tools a la derecha, de modo que
la interfaz se explica por sí sola — una guía de bienvenida recorre los
tres pasos (conectar, navegar/editar, consultar/organizar) la primera vez
que no hay ninguna conexión abierta.

## Conectar por URL

Al pegar una cadena de conexión, cada campo del formulario de conexión —
host, puerto, usuario, contraseña, base de datos, parámetros de
consulta — se completa automáticamente. Las cadenas mal formadas producen
un error en pantalla que nombra la parte problemática. Un botón de
**Test connection** con un solo clic valida la conectividad antes de
guardar.

## Sesiones multi-pestaña

Múltiples conexiones y pestañas de consulta permanecen abiertas de forma
simultánea e independiente — cerrar una pestaña nunca afecta las
transacciones abiertas ni las ediciones sin enviar de otra pestaña.

## Editor de consultas

Un editor basado en Monaco con resaltado de sintaxis adaptado al motor
activo (dialecto SQL, estilo shell de Mongo, o comandos de Redis),
autocompletado obtenido del esquema en vivo, ejecución de múltiples
sentencias con reporte de éxito/fallo por cada una, y consultas
cancelables.

![Editor de consultas con una plantilla cargada](/screenshots/query-editor.png)

## Grilla de datos editable estilo hoja de cálculo

Para PostgreSQL y MySQL, los datos de tabla se abren en una grilla estilo
hoja de cálculo: doble clic en una celda la edita en el lugar (se
confirma como un `UPDATE` real por clave primaria), clic derecho en una
fila abre un menú contextual, y **+ Add row** genera un `INSERT` real.
Las tablas sin una clave primaria identificable permanecen de solo
lectura, con el motivo indicado, ya que no hay una forma segura de
apuntar a una única fila sin ella. Las escrituras fallidas muestran el
mensaje de error real de la base de datos directamente sobre la celda o
fila afectada.

![Grilla de datos editable con datos de ejemplo](/screenshots/data-grid.png)

## Documentos de MongoDB y claves de Redis

Las colecciones de MongoDB se representan como un árbol
expandible/colapsable que refleja la estructura BSON, con ediciones en el
lugar validadas como JSON antes de guardar. Las claves de Redis son
navegables y editables en los tipos string, hash, list, set y sorted-set,
con vista/edición de TTL y filtrado de claves por patrón (por ejemplo,
`session:*`).

## Snippets, historial y plantillas

Las consultas usadas con frecuencia pueden nombrarse, etiquetarse y
guardarse — con alcance a una sola conexión o marcadas como globales.
Cada ejecución queda registrada con marca de tiempo, duración,
éxito/fallo y cantidad de filas, filtrable y reproducible en una nueva
pestaña. Una galería de plantillas SQL iniciales (por ejemplo, "Auth:
users + sessions + tokens") puede insertarse con un solo clic.

## Crear tabla

Un formulario de "Create table" (nombre + columnas con
tipo/nullable/clave primaria/valor por defecto) genera y ejecuta un
`CREATE TABLE` real para PostgreSQL y MySQL — sin necesidad de escribir
SQL por separado para la configuración común de esquema.
