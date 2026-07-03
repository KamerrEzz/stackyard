# Gestor de entornos

El Gestor de entornos se encarga por completo del ciclo de "levantar una
base de datos local", de punta a punta, para que nunca sea necesario
escribir ni editar a mano un archivo `docker-compose.yml`.

![Environments — crear y gestionar perfiles](/screenshots/environments.png)

## Perfiles

Un **perfil** es un conjunto nombrado y reutilizable de servicios —
cualquier combinación de PostgreSQL, MySQL, MongoDB y Redis, cada uno con
su propia imagen/versión, puerto de host, credenciales, nombre de base de
datos/esquema inicial, y volumen. Los perfiles persisten localmente y
sobreviven a un reinicio de la app; pueden duplicarse, renombrarse y
eliminarse.

## Ciclo de vida con un clic

Iniciar un perfil que nunca corrió antes crea automáticamente los
recursos de Docker necesarios — red, volúmenes nombrados, contenedores —,
equivalente a un `docker compose up -d` implícito, pero sin que se
escriba nunca un archivo compose en disco. Para un perfil ya configurado,
el flujo completo es: **seleccionar perfil → hacer clic en Start**. Los
conflictos de puerto se detectan antes de iniciar y se muestran junto con
un puerto libre sugerido, en lugar de un error crudo de Docker.

## Cadenas de conexión

Cada servicio en ejecución obtiene una cadena de conexión autogenerada en
el formato de URL canónico de su motor (`postgres://`, `mysql://`,
`mongodb://`, `redis://`). Un clic la copia al portapapeles, con una
confirmación en pantalla.

## Reinicio de volumen / datos

"Reset data" sobre un solo servicio lo detiene, elimina únicamente su
volumen, y lo recrea limpio en el próximo inicio — los servicios hermanos
dentro del mismo perfil siguen corriendo durante todo el proceso. La
acción requiere confirmación explícita, dado que es destructiva e
irreversible.

## Estado en tiempo real

Un panel en vivo muestra cada contenedor gestionado en todos los
perfiles: estado, puerto mapeado, % de CPU y uso de RAM, actualizados de
forma continua — incluyendo contenedores iniciados o detenidos fuera de
la app (por ejemplo, vía Docker Desktop o la CLI directamente).
