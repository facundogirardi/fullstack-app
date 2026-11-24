import { FormEvent, useEffect, useState } from "react";
import { api, Task } from "./api";

export default function App() {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [title, setTitle] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api
      .list()
      .then(setTasks)
      .catch((e: Error) => setError(`No se pudo conectar con el backend: ${e.message}`))
      .finally(() => setLoading(false));
  }, []);

  async function run(action: () => Promise<void>) {
    try {
      setError(null);
      await action();
    } catch (e) {
      setError((e as Error).message);
    }
  }

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    run(async () => {
      const task = await api.create(title);
      setTasks((prev) => [...prev, task]);
      setTitle("");
    });
  }

  const toggle = (id: number) =>
    run(async () => {
      const updated = await api.toggle(id);
      setTasks((prev) => prev.map((t) => (t.id === id ? updated : t)));
    });

  const remove = (id: number) =>
    run(async () => {
      await api.remove(id);
      setTasks((prev) => prev.filter((t) => t.id !== id));
    });

  const pending = tasks.filter((t) => !t.done).length;

  return (
    <main className="container">
      <h1>Tareas</h1>

      <form onSubmit={handleSubmit} className="form">
        <input
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="Nueva tarea…"
        />
        <button type="submit" disabled={!title.trim()}>
          Agregar
        </button>
      </form>

      {error && <p className="error">{error}</p>}

      {loading ? (
        <p>Cargando…</p>
      ) : (
        <>
          <ul className="list">
            {tasks.map((t) => (
              <li key={t.id} className={t.done ? "done" : ""}>
                <label>
                  <input type="checkbox" checked={t.done} onChange={() => toggle(t.id)} />
                  <span>{t.title}</span>
                </label>
                <button className="delete" onClick={() => remove(t.id)} aria-label="Eliminar">
                  ✕
                </button>
              </li>
            ))}
          </ul>
          <p className="footer">
            {pending} pendiente(s) de {tasks.length}
          </p>
        </>
      )}
    </main>
  );
}
