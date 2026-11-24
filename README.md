# Fullstack App: Go + React (TypeScript)

Aplicación de tareas sencilla. Sirve como punto de partida para un proyecto con backend en Go, frontend en React con TypeScript y base de datos PostgreSQL.

- **Backend** (`backend/`): API REST en Go. Corre en el puerto **8080** (o el que indique la variable `PORT`).
- **Frontend** (`frontend/`): React 18 + TypeScript con Vite. Corre en el puerto **5173**.
- **Base de datos**: PostgreSQL. Es **opcional** en desarrollo: si no definís `DATABASE_URL`, el backend guarda los datos en memoria y se pierden al reiniciarlo.

---

## Índice

1. [Arquitectura](#1-arquitectura)
2. [Cómo funciona el backend](#2-cómo-funciona-el-backend)
3. [Cómo funciona el frontend](#3-cómo-funciona-el-frontend)
4. [Levantar y probar el proyecto](#4-levantar-y-probar-el-proyecto)
5. [Usar una base de datos (persistencia)](#5-usar-una-base-de-datos-persistencia)
6. [Desplegar en Google Cloud (GCP)](#6-desplegar-en-google-cloud-gcp)
7. [Problemas frecuentes](#7-problemas-frecuentes)

---

## 1. Arquitectura

```
fullstack-app/
├── docker-compose.yml      # PostgreSQL local para desarrollo
├── backend/
│   ├── go.mod              # Módulo de Go (Go 1.22+) y dependencias
│   ├── main.go             # Modelo, interfaz TaskStore, rutas HTTP y arranque
│   ├── memory_store.go     # Almacenamiento en memoria
│   ├── postgres_store.go   # Almacenamiento en PostgreSQL
│   ├── Dockerfile          # Imagen para Cloud Run
│   └── .dockerignore
└── frontend/
    ├── index.html          # HTML base donde se monta React
    ├── package.json        # Dependencias y scripts de npm
    ├── tsconfig.json       # Configuración de TypeScript (modo estricto)
    ├── vite.config.ts      # Configuración de Vite y proxy hacia el backend
    ├── firebase.json       # Configuración de Firebase Hosting (despliegue)
    ├── .firebaserc         # Proyecto de GCP/Firebase al que se despliega
    └── src/
        ├── main.tsx        # Punto de entrada: monta <App /> en #root
        ├── App.tsx         # Interfaz y estado de la app
        ├── api.ts          # Tipos y funciones que llaman a la API
        └── styles.css      # Estilos
```

Flujo de una petición en desarrollo:

```
Navegador (localhost:5173)
   │  fetch("/api/tasks")
   ▼
Servidor de Vite (5173) ── proxy /api ──►  Backend Go (8080)
                                              │
                                              ▼
                                  TaskStore: memoria o PostgreSQL
```

El navegador solo habla con Vite. Vite reenvía todo lo que empieza con `/api` al backend. Por eso el frontend usa rutas relativas (`/api/tasks`) y no necesita conocer la URL del backend.

---

## 2. Cómo funciona el backend

### 2.1 Modelo (`main.go`)

```go
type Task struct {
    ID    int    `json:"id"`
    Title string `json:"title"`
    Done  bool   `json:"done"`
}
```

Las etiquetas `json:"..."` indican cómo se llama cada campo en el JSON que se envía y recibe.

### 2.2 Interfaz de almacenamiento (`main.go`)

Los handlers no saben *dónde* se guardan los datos. Solo usan esta interfaz:

```go
type TaskStore interface {
    List() ([]Task, error)
    Create(title string) (Task, error)
    Toggle(id int) (Task, error)
    Delete(id int) error
}
```

Hay dos implementaciones:

| Implementación  | Archivo             | Cuándo se usa                    |
|-----------------|---------------------|----------------------------------|
| `MemoryStore`   | `memory_store.go`   | Si `DATABASE_URL` **no** está definida |
| `PostgresStore` | `postgres_store.go` | Si `DATABASE_URL` está definida  |

Si una tarea no existe, ambas devuelven el error `ErrNotFound`, que los handlers convierten en un `404`. Cualquier otro error se registra en el log y se responde `500 error interno` (así no se filtran detalles de la base de datos al cliente).

**`MemoryStore`** guarda las tareas en un `map[int]Task`:

- Usa un `sync.Mutex` porque el servidor HTTP de Go atiende cada petición en una goroutine distinta. Sin el mutex, dos peticiones simultáneas podrían corromper el mapa.
- `List` recorre los IDs en orden para que las tareas salgan en el orden en que se crearon (un `map` de Go no garantiza orden).

**`PostgresStore`** se explica en la [sección 5.3](#53-cómo-está-implementado).

### 2.3 Rutas HTTP (`newRouter` en `main.go`)

Se usa `http.ServeMux` con los patrones de Go 1.22, que permiten indicar el método y capturar parámetros (`{id}`):

| Método   | Ruta                      | Cuerpo                   | Respuesta correcta          | Errores posibles                  |
|----------|---------------------------|--------------------------|-----------------------------|-----------------------------------|
| `GET`    | `/api/tasks`              | —                        | `200` + lista de tareas     | `500`                             |
| `POST`   | `/api/tasks`              | `{ "title": "Comprar" }` | `201` + tarea creada        | `400` JSON inválido o título vacío, `500` |
| `PATCH`  | `/api/tasks/{id}/toggle`  | —                        | `200` + tarea actualizada   | `400` id inválido, `404` no existe, `500` |
| `DELETE` | `/api/tasks/{id}`         | —                        | `204` sin cuerpo            | `400` id inválido, `404` no existe, `500` |

Los errores siempre tienen la forma:

```json
{ "error": "tarea no encontrada" }
```

Funciones auxiliares:

- `writeJSON`: pone el `Content-Type`, escribe el código de estado y serializa la respuesta.
- `writeError`: atajo para responder `{ "error": "..." }`.
- `handleStoreError`: traduce los errores del almacenamiento a `404` o `500`.
- `parseID`: lee `{id}` con `r.PathValue("id")` y lo convierte a número; si falla responde `400`.

### 2.4 CORS

El middleware `cors` agrega las cabeceras que permiten que una página en `http://localhost:5173` llame directamente al backend, y responde a las peticiones `OPTIONS` (preflight) del navegador. En desarrollo no es estrictamente necesario porque el proxy de Vite evita el problema, y en GCP tampoco, porque Firebase Hosting hace lo mismo. Sirve si llamás al backend sin pasar por ninguno de los dos.

### 2.5 Arranque (`main`)

1. Si existe `DATABASE_URL`, se conecta a PostgreSQL (y termina con error si no puede). Si no, usa `MemoryStore` y carga dos tareas de ejemplo.
2. Lee el puerto de la variable `PORT` (la usa Cloud Run); si no está, usa `8080`.
3. Arranca el servidor.

El log indica qué almacenamiento se está usando: `Usando PostgreSQL` o `Usando almacenamiento en memoria`.

---

## 3. Cómo funciona el frontend

### 3.1 `src/api.ts`: capa de acceso a la API

- Define la interfaz `Task`, igual al JSON que devuelve el backend.
- `request<T>()` envuelve `fetch`: agrega la cabecera JSON, lanza un `Error` con el mensaje del backend si la respuesta no es `2xx`, y no intenta leer JSON cuando la respuesta es `204`.
- El objeto `api` expone una función por endpoint: `list`, `create`, `toggle` y `remove`.

Tener todas las llamadas aquí hace que los componentes no sepan nada de URLs ni de `fetch`.

### 3.2 `src/App.tsx`: interfaz y estado

Estado del componente:

| Estado    | Para qué sirve                                   |
|-----------|--------------------------------------------------|
| `tasks`   | Lista de tareas que se muestra                   |
| `title`   | Texto del campo "Nueva tarea"                    |
| `error`   | Mensaje de error a mostrar (o `null`)            |
| `loading` | `true` mientras se carga la lista por primera vez |

Funcionamiento:

1. Al montarse, un `useEffect` llama a `api.list()` y guarda el resultado. Si falla, muestra "No se pudo conectar con el backend".
2. Al enviar el formulario se llama a `api.create()` y la tarea nueva se agrega al final de la lista.
3. El checkbox llama a `api.toggle()` y reemplaza la tarea con la versión que devuelve el backend.
4. El botón ✕ llama a `api.remove()` y quita la tarea de la lista.
5. La función `run()` centraliza el manejo de errores: limpia el error anterior, ejecuta la acción y, si falla, muestra el mensaje.

El estado local solo se actualiza **después** de que el backend responde bien, así la pantalla siempre refleja lo que hay en el servidor.

### 3.3 `vite.config.ts`: proxy

```ts
server: {
  port: 5173,
  proxy: { "/api": "http://localhost:8080" },
}
```

Si cambiás el puerto del backend, cambialo también aquí.

---

## 4. Levantar y probar el proyecto

### 4.1 Requisitos

| Herramienta | Versión mínima | Verificar con    | Descarga                      | ¿Obligatorio? |
|-------------|----------------|------------------|-------------------------------|---------------|
| Go          | 1.22           | `go version`     | https://go.dev/dl/            | Sí            |
| Node.js     | 18             | `node --version` | https://nodejs.org/           | Sí            |
| Docker      | —              | `docker --version` | https://www.docker.com/products/docker-desktop/ | Solo para usar PostgreSQL en local |

> En Windows, después de instalar Go cerrá y volvé a abrir la terminal (o VS Code) para que se actualice el `PATH`.

### 4.2 Descargar las dependencias del backend (solo la primera vez)

```bash
cd backend
go mod tidy
```

Esto descarga el driver de PostgreSQL (`pgx`) y genera el archivo `go.sum`. **Hacelo antes de compilar o desplegar**: el `Dockerfile` necesita `go.sum`.

### 4.3 Levantar el backend

```bash
cd backend
go run .
```

Deberías ver:

```
Usando almacenamiento en memoria (DATABASE_URL no definida)
Backend escuchando en http://localhost:8080
```

#### Compilar un ejecutable (opcional)

**Cómo compilarlo, paso a paso:**

1. Abrí una terminal en la carpeta del backend:
   ```powershell
   cd backend
   ```
2. Si es la primera vez, descargá las dependencias (ver [4.2](#42-descargar-las-dependencias-del-backend-solo-la-primera-vez)):
   ```powershell
   go mod tidy
   ```
3. (Recomendado) Revisá el código en busca de errores comunes:
   ```powershell
   go vet ./...
   ```
4. Compilá:
   ```powershell
   go build -o tareas-backend.exe .
   ```
   - `go build` compila el programa.
   - `-o tareas-backend.exe` indica el nombre del archivo que se genera. Sin `-o`, se llamaría como el módulo (`fullstack-backend.exe`).
   - `.` significa "compilar el paquete de esta carpeta" (`main.go`, `memory_store.go` y `postgres_store.go`).

   Si todo sale bien, **no muestra nada** y aparece `tareas-backend.exe` en la carpeta. Si hay un error en el código, lo muestra con el archivo y la línea, y no genera el `.exe`.
5. Ejecutalo:
   ```powershell
   .\tareas-backend.exe
   ```
   Deberías ver `Backend escuchando en http://localhost:8080`. Se detiene con `Ctrl+C`.

**Ejecutable más chico (opcional):** quitando la información de depuración el archivo pesa bastante menos. Sirve para distribuirlo; para desarrollar no hace falta.

```powershell
go build -ldflags "-s -w" -o tareas-backend.exe .
```

**Errores frecuentes al compilar:**

| Error | Solución |
|-------|----------|
| `go : El término 'go' no se reconoce...` | Go no está instalado o la terminal no ve el `PATH`. Reabrí la terminal |
| `missing go.sum entry...` o `no required module provides package` | Ejecutá `go mod tidy` |
| `Acceso denegado` / `The process cannot access the file` | `tareas-backend.exe` está corriendo. Detenelo con `Ctrl+C` y volvé a compilar |

**Qué es `tareas-backend.exe`:** el backend ya compilado, en un solo archivo. Incluye todo el programa y el driver de PostgreSQL, así que no necesita nada más para funcionar.

**Para qué sirve:**
- **Arrancar más rápido:** hace lo mismo que `go run .`, pero no compila cada vez.
- **Usarlo en otra PC con Windows sin instalar Go:** alcanza con copiar el archivo y ejecutarlo.
- **Configurarlo igual que con `go run`:** acepta las mismas variables `PORT` y `DATABASE_URL`.

  ```powershell
  $env:PORT = "9090"; $env:DATABASE_URL = "postgres://tareas:tareas@localhost:5432/tareas?sslmode=disable"; .\tareas-backend.exe
  ```

**Tené en cuenta:**
- **No se actualiza solo.** Si cambiás el código, el `.exe` sigue teniendo la versión anterior hasta que lo vuelvas a compilar.
- **Solo funciona en Windows.** Para Linux o Mac hay que compilarlo para ese sistema con las variables `GOOS` y `GOARCH`:

  ```powershell
  # Linux
  $env:GOOS = "linux"; $env:GOARCH = "amd64"; go build -o tareas-backend .

  # Mac con chip Apple (M1, M2...)
  $env:GOOS = "darwin"; $env:GOARCH = "arm64"; go build -o tareas-backend .

  # Volver a compilar para Windows
  Remove-Item Env:GOOS, Env:GOARCH
  ```

- **No hace falta para desplegar en GCP:** el `Dockerfile` compila su propio binario para Linux dentro del contenedor (ver [6.6](#66-el-contenedor-del-backend)).
- **No hace falta para desarrollar:** `go run .` compila y ejecuta en un solo paso.
- **No se sube a git:** es un archivo generado que se puede recrear en cualquier momento, por eso está en el `.gitignore`. Se puede borrar sin problema.

### 4.4 Levantar el frontend

En **otra** terminal:

```bash
cd frontend
npm install      # solo la primera vez
npm run dev
```

Abrí http://localhost:5173. Tenés que ver las dos tareas de ejemplo.

Otros scripts:

| Comando           | Qué hace                                                    |
|-------------------|-------------------------------------------------------------|
| `npm run dev`     | Servidor de desarrollo con recarga automática               |
| `npm run build`   | Revisa tipos con TypeScript y genera la versión final en `dist/` |
| `npm run preview` | Sirve localmente lo que hay en `dist/`                      |

### 4.5 Probar la API a mano

Con el backend corriendo podés probar cada endpoint sin el frontend.

**Con `curl`** (en PowerShell escribí `curl.exe`, porque `curl` a secas es un alias de otro comando):

> **Ojo en Windows PowerShell 5.1:** al pasarle a `curl.exe` un JSON con `-d "{\"title\":...}"`, PowerShell rompe las comillas y el backend responde `JSON inválido`. Los ejemplos con `-d` funcionan tal cual en Git Bash o en `cmd`. En PowerShell, guardá el cuerpo en un archivo y mandalo con `--data-binary "@body.json"`, o usá `Invoke-RestMethod` (más abajo).

```bash
# Listar
curl.exe http://localhost:8080/api/tasks

# Crear
curl.exe -X POST http://localhost:8080/api/tasks -H "Content-Type: application/json" -d "{\"title\":\"Comprar pan\"}"

# Marcar / desmarcar la tarea 1
curl.exe -X PATCH http://localhost:8080/api/tasks/1/toggle

# Eliminar la tarea 1 (-i muestra el código de estado: 204)
curl.exe -i -X DELETE http://localhost:8080/api/tasks/1
```

**Con PowerShell (`Invoke-RestMethod`)**:

```powershell
Invoke-RestMethod http://localhost:8080/api/tasks

Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/tasks `
  -ContentType "application/json" -Body '{"title":"Comprar pan"}'

Invoke-RestMethod -Method Patch  -Uri http://localhost:8080/api/tasks/1/toggle
Invoke-RestMethod -Method Delete -Uri http://localhost:8080/api/tasks/1
```

**Probar los errores**:

```bash
# 400: título vacío
curl.exe -i -X POST http://localhost:8080/api/tasks -H "Content-Type: application/json" -d "{\"title\":\"\"}"

# 400: id que no es número
curl.exe -i -X PATCH http://localhost:8080/api/tasks/abc/toggle

# 404: tarea inexistente
curl.exe -i -X DELETE http://localhost:8080/api/tasks/999
```

También podés usar Postman, Insomnia o la extensión REST Client de VS Code con las mismas URLs.

### 4.6 Probar la app completa

1. Agregá una tarea desde la interfaz y comprobá que aparece en `curl.exe http://localhost:8080/api/tasks`.
2. Marcala como hecha: debe tacharse y el contador de pendientes debe bajar.
3. Eliminala y comprobá que desaparece también de la API.
4. Detené el backend (`Ctrl+C`) y recargá la página: debe aparecer el mensaje de error de conexión.
5. Volvé a levantar el backend: en modo memoria las tareas que creaste **ya no están**. Para que se mantengan, usá PostgreSQL (siguiente sección).

---

## 5. Usar una base de datos (persistencia)

El backend ya tiene soporte para **PostgreSQL** con el driver [`pgx`](https://github.com/jackc/pgx). Solo hay que levantar la base y definir la variable `DATABASE_URL`. El frontend **no cambia**: la API responde igual.

### 5.1 Levantar PostgreSQL con Docker

Necesitás [Docker Desktop](https://www.docker.com/products/docker-desktop/) abierto. Desde la carpeta `fullstack-app/`:

```bash
docker compose up -d
```

Eso usa el [`docker-compose.yml`](docker-compose.yml) incluido, que crea:

- Un contenedor con PostgreSQL 16 en el puerto `5432`.
- Usuario `tareas`, contraseña `tareas` y base `tareas` (solo para desarrollo local).
- Un volumen `tareas-data` donde se guardan los datos, que sobreviven aunque borres el contenedor.

Comandos útiles:

| Comando | Qué hace |
|---------|----------|
| `docker compose ps` | Muestra si la base está corriendo |
| `docker compose stop` | Detiene la base (los datos se mantienen) |
| `docker compose start` | La vuelve a arrancar |
| `docker compose down -v` | Borra el contenedor **y los datos** |

### 5.2 Ejecutar el backend con la base de datos

La cadena de conexión local es:

```
postgres://tareas:tareas@localhost:5432/tareas?sslmode=disable
```

**PowerShell:**

```powershell
cd backend
$env:DATABASE_URL = "postgres://tareas:tareas@localhost:5432/tareas?sslmode=disable"
go run .
```

**Bash / Git Bash:**

```bash
cd backend
DATABASE_URL="postgres://tareas:tareas@localhost:5432/tareas?sslmode=disable" go run .
```

Deberías ver `Usando PostgreSQL`. La variable vale solo para esa terminal: si abrís otra, hay que definirla de nuevo.

En modo PostgreSQL no se cargan tareas de ejemplo: la lista empieza vacía.

### 5.3 Cómo está implementado

Todo está en [`backend/postgres_store.go`](backend/postgres_store.go):

- **Conexión:** `NewPostgresStore` abre la conexión con `sql.Open("pgx", dsn)` y la prueba con `db.Ping()`. Si la base no responde, el backend se detiene con un mensaje claro en vez de fallar más tarde.
- **Tabla:** al arrancar ejecuta `CREATE TABLE IF NOT EXISTS tasks (...)`, así no hay que crearla a mano:

  | Columna | Tipo      | Notas                    |
  |---------|-----------|--------------------------|
  | `id`    | `SERIAL`  | Clave primaria, autoincremental |
  | `title` | `TEXT`    | Obligatorio              |
  | `done`  | `BOOLEAN` | Por defecto `FALSE`      |

- **Consultas parametrizadas:** los valores se pasan con `$1` en lugar de concatenarlos en el SQL. Eso evita **inyección SQL**.
- **`RETURNING`:** `INSERT` y `UPDATE` devuelven la fila creada o modificada en la misma consulta, sin un `SELECT` extra.
- **"No encontrado":** `Toggle` detecta `sql.ErrNoRows` y `Delete` revisa `RowsAffected()`. En los dos casos devuelven `ErrNotFound`, igual que `MemoryStore`.
- **Concurrencia:** `sql.DB` ya es un pool de conexiones seguro para varias goroutines, así que no hace falta un mutex.

### 5.4 Comprobar que los datos persisten

1. Creá algunas tareas desde la interfaz.
2. Detené el backend con `Ctrl+C` y volvé a levantarlo con el mismo comando.
3. Recargá la página: las tareas siguen ahí.

Para mirar la tabla directamente:

```bash
docker compose exec db psql -U tareas -d tareas -c "SELECT * FROM tasks;"
```

### 5.5 Alternativa sin Docker: SQLite

SQLite guarda todo en un único archivo y no necesita un servidor. Para usarlo, podés crear un `sqlite_store.go` copiando `postgres_store.go` con el driver [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) (escrito en Go puro, funciona en Windows sin compilador de C):

```bash
go get modernc.org/sqlite
```

Cambios respecto de PostgreSQL:

| PostgreSQL                               | SQLite                                       |
|------------------------------------------|----------------------------------------------|
| `_ "github.com/jackc/pgx/v5/stdlib"`     | `_ "modernc.org/sqlite"`                     |
| `sql.Open("pgx", dsn)`                   | `sql.Open("sqlite", "tareas.db")`            |
| `id SERIAL PRIMARY KEY`                  | `id INTEGER PRIMARY KEY AUTOINCREMENT`       |
| `done BOOLEAN NOT NULL DEFAULT FALSE`    | `done INTEGER NOT NULL DEFAULT 0`            |
| Parámetros `$1`, `$2`                    | Parámetros `?`                               |
| `SET done = NOT done`                    | `SET done = 1 - done`                        |

`RETURNING` también funciona en SQLite. Después elegí `SQLiteStore` en `main()` igual que se elige `PostgresStore`. El archivo `tareas.db` ya está en el `.gitignore`.

---

## 6. Desplegar en Google Cloud (GCP)

El código ya está preparado para desplegarse: el backend lee `PORT` y `DATABASE_URL`, y están incluidos el `Dockerfile` y la configuración de Firebase.

### 6.1 Arquitectura en la nube

```
Usuario
  │  https://TU_PROYECTO.web.app
  ▼
Firebase Hosting ──── archivos estáticos del frontend (dist/)
  │
  │  /api/**  (rewrite)
  ▼
Cloud Run: tareas-backend ──── contenedor con el backend en Go
  │
  │  socket de Cloud SQL (/cloudsql/...)
  ▼
Cloud SQL: PostgreSQL 16
```

| Pieza     | Servicio de GCP     | Por qué |
|-----------|---------------------|---------|
| Frontend  | **Firebase Hosting** | Sirve archivos estáticos con HTTPS y CDN. Redirige `/api/**` a Cloud Run, así el frontend sigue usando rutas relativas y no hay problemas de CORS. |
| Backend   | **Cloud Run**        | Ejecuta contenedores sin administrar servidores, escala a cero cuando no hay tráfico y se conecta a Cloud SQL de forma nativa. |
| Base de datos | **Cloud SQL (PostgreSQL)** | PostgreSQL administrado: backups, parches y alta disponibilidad opcional. |
| Contraseña | **Secret Manager** | Guarda la `DATABASE_URL` fuera del código y de la configuración visible. |

En todos los comandos reemplazá:

- `TU_PROYECTO` por el ID de tu proyecto de GCP.
- `TU_PASSWORD` por una contraseña segura para la base de datos.
- `us-central1` por otra región si preferís. Usá la misma en todos los pasos **y en `frontend/firebase.json`**.

### 6.2 Requisitos

1. Una cuenta de Google Cloud con un **proyecto** creado y **facturación habilitada** (Cloud SQL no funciona sin facturación).
2. [Google Cloud CLI (`gcloud`)](https://cloud.google.com/sdk/docs/install) instalada.
3. Haber ejecutado `go mod tidy` en `backend/` (ver [4.2](#42-descargar-las-dependencias-del-backend-solo-la-primera-vez)), para que exista `go.sum`.

Iniciar sesión y elegir el proyecto:

```bash
gcloud auth login
gcloud config set project TU_PROYECTO
```

Habilitar las APIs necesarias:

```bash
gcloud services enable run.googleapis.com sqladmin.googleapis.com cloudbuild.googleapis.com artifactregistry.googleapis.com secretmanager.googleapis.com
```

### 6.3 Crear la base de datos en Cloud SQL

```bash
# Instancia de PostgreSQL 16 (la más chica; tarda varios minutos en crearse)
gcloud sql instances create tareas-db --database-version=POSTGRES_16 --edition=ENTERPRISE --tier=db-f1-micro --region=us-central1

# Base de datos y usuario
gcloud sql databases create tareas --instance=tareas-db
gcloud sql users create tareas --instance=tareas-db --password=TU_PASSWORD
```

Obtené el **nombre de conexión** de la instancia, lo vas a necesitar después:

```bash
gcloud sql instances describe tareas-db --format="value(connectionName)"
# Ejemplo de salida: TU_PROYECTO:us-central1:tareas-db
```

> `db-f1-micro` es un tamaño compartido, suficiente para pruebas. Para producción elegí un `--tier` mayor y considerá `--availability-type=REGIONAL`.

La tabla `tasks` no hay que crearla a mano: el backend la crea al arrancar.

### 6.4 Guardar la cadena de conexión en Secret Manager

Cloud Run se conecta a Cloud SQL por un **socket Unix** que aparece dentro del contenedor en `/cloudsql/NOMBRE_DE_CONEXION`. Con `pgx` la cadena de conexión queda así:

```
host=/cloudsql/TU_PROYECTO:us-central1:tareas-db user=tareas password=TU_PASSWORD dbname=tareas sslmode=disable
```

(`sslmode=disable` es correcto aquí: la conexión por socket ya va cifrada y autenticada por Cloud SQL.)

Creá el secreto:

**Bash / Git Bash:**

```bash
printf '%s' "host=/cloudsql/TU_PROYECTO:us-central1:tareas-db user=tareas password=TU_PASSWORD dbname=tareas sslmode=disable" | gcloud secrets create database-url --data-file=-
```

**PowerShell:**

```powershell
"host=/cloudsql/TU_PROYECTO:us-central1:tareas-db user=tareas password=TU_PASSWORD dbname=tareas sslmode=disable" | Out-File -NoNewline -Encoding ascii dsn.txt
gcloud secrets create database-url --data-file=dsn.txt
Remove-Item dsn.txt
```

### 6.5 Dar permisos a Cloud Run

Cloud Run usa por defecto la cuenta de servicio de Compute Engine. Necesita poder leer el secreto y conectarse a Cloud SQL.

Obtené el número del proyecto:

```bash
gcloud projects describe TU_PROYECTO --format="value(projectNumber)"
```

Y otorgá los permisos (reemplazá `NUMERO_PROYECTO`):

```bash
gcloud projects add-iam-policy-binding TU_PROYECTO --member="serviceAccount:NUMERO_PROYECTO-compute@developer.gserviceaccount.com" --role="roles/secretmanager.secretAccessor"

gcloud projects add-iam-policy-binding TU_PROYECTO --member="serviceAccount:NUMERO_PROYECTO-compute@developer.gserviceaccount.com" --role="roles/cloudsql.client"
```

### 6.6 El contenedor del backend

No hay que preparar nada: [`backend/Dockerfile`](backend/Dockerfile) ya está incluido.

```dockerfile
# Etapa 1: compilar
FROM golang:1.22 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o server .

# Etapa 2: imagen mínima solo con el binario
FROM gcr.io/distroless/static-debian12
COPY --from=build /app/server /server
ENTRYPOINT ["/server"]
```

- La compilación en dos etapas deja una imagen final de pocos MB, sin el compilador de Go ni el código fuente.
- `go.mod` y `go.sum` se copian antes que el resto para que Docker reutilice la descarga de dependencias si solo cambió el código.
- Si tenés Docker, podés probar la imagen en local:

  ```bash
  cd backend
  docker build -t tareas-backend .
  docker run --rm -p 8080:8080 tareas-backend
  ```

### 6.7 Desplegar el backend en Cloud Run

Desde la carpeta `backend/`:

```bash
gcloud run deploy tareas-backend --source . --region=us-central1 --allow-unauthenticated --add-cloudsql-instances=TU_PROYECTO:us-central1:tareas-db --set-secrets=DATABASE_URL=database-url:latest
```

Qué hace cada opción:

| Opción | Efecto |
|--------|--------|
| `--source .` | Sube el código, lo compila con Cloud Build usando el `Dockerfile` y guarda la imagen en Artifact Registry |
| `--allow-unauthenticated` | Permite llamar a la API sin iniciar sesión en Google (necesario para que la use el frontend) |
| `--add-cloudsql-instances` | Monta el socket de Cloud SQL en `/cloudsql/...` dentro del contenedor |
| `--set-secrets` | Expone el secreto `database-url` como la variable de entorno `DATABASE_URL` |

La primera vez puede preguntar si querés crear un repositorio de Artifact Registry: respondé que sí.

Al terminar muestra una URL del tipo `https://tareas-backend-xxxxx-uc.a.run.app`. Probala:

```bash
curl.exe https://tareas-backend-xxxxx-uc.a.run.app/api/tasks
# Respuesta esperada: []
```

En los logs tiene que aparecer `Usando PostgreSQL`:

```bash
gcloud run services logs read tareas-backend --region=us-central1
```

### 6.8 Desplegar el frontend en Firebase Hosting

**1. Instalar la CLI de Firebase e iniciar sesión:**

```bash
npm install -g firebase-tools
firebase login
```

**2. Agregar Firebase a tu proyecto de GCP** (solo la primera vez):

```bash
firebase projects:addfirebase TU_PROYECTO
```

**3. Poner el ID de tu proyecto en [`frontend/.firebaserc`](frontend/.firebaserc)**, reemplazando `TU_PROYECTO`:

```json
{
  "projects": {
    "default": "TU_PROYECTO"
  }
}
```

[`frontend/firebase.json`](frontend/firebase.json) ya está listo:

```json
{
  "hosting": {
    "public": "dist",
    "ignore": ["firebase.json", "**/.*", "**/node_modules/**"],
    "rewrites": [
      {
        "source": "/api/**",
        "run": { "serviceId": "tareas-backend", "region": "us-central1" }
      },
      { "source": "**", "destination": "/index.html" }
    ]
  }
}
```

- La primera regla reenvía las llamadas a `/api/...` al servicio de Cloud Run. Cumple el mismo papel que el proxy de Vite en desarrollo, por eso **no hay que cambiar nada del código del frontend**.
- La segunda devuelve `index.html` para cualquier otra ruta (útil si más adelante agregás un router).
- Si usaste otro nombre de servicio o de región, cambialos aquí.

**4. Compilar y desplegar:**

```bash
cd frontend
npm run build
firebase deploy --only hosting
```

La app queda disponible en `https://TU_PROYECTO.web.app`.

### 6.9 Probar el despliegue

1. Abrí `https://TU_PROYECTO.web.app` y creá algunas tareas.
2. Comprobá que la API responde a través de Hosting: `curl.exe https://TU_PROYECTO.web.app/api/tasks`.
3. Forzá un reinicio del backend desplegando de nuevo (`gcloud run deploy ...`) y recargá la página: las tareas siguen ahí porque están en Cloud SQL.
4. Si querés mirar la tabla, conectate desde Cloud Shell:

   ```bash
   gcloud sql connect tareas-db --user=tareas --database=tareas
   ```

   y ejecutá `SELECT * FROM tasks;`.

### 6.10 Actualizar la app

| Cambiaste... | Ejecutá |
|--------------|---------|
| El backend | `gcloud run deploy tareas-backend --source . --region=us-central1` (desde `backend/`; las demás opciones se mantienen de la vez anterior) |
| El frontend | `npm run build` y `firebase deploy --only hosting` (desde `frontend/`) |
| La contraseña de la base | `gcloud sql users set-password tareas --instance=tareas-db --password=NUEVA`, luego `gcloud secrets versions add database-url --data-file=...` con la nueva cadena, y volvé a desplegar el backend |

### 6.11 Costos y cómo borrar todo

- **Cloud Run** y **Firebase Hosting** tienen niveles gratuitos que alcanzan de sobra para un proyecto de prueba.
- **Cloud SQL no es gratuito**: la instancia se cobra por hora **mientras exista**, aunque nadie use la app. Revisá el precio actual en https://cloud.google.com/sql/pricing.

Para no pagar mientras no la usás, podés detener la instancia (sigue cobrando el almacenamiento, pero no la CPU):

```bash
gcloud sql instances patch tareas-db --activation-policy=NEVER    # detener
gcloud sql instances patch tareas-db --activation-policy=ALWAYS   # volver a encender
```

Para borrar todo:

```bash
gcloud run services delete tareas-backend --region=us-central1
gcloud sql instances delete tareas-db
gcloud secrets delete database-url
firebase hosting:disable
```

> Borrar la instancia de Cloud SQL elimina **todos los datos** y no se puede deshacer. Si los necesitás, exportalos antes con `gcloud sql export sql`.

### 6.12 Alternativas

- **Frontend en Cloud Storage** en vez de Firebase Hosting: subís `dist/` a un bucket público. No tiene rewrites, así que el frontend debe llamar a la URL completa de Cloud Run (por ejemplo con una variable `VITE_API_URL` usada en `api.ts`) y hay que agregar el dominio del bucket al middleware `cors` del backend.
- **Todo en un solo servicio de Cloud Run**: el backend puede servir también los archivos de `dist/` con `http.FileServer`. Es un único despliegue, pero se pierde el CDN de Firebase Hosting.
- **Conexión privada a Cloud SQL**: para producción podés crear la instancia solo con IP privada (`--no-assign-ip` y una red VPC) y conectar Cloud Run por VPC.

---

## 7. Problemas frecuentes

| Síntoma | Causa probable | Solución |
|---------|----------------|----------|
| `go : El término 'go' no se reconoce...` | Go no está instalado o la terminal no ve el `PATH` nuevo | Instalá Go y reabrí la terminal / VS Code |
| `missing go.sum entry for module providing package github.com/jackc/pgx/...` | No se ejecutó `go mod tidy` | `cd backend` y `go mod tidy` |
| La página muestra "No se pudo conectar con el backend" | El backend no está corriendo o está en otro puerto | Levantá `go run .` en `backend/` y revisá el puerto en `vite.config.ts` |
| `listen tcp :8080: bind: Only one usage...` | Otro programa usa el puerto 8080 | Cerralo, o levantá el backend con otra `PORT` y cambiá el proxy en `vite.config.ts` |
| Error de CORS en la consola del navegador | El frontend llama al backend sin pasar por el proxy, desde un origen distinto a `localhost:5173` | Usá rutas relativas (`/api/...`) o agregá tu origen en el middleware `cors` |
| `no se pudo conectar a la base de datos: ... connection refused` | PostgreSQL no está corriendo | `docker compose up -d` y comprobá con `docker compose ps` |
| `password authentication failed` | La `DATABASE_URL` no coincide con el `docker-compose.yml` | Revisá usuario, contraseña y nombre de la base |
| Las tareas desaparecen al reiniciar | `DATABASE_URL` no está definida en esa terminal | Definila de nuevo; el log debe decir `Usando PostgreSQL` |
| **GCP:** el build falla con `COPY failed: ... go.sum: not found` | Falta `go.sum` | Ejecutá `go mod tidy` en `backend/` y volvé a desplegar |
| **GCP:** el despliegue en Cloud Run falla con "container failed to start and listen on the port" | El backend no pudo conectarse a la base al arrancar | Revisá `gcloud run services logs read tareas-backend --region=us-central1`, el secreto y los permisos de 6.5 |
| **GCP:** `permission denied` al leer el secreto o al conectar a Cloud SQL | Falta algún rol en la cuenta de servicio | Repetí los comandos de 6.5 con el número de proyecto correcto |
| **GCP:** `https://TU_PROYECTO.web.app/api/tasks` devuelve 404 o el HTML de la app | El `serviceId` o la `region` del rewrite no coinciden con el servicio de Cloud Run | Revisá `firebase.json` y volvé a ejecutar `firebase deploy --only hosting` |
| **GCP:** `Cloud SQL instance does not exist` | El nombre de conexión está mal escrito | Copialo de `gcloud sql instances describe tareas-db --format="value(connectionName)"` |
