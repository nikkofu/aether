# Todo REST API Demo

This package is a small SQLite-backed CRUD demo included in the repository. It is useful for local API experiments, but it is not the main Aether control-plane service.

## Endpoints

- `GET /api/v1/todos`
- `POST /api/v1/todos`
- `GET /api/v1/todos/:id`
- `PUT /api/v1/todos/:id`
- `PATCH /api/v1/todos/:id`
- `DELETE /api/v1/todos/:id`

## Run

```bash
go run cmd/todo_api/main.go
```

Default listen address:

- `http://localhost:8081`

## Test

```bash
go test ./internal/delivery/todo/...
```
