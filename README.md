# Gloaming

A local service: a queue of planned watches and a plan for the free time you have.

This is part one. One program serves the pages and the JSON API.

```bash
go test ./...
go run ./cmd/queue
```

Open http://127.0.0.1:8080. The root path leads to the queue. Search reads series from TVMaze and films from Wikidata. A series is queued by season, and the title page can add every season in order, skipping seasons already in the queue. Search waits longer for a film when the series list comes back first, and looks films up in Russian and English. A title page shows the synopsis and each season's poster. Search can narrow the hits already on the page by genre. The plan counts only the episodes still left to watch. Minutes per day, above that plan, show when the current season ends and when the whole queue ends. If the catalog has no match, the search page saves your own card: year, episode count, and minutes per episode. A film is one episode. That card stays on your account. Successful catalog responses are cached in Redis for twelve hours. A series whose episode list is still loading is marked assembling and filled in afterwards. Marking an episode publishes a Kafka event. Separate consumers store the pace, a Thursday letter when the current season ends that day, and genre picks from the titles already in the queue. Posters are copied onto this server and served from `/posters/`. Registration asks for an email address. An existing account can save one later on the queue page. A welcome letter and a password-reset link go out by SMTP, and the Thursday letter is sent to that address. The session cookie is marked Secure only on HTTPS. Caddy serves that HTTPS on port 8443 with its local certificate. Port 8088 stays plain HTTP, and so does the app on 8080. Login, registration, password reset, and search are rate-limited when Redis is up. `GET /metrics` exposes request counts for Prometheus.

PostgreSQL has to be running. The default address is `postgres://127.0.0.1/gloaming?sslmode=disable`. The program creates the `gloaming` database on first launch when it can. `GLOAMING_DSN` overrides the address. `SLATE_ADDR` still sets the listen address, and `SLATE_CATALOG` still sets the series catalog. `GLOAMING_REDIS` defaults to `redis://127.0.0.1:6379/0`. Set it to `off` to run without the cache. `GLOAMING_KAFKA` defaults to `127.0.0.1:9092`. Set it to `off` to keep the catalog worker inside the process. `GLOAMING_POSTERS` is the directory for copied posters and defaults to `posters`. `GLOAMING_SMTP` defaults to `127.0.0.1:1025`; set it to `off` to skip mail. `GLOAMING_MAIL_FROM` defaults to `gloaming@localhost`. `GLOAMING_PUBLIC_URL` is the link written into reset mail and defaults to `http://127.0.0.1:8080`. `GLOAMING_SECURE=1` forces the Secure cookie even on plain HTTP.

```bash
docker compose up -d
GLOAMING_DSN=postgres://gloaming:gloaming@127.0.0.1:5432/gloaming?sslmode=disable go run ./cmd/queue
```
