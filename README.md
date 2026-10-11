# Gloaming

A local service: a queue of planned watches and a plan for the free time you have.

This is part one. One program serves the pages and the JSON API.

```bash
go test ./...
go run ./cmd/queue
```

Open http://127.0.0.1:8080. The root path leads to the queue. Search reads series from TVMaze and films from Wikidata. The database `queue.db` is created next to the program. For development, `SLATE_DB` sets the database path and `SLATE_ADDR` sets the address.
