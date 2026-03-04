package todo

import (
	"time"
)

// Todo 代表一个待办事项。
type Todo struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Completed   bool      `json:"completed"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Store 定义了待办事项的存储接口。
type Store interface {
	Create(todo *Todo) error
	Get(id string) (*Todo, error)
	List() ([]*Todo, error)
	Update(todo *Todo) error
	Delete(id string) error
}
