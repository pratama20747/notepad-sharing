# Notepad Sharelink

MVP notepad yang bisa dibagikan lewat link, dengan dua mode:

- **Public** — tanpa password, siapapun yang punya link bisa lihat, edit, dan hapus. Disimpan **plaintext**.
- **Private** — dibuat dengan password, isi disimpan **terenkripsi** (AES-256-GCM, key diturunkan dari password via Argon2id). Untuk lihat/edit/hapus wajib memasukkan password yang sama.

## Stack

- Go + [Gin](https://github.com/gin-gonic/gin) — HTTP framework
- [sqlc](https://sqlc.dev/) — generate kode Go type-safe dari SQL (pakai driver `pgx/v5`)
- [Neon](https://neon.tech) — Postgres serverless
- `golang.org/x/crypto/argon2` + `crypto/aes` (GCM) — enkripsi mode private

## Struktur folder

```
notepad-sharelink/
├── cmd/server/main.go          # entry point
├── frontend/
│   └── index.html               # SPA frontend (disajikan via backend)
├── internal/
│   ├── config/                  # load env var
│   ├── cryptoutil/              # derive key + encrypt/decrypt
│   ├── idgen/                   # generator ID slug link
│   ├── db/
│   │   ├── migrations/          # schema SQL (dipakai sqlc & migrasi manual)
│   │   ├── query/               # query SQL sumber untuk sqlc generate
│   │   └── sqlc/                # hasil generate sqlc
│   ├── service/                 # business logic
│   ├── handler/                 # HTTP handler (Gin)
│   └── router/                  # route registration
├── sqlc.yaml
├── go.mod
└── .env.example
```

## Cara menjalankan

### 1. Siapkan database Neon


1. Buat project di [neon.tech](https://neon.tech), catat connection string-nya.
2. Jalankan isi `internal/db/migrations/001_create_notes.sql` ke database (lewat Neon SQL editor, `psql`, atau tool migrasi favoritmu). Untuk MVP ini belum dipasang tool migrasi otomatis (mis. `golang-migrate`) — silakan tambahkan kalau mau lebih rapi.

### 2. Install sqlc & generate kode

```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
sqlc generate
```

Perintah ini akan mengisi folder `internal/db/sqlc/` dengan kode Go (struct `Note`, `Queries`, dll) berdasarkan `sqlc.yaml`, schema, dan query yang sudah disiapkan.

### 3. Set environment variable

```bash
cp .env.example .env
# lalu isi DATABASE_URL dengan connection string Neon kamu
```

### 4. Install dependency & jalankan

```bash
go mod tidy
go run ./cmd/server
```

Server jalan di `http://localhost:8080` (atau sesuai `PORT`).

## API

### Create note

```bash
# Mode public (title opsional)
curl -X POST localhost:8080/api/notes \
  -H "Content-Type: application/json" \
  -d '{"mode":"public","title":"Catatan pertama","content":"Halo dunia"}'

# Mode private
curl -X POST localhost:8080/api/notes \
  -H "Content-Type: application/json" \
  -d '{"mode":"private","title":"Rahasia","content":"Rahasia banget","password":"secret123"}'
```

Response:
```json
{"id":"aB3xQ9Kd1mZp","share_url":"/n/aB3xQ9Kd1mZp","mode":"public","title":"Catatan tanpa judul"}
```

### List notes (daftar catatan)

```bash
# Limit & offset opsional, default limit=20, max=100
curl "localhost:8080/api/notes?limit=10&offset=0"
```

Response:
```json
{
  "notes": [
    {
      "id": "aB3xQ9Kd1mZp",
      "mode": "public",
      "title": "Catatan pertama",
      "created_at": "2025-01-01T00:00:00Z",
      "updated_at": "2025-01-01T00:00:00Z"
    },
    {
      "id": "xY7kL2pQ4rWn",
      "mode": "private",
      "title": "Rahasia",
      "created_at": "2025-01-02T00:00:00Z",
      "updated_at": "2025-01-02T00:00:00Z"
    }
  ]
}
```

### Get note

```bash
curl localhost:8080/api/notes/aB3xQ9Kd1mZp
```

Response:
```json
{"id":"aB3xQ9Kd1mZp","mode":"public","title":"Catatan pertama","content":"Halo dunia"}
```

- Mode public → langsung dapat field `title` dan `content`.
- Mode private → dapat `{"id":"...","mode":"private","title":"Rahasia","locked":true}`, harus unlock dulu.

### Unlock note private

```bash
curl -X POST localhost:8080/api/notes/aB3xQ9Kd1mZp/unlock \
  -H "Content-Type: application/json" \
  -d '{"password":"secret123"}'
```

Response:
```json
{"id":"aB3xQ9Kd1mZp","title":"Judul rahasia","content":"Rahasia banget"}
```
> Endpoint ini mengembalikan `title` (plaintext) dan `content` hasil dekripsi, tidak perlu fetch GET lagi.

### Update note

```bash
curl -X PUT localhost:8080/api/notes/aB3xQ9Kd1mZp \
  -H "Content-Type: application/json" \
  -d '{"title":"Judul baru","content":"Isi baru","password":"secret123"}'
```
(`password` diabaikan/tidak perlu untuk note mode public)

Endpoint ini mengupdate `title` dan `content` sekaligus.

### Delete note

```bash
curl -X DELETE localhost:8080/api/notes/aB3xQ9Kd1mZp \
  -H "Content-Type: application/json" \
  -d '{"password":"secret123"}'
```

## Catatan desain & keamanan

- **Tidak ada hash password terpisah.** Untuk mode private, password diverifikasi dengan cara mencoba dekripsi konten yang tersimpan. Kalau salah, AES-GCM authentication tag gagal → dianggap password salah. Ini menyederhanakan skema (tidak perlu kolom `password_hash`) sekaligus memastikan password yang dipakai untuk verifikasi = password yang dipakai untuk decrypt (konsisten).
- **Salt & key derivation**: tiap note private punya salt unik (16 byte random), key diturunkan pakai Argon2id sebelum dipakai AES-256-GCM. Salt disimpan plaintext di DB (memang begitu cara kerja Argon2 — salt tidak perlu dirahasiakan).
- **ID/slug link** memakai 12 karakter acak dari `crypto/rand` (bukan `math/rand`) supaya tidak mudah ditebak — ini penting khususnya untuk note mode public karena ID adalah satu-satunya "kunci" aksesnya.
- **Ini MVP/prototype**: belum ada rate limiting, belum ada TTL/auto-expire note, belum ada logging terstruktur, dan CORS masih `*`. Untuk production, tambahkan itu semua plus pertimbangkan menaikkan parameter Argon2id (memory/time cost) sesuai kapasitas server.
