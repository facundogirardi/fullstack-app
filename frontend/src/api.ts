export interface Task {
  id: number;
  title: string;
  done: boolean;
}

async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await fetch(url, {
    headers: { "Content-Type": "application/json" },
    ...options,
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error ?? `Error ${res.status}`);
  }
  return (res.status === 204 ? undefined : await res.json()) as T;
}

export const api = {
  list: () => request<Task[]>("/api/tasks"),
  create: (title: string) =>
    request<Task>("/api/tasks", { method: "POST", body: JSON.stringify({ title }) }),
  toggle: (id: number) => request<Task>(`/api/tasks/${id}/toggle`, { method: "PATCH" }),
  remove: (id: number) => request<void>(`/api/tasks/${id}`, { method: "DELETE" }),
};
