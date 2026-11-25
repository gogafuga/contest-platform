package tasks

import (
	"devsec-contest01/internal/tasks/model"

	"devsec-contest01/internal/tasks/task_01"
	"devsec-contest01/internal/tasks/task_02"
	"devsec-contest01/internal/tasks/task_03"
)

type Registry struct {
	tasks map[string]*model.Task
	order []string
}

func NewRegistry() *Registry {
	list := []*model.Task{
		task_01.New(),
		task_02.New(),
		task_03.New(),
	}

	r := &Registry{
		tasks: make(map[string]*model.Task, len(list)),
		order: make([]string, 0, len(list)),
	}
	for _, t := range list {
		r.tasks[t.ID] = t
		r.order = append(r.order, t.ID)
	}
	return r
}

func (r *Registry) Get(id string) (*model.Task, bool) {
	t, ok := r.tasks[id]
	return t, ok
}

// List returns every task in canonical contest order.
func (r *Registry) List() []*model.Task {
	out := make([]*model.Task, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.tasks[id])
	}
	return out
}
