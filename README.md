# Notepad Sharelink

MVP notepad yang bisa dibagikan lewat link, dengan dua mode:

- **Public** — tanpa password, siapapun yang punya link bisa lihat. Disimpan **plaintext**.
- **Private** — dibuat dengan password, isi disimpan **terenkripsi** (AES-256-GCM, key diturunkan dari password via Argon2id). Untuk lihat/edit/hapus wajib memasukkan password yang sama.

Fitur tambahan:

- **Autentikasi & session** — register/login pakai email + password, session dikelola via refresh token rotation.
- **Otorisasi per user** — setiap note terikat ke pemilik (user_id). List, update, dan delete hanya oleh pemilik. Share link (GET + unlock) tetap publik.
- **Multi-device** — refresh token disimpan di tabel sessions, bisa login dari banyak device sekaligus.

## Stack

- Go + [Gin](https://github.com/gin-gonic/gin) — HTTP framework
- [sqlc](https://sqlc.dev/) — generate kode Go type-safe dari SQL (pakai driver `pgx/v5`)
- [Neon](https://neon.tech) — Postgres serverless
- `golang.org/x/crypto` — bcrypt (hash password), argon2 (derive key enkripsi private note)
- `github.com/golang-jwt/jwt/v5` — JWT access token
- `crypto/aes` (GCM) — enkripsi mode private

## Struktur folder

```
notepad-sharelink/
├── cmd/server/main.go          # entry point
├── frontend/
│   └── index.html               # SPA frontend (disajikan via backend)
├── internal/
│   ├── authutil/                # JWT manager, hash password (bcrypt), refresh token
│   ├── config/                  # load env var
│   ├── cryptoutil/              # derive key + encrypt/decrypt
│   ├── idgen/                   # generator ID slug link
│   ├── db/
│   │   ├── migrations/          # schema SQL (dipakai sqlc & migrasi manual)
│   │   ├── query/               # query SQL sumber untuk sqlc generate
│   │   └── sqlc/                # hasil generate sqlc
│   ├── service/                 # business logic (auth + notes)
│   ├── handler/                 # HTTP handler (Gin)
│   ├── middleware/              # auth middleware (JWT validation)
│   └── router/                  # route registration
├── sqlc.yaml
├── go.mod
└── .env.example
```

## Cara menjalankan

### 1. Siapkan database Neon

1. Buat project di [neon.tech](https://neon.tech), catat connection string-nya.
2. Jalankan isi `internal/db/migrations/001_create_notes.sql` ke database (lewat Neon SQL editor, `psql`, atau tool migrasi favoritmu). Untuk MVP ini belum dipasang tool migrasi otomatis — silakan tambahkan kalau mau lebih rapi.

### 2. Install sqlc & generate kode

```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
sqlc generate
```

Perintah ini akan mengisi folder `internal/db/sqlc/` dengan kode Go (struct tabel, Queries, dll) berdasarkan `sqlc.yaml`, schema, dan query yang sudah disiapkan.

### 3. Set environment variable

```bash
cp .env.example .env
# lalu isi:
#   DATABASE_URL   — connection string Neon kamu
#   JWT_SECRET     — secret key untuk menandatangani access token (min. 32 karakter random)
```

### 4. Install dependency & jalankan

```bash
go mod tidy
go run ./cmd/server
```

Server jalan di `http://localhost:8080` (atau sesuai `PORT`).

## Autentikasi

Semua endpoint note (kecuali GetNote & Unlock) membutuhkan access token JWT yang dikirim via header:

```
Authorization: Bearer <access_token>
```

### Flow

```
Register/Login
  ↓
Server set refresh_token di HttpOnly cookie (tidak bisa diakses JS)
Response body: { access_token }
  ↓
Client kirim access_token di setiap request (Authorization: Bearer <token>).
Jika access_token expired (401), client minta refresh:
  POST /api/auth/refresh  (refresh token otomatis terkirim via cookie)
  → Server set cookie baru + response access_token baru.
  ↓
Refresh token lama langsung di-revoke (rotation).
Jika refresh token juga expired/invalid → client harus login ulang.
```

**Mobile (Flutter)**: Karena Flutter tidak bisa mengelola HttpOnly cookie secara native,
ada endpoint terpisah yang menerima refresh token via body:

```
/login-mobile  → { access_token, refresh_token }  (body response)
/refresh-mobile → body: { refresh_token } → { access_token, refresh_token }
/logout-mobile  → body: { refresh_token }
```

### API Auth

> **Catatan untuk client web**: refresh token dikirim/dibaca otomatis via HttpOnly
> cookie oleh browser. Client tidak perlu menyimpan atau mengirim refresh token
> secara manual. Untuk mobile (Flutter), lihat bagian [Mobile API](#mobile-api-flutter).

#### Register

```bash
curl -X POST localhost:8080/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"rahasia123"}'
```

Response:
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIs..."
}
```
Refresh token otomatis di-set sebagai HttpOnly cookie → tidak perlu dikelola manual.

#### Login

```bash
curl -X POST localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"rahasia123"}'
```

Response — sama seperti register (hanya access_token di body, refresh token di cookie).

#### Refresh token

```bash
curl -X POST localhost:8080/api/auth/refresh
# no body needed — refresh token otomatis terkirim via cookie
```

Response — access_token baru di body, refresh token baru di cookie. Token lama di-revoke.

#### Logout

```bash
curl -X POST localhost:8080/api/auth/logout
# no body needed — refresh token dibaca dari cookie
```

Hanya session dengan refresh token tersebut yang di-revoke, device lain tidak terpengaruh. Cookie langsung dihapus.

### Mobile API (Flutter)

Untuk mobile client yang tidak bisa mengelola HttpOnly cookie, gunakan endpoint
terpisah — refresh token dikirim dan dikembalikan via body.

#### Login mobile

```bash
curl -X POST localhost:8080/api/auth/login-mobile \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"rahasia123"}'
```

Response:
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIs...",
  "refresh_token": "a1b2c3d4e5f6..."
}
```

#### Refresh mobile

```bash
curl -X POST localhost:8080/api/auth/refresh-mobile \
  -H "Content-Type: application/json" \
  -d '{"refresh_token":"a1b2c3d4e5f6..."}'
```

Response — TokenPair baru. Token lama di-revoke.

#### Logout mobile

```bash
curl -X POST localhost:8080/api/auth/logout-mobile \
  -H "Content-Type: application/json" \
  -d '{"refresh_token":"a1b2c3d4e5f6..."}'
```

Hanya session dengan refresh token tersebut yang di-revoke.

### Keamanan password (login)

- Password di-hash pakai **bcrypt** (cost default = 10) — setiap hash mengandung salt acak 16 byte, sehingga hash untuk password yang sama selalu berbeda.
- Verifikasi: server mencari user by email, lalu membandingkan input password dengan hash dari baris yang sama (`bcrypt.CompareHashAndPassword`). Binding ke user terjadi **via query**, bukan di hash-nya.
- Bcrypt berbeda dengan Argon2id yang dipakai untuk enkripsi note private — bcrypt untuk hash password login, Argon2id untuk derive key enkripsi AES-GCM.

### Refresh token vs Access token

| Token | Umur | Penyimpanan DB | Cara hash |
|---|---|---|---|
| Access token (JWT) | 15 menit | Tidak | HMAC-SHA256 (secret server) |
| Refresh token | 30 hari | Hash SHA-256 di tabel `sessions` | SHA-256 (cepat, karena token sudah random 32 byte) |

Refresh token asli (32 byte random) tidak pernah disimpan — hanya hash SHA-256-nya. Karena entropinya tinggi (256 bit), SHA-256 sudah aman tanpa perlu bcrypt/Argon2.

## API Notes

Semua endpoint di bawah prefix `/api/notes` membutuhkan **Authorization header** kecuali:

- `GET /api/notes/:id` — share link publik
- `POST /api/notes/:id/unlock` — unlock via password (publik)

Title selalu plaintext (TEXT di DB) — judul note private tetap terlihat di daftar & saat share link.

### Create note

```bash
curl -X POST localhost:8080/api/notes \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <access_token>" \
  -d '{"mode":"public","title":"Catatan pertama","content":"Halo dunia"}'

# Mode private
curl -X POST localhost:8080/api/notes \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <access_token>" \
  -d '{"mode":"private","title":"Rahasia","content":"Rahasia banget","password":"secret123"}'
```

Response:
```json
{"id":"aB3xQ9Kd1mZp","share_url":"/n/aB3xQ9Kd1mZp","mode":"public","title":"Catatan tanpa judul"}
```

### List notes (milik user yang login)

```bash
curl "localhost:8080/api/notes?limit=10&offset=0" \
  -H "Authorization: Bearer <access_token>"
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
    }
  ]
}
```

> Hanya menampilkan note milik user yang sedang login (isolasi per user).

### Get note (share link — publik)

```bash
curl localhost:8080/api/notes/aB3xQ9Kd1mZp
```

Response:
```json
{"id":"aB3xQ9Kd1mZp","mode":"public","title":"Catatan pertama","content":"Halo dunia"}
```

- Mode public → langsung dapat `title` dan `content`.
- Mode private → `{"id":"...","mode":"private","title":"\U0001F512 Catatan privat","locked":true}`, harus unlock dulu.

### Unlock note private (publik)

```bash
curl -X POST localhost:8080/api/notes/aB3xQ9Kd1mZp/unlock \
  -H "Content-Type: application/json" \
  -d '{"password":"secret123"}'
```

Response:
```json
{"id":"aB3xQ9Kd1mZp","title":"Judul rahasia","content":"Rahasia banget"}
```

### Update note (hanya pemilik)

```bash
curl -X PUT localhost:8080/api/notes/aB3xQ9Kd1mZp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <access_token>" \
  -d '{"title":"Judul baru","content":"Isi baru","password":"secret123"}'
```

(`password` diabaikan untuk note mode public.)

Hanya pemilik note (userID dari access token cocok dengan `notes.user_id`) yang boleh update.

### Delete note (hanya pemilik)

```bash
curl -X DELETE localhost:8080/api/notes/aB3xQ9Kd1mZp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <access_token>" \
  -d '{"password":"secret123"}'
```

Hanya pemilik note yang boleh delete.

## Catatan desain & keamanan

- **Verifikasi password note private** dilakukan dengan mencoba dekripsi konten yang tersimpan pakai AES-GCM. Kalau password salah, authentication tag gagal → dianggap password salah. Ini menyederhanakan skema (tidak perlu kolom `password_hash` terpisah) sekaligus memastikan password verifikasi = password dekripsi.
- **Salt & key derivation (private note)**: tiap note private punya salt unik (16 byte random), key diturunkan pakai Argon2id sebelum dipakai AES-256-GCM. Salt disimpan plaintext di DB (begitulah cara kerja Argon2 — salt tidak perlu dirahasiakan).
- **Dua algoritma hash berbeda**: bcrypt untuk hash password login (karena low-entropy), SHA-256 untuk hash refresh token (karena token sudah random 256-bit), Argon2id untuk derive key enkripsi note (karena butuh KDF).
- **ID/slug link** memakai 12 karakter acak dari `crypto/rand` (bukan `math/rand`) supaya tidak mudah ditebak — penting untuk share link publik.
- **Rate limiting**: endpoint auth dilindungi rate limiter (10 request/menit per IP) untuk menghambat brute force.
- **Session cleanup**: session expired/revoked secara otomatis dihapus dari database setiap jam oleh background goroutine.
- **Structured logging**: menggunakan `slog` (standard library sejak Go 1.21). JSON logging otomatis diaktifkan saat `APP_ENV=production`.
- **Graceful shutdown**: server menerima SIGINT/SIGTERM, menunggu request selesai dalam batas waktu 10 detik (via `http.Server.Shutdown()`), lalu berhenti.
- **Refresh token via HttpOnly cookie**: untuk client web, refresh token tidak bisa diakses JavaScript sehingga aman dari XSS. Untuk mobile, ada endpoint terpisah (body-based).
- **CORS** — di development pakai `*`. Di production hanya allowlist tertentu (`Access-Control-Allow-Credentials: true` aktif).
- **404 handler**: route yang tidak dikenal mengembalikan JSON `{"error":"Not found"}`, bukan serve index.html.
- **Ini MVP/prototype**: belum ada TTL/auto-expire note. Untuk production, tambahkan itu.
