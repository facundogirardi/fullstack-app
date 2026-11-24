package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

type Task struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

var ErrNotFound = errors.New("tarea no encontrada")

// TaskStore es lo que los handlers necesitan para guardar tareas.
// La implementan MemoryStore (memory_store.go) y PostgresStore (postgres_store.go).
type TaskStore interface {
	List() ([]Task, error)
	Create(title string) (Task, error)
	Toggle(id int) (Task, error)
	Delete(id int) error
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func handleStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	log.Println("error de almacenamiento:", err)
	writeError(w, http.StatusInternalServerError, "error interno")
}

func parseID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "id inválido")
		return 0, false
	}
	return id, true
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func newRouter(store TaskStore) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/tasks", func(w http.ResponseWriter, r *http.Request) {
		tasks, err := store.List()
		if err != nil {
			handleStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, tasks)
	})

	mux.HandleFunc("POST /api/tasks", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Title string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "JSON inválido")
			return
		}
		title := strings.TrimSpace(body.Title)
		if title == "" {
			writeError(w, http.StatusBadRequest, "el título es obligatorio")
			return
		}
		t, err := store.Create(title)
		if err != nil {
			handleStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, t)
	})

	mux.HandleFunc("PATCH /api/tasks/{id}/toggle", func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r)
		if !ok {
			return
		}
		t, err := store.Toggle(id)
		if err != nil {
			handleStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, t)
	})

	mux.HandleFunc("DELETE /api/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseID(w, r)
		if !ok {
			return
		}
		if err := store.Delete(id); err != nil {
			handleStoreError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	return cors(mux)
}

func main() {
	var store TaskStore

	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		pg, err := NewPostgresStore(dsn)
		if err != nil {
			log.Fatal("no se pudo conectar a la base de datos: ", err)
		}
		log.Println("Usando PostgreSQL")
		store = pg
	} else {
		mem := NewMemoryStore()
		mem.Create("Aprender Go")
		mem.Create("Aprender React con TypeScript")
		log.Println("Usando almacenamiento en memoria (DATABASE_URL no definida)")
		store = mem
	}

	// Cloud Run indica el puerto con la variable PORT.
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	log.Printf("Backend escuchando en http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, newRouter(store)))
}
