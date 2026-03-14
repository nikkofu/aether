# Aether Web UI

This package contains the lightweight React dashboard used to watch the Aether daemon event stream in real time.

Current scope:

- connect to the daemon SSE endpoint
- show the latest streamed agent and system events
- provide a simple live telemetry view for local development

It is not yet a full control plane UI.

## Prerequisites

- Node.js `20+`
- a running `aetherd` instance

## Development

Install dependencies:

```bash
npm install
```

Start the dev server:

```bash
VITE_AETHERD_URL=http://localhost:8090 npm run dev
```

If `VITE_AETHERD_URL` is not set, the app defaults to `http://localhost:8090`.

Task submission notes:

- choose one of the supported explicit workflow patterns from the task form
- when using `parallel`, you can provide explicit branch `name/task` pairs; leaving every branch task empty falls back to the runtime's built-in default branches

## Production Build

```bash
npm run build
```

## Environment Variables

- `VITE_AETHERD_URL`: base URL for the daemon that serves `/stream`

## Related Backend Endpoints

- `GET /stream` from `cmd/aetherd`
- trace and metrics APIs are served separately by `cmd/observability_api`
