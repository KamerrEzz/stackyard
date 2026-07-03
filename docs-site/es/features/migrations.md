# Migraciones

Versionado de esquema mínimo y explícito para PostgreSQL y MySQL — sin
automatismos ocultos, sin rollback masivo.

- **Create migration** genera un par de archivos up/down con marca de
  tiempo, vinculado a un perfil de conexión.
- **Apply** ejecuta todas las migraciones pendientes en orden y registra
  el estado aplicado en una tabla de seguimiento (`schema_migrations`)
  dentro de la base de datos destino. Un fallo a mitad de la ejecución
  deja la tabla de seguimiento correcta — la migración fallida no queda
  marcada como aplicada — y muestra el error real de la base de datos.
- **Rollback** revierte exactamente una migración a la vez; no existe
  rollback masivo por diseño.

Un panel dedicado de migraciones lista las migraciones pendientes y
aplicadas por conexión, con acciones de apply/rollback disponibles
directamente desde la interfaz.
