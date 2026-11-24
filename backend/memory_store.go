package main

import "sync"

// MemoryStore guarda las tareas en memoria. Los datos se pierden al reiniciar.
type MemoryStore struct {
	mu     sync.Mutex
	tasks  map[int]Task
	nextID int
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{tasks: make(map[int]Task), nextID: 1}
}

func (s *MemoryStore) List() ([]Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Task, 0, len(s.tasks))
	for i := 1; i < s.nextID; i++ {
		if t, ok := s.tasks[i]; ok {
			out = append(out, t)
		}
	}
	return out, nil
}

func (s *MemoryStore) Create(title string) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := Task{ID: s.nextID, Title: title}
	s.tasks[t.ID] = t
	s.nextID++
	return t, nil
}

func (s *MemoryStore) Toggle(id int) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return Task{}, ErrNotFound
	}
	t.Done = !t.Done
	s.tasks[id] = t
	return t, nil
}

func (s *MemoryStore) Delete(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tasks[id]; !ok {
		return ErrNotFound
	}
	delete(s.tasks, id)
	return nil
}
