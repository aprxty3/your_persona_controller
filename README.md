# controller-api

Backend API (Go) untuk **Your Persona's** — platform AI-powered psychological assessment.

## Stack

Go · PostgreSQL (GORM) · Redis · Asynq · Gemini API · Cloudflare R2 · Clean Architecture + DDD

## Menjalankan Lokal

```
cp .env.example .env
docker compose -f docker/docker-compose.yml -f docker/docker-compose.dev.yml up
go run ./cmd/migrate
go run ./cmd/api      # atau: air, untuk hot-reload
go run ./cmd/worker
```

- Mailpit (email dev catcher): http://localhost:8025
- MinIO console (S3-compatible dev storage): http://localhost:9001

## Testing

```
go test ./...
```

## Dokumentasi

Aturan arsitektur & konvensi untuk kontribusi (termasuk untuk AI coding agent) ada di [`AGENTS.md`](./AGENTS.md). API specification, job spec, dan testing strategy lengkap ada di [`TECHNICAL_DOCUMENTATION.md`](./TECHNICAL_DOCUMENTATION.md). Spesifikasi produk lengkap dikelola di repo terpisah — hubungi maintainer kalau butuh akses.

## License

All Rights Reserved — lihat [`LICENSE`](./LICENSE). Repo ini publik untuk keperluan portofolio/demonstrasi, bukan untuk digunakan ulang tanpa izin.
