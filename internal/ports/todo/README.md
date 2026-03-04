# Todo REST API

这是一个简单的待办事项 REST API，使用 Go 语言和 SQLite 数据库实现。

## 功能

- 获取所有待办事项：`GET /api/v1/todos`
- 创建新的待办事项：`POST /api/v1/todos`
- 获取单个待办事项：`GET /api/v1/todos/:id`
- 更新待办事项：`PUT /api/v1/todos/:id`
- 部分更新待办事项：`PATCH /api/v1/todos/:id`
- 删除待办事项：`DELETE /api/v1/todos/:id`

## 运行方式

1. 确保安装了 Go 1.25.5 或更高版本。
2. 在项目根目录下运行：

   ```bash
   go run cmd/todo_api/main.go
   ```

3. API 将在 `http://localhost:8081` 监听。

## 运行测试

```bash
go test ./internal/todo/...
```
