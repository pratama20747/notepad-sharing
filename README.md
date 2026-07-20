# Notepad Sharelink

MVP notepad yang bisa dibagikan lewat link, dengan dua mode:

- **Public** — tanpa password, siapapun yang punya link bisa lihat. Disimpan **plaintext**.
- **Private** — dibuat dengan password, isi disimpan **terenkripsi** (AES-256-GCM, key diturunkan dari password via Argon2id). Untuk lihat/edit/hapus wajib memasukkan password yang sama.

Fitur tambahan:

- **Autentikasi & session** — register/login pakai email + password, session dikelola via refresh token rotation.
- **Otorisasi per user** — setiap note terikat ke pemilik (user_id). List, update, dan delete hanya oleh pemilik. Share link (GET + unlock) tetap publik.
- **Multi-device** — refresh token disimpan di tabel sessions, bisa login dari banyak device sekaligus.
- **Email verification** — verifikasi email wajib sebelum bisa membuat/mengelola note. Link verifikasi dikirim via [Resend](https://resend.com).
- **Google OAuth** — login dengan akun Google. Pengguna password bisa menautkan akun Google, dan pengguna Google bisa menambahkan password (merge via email verification).

## Stack

- Go + [Gin](https://github.com/gin-gonic/gin) — HTTP framework
- [sqlc](https://sqlc.dev/) — generate kode Go type-safe dari SQL (pakai driver `pgx/v5`)
- [Neon](https://neon.tech) — Postgres serverless
- `golang.org/x/crypto` — bcrypt (hash password), argon2 (derive key enkripsi private note)
- `github.com/golang-jwt/jwt/v5` — JWT access token
- `crypto/aes` (GCM) — enkripsi mode private (standard library)
- `crypto/rand` — generate ID & token (standard library)
- [Resend](https://resend.com) — kirim email verifikasi & merge password
- `github.com/ulule/limiter/v3` — rate limiting middleware

## Struktur folder

```
notepad-sharelink/
├── cmd/server/main.go          # entry point
├── internal/
│   ├── authutil/                # JWT manager, hash password (bcrypt), refresh token, verification token
│   ├── config/                  # load env var
│   ├── cryptoutil/              # derive key + encrypt/decrypt
│   ├── idgen/                   # generator ID slug link
│   ├── db/
│   │   ├── migrations/          # schema SQL (dipakai sqlc & migrasi manual)
│   │   ├── query/               # query SQL sumber untuk sqlc generate
│   │   └── sqlc/                # hasil generate sqlc
│   ├── oauthutil/               # Google OAuth config & helpers
│   ├── service/                 # business logic (auth, notes, mailer, cleaners)
│   ├── handler/                 # HTTP handler (Gin)
│   ├── middleware/              # auth middleware (JWT, rate limiter, verified)
│   └── router/                  # route registration
├── sqlc.yaml
├── go.mod
├── go.sum
├── docker-compose.yaml
└── .env.example
```

## Cara menjalankan

### 1. Siapkan database

**Opsi A: Neon (Serverless Postgres)**

1. Buat project di [neon.tech](https://neon.tech), catat connection string-nya.
2. Jalankan isi `internal/db/migrations/001_create_notes.sql` ke database.

**Opsi B: Docker (Lokal)**

```bash
docker-compose up -d
```

Lalu jalankan migrasi:

```bash
psql postgres://myuser:mypassword@localhost:5432/mydatabase -f internal/db/migrations/001_create_notes.sql
```

### 2. Install sqlc & generate kode

```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
sqlc generate
```

### 3. Set environment variable

```bash
cp .env.example .env
# lalu isi semua env var yang dibutuhkan (lihat .env.example)
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

**Mobile (Flutter)**: Karena Flutter tidak bisa mengelola HttpOnly cookie secara native, ada endpoint terpisah yang menerima refresh token via body:

```
/login-mobile  → { access_token, refresh_token }
/refresh-mobile → body: { refresh_token } → { access_token, refresh_token }
/logout-mobile  → body: { refresh_token }
```

### API Auth

> **Catatan untuk client web**: refresh token dikirim/dibaca otomatis via HttpOnly cookie oleh browser.

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

Refresh token di-set sebagai HttpOnly cookie. Email verifikasi dikirim ke inbox.

#### Login

```bash
curl -X POST localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"rahasia123"}'
```

Response — access_token di body, refresh token di cookie.

#### Login dengan Google

Redirect browser ke endpoint berikut (state disimpan di short-lived cookie untuk CSRF protection):

```bash
curl -v -X GET localhost:8080/api/auth/google/login
```

Setelah sukses, browser di-redirect ke `GOOGLE_FRONTEND_REDIRECT_URL` dengan cookie refresh_token ter-set.

#### Refresh token

```bash
curl -X POST localhost:8080/api/auth/refresh
# refresh token otomatis terkirim via cookie
```

Response — access_token baru di body, refresh token baru di cookie.

#### Logout

```bash
curl -X POST localhost:8080/api/auth/logout
# refresh token dibaca dari cookie
```

Session di-revoke, cookie dihapus. Device lain tidak terpengaruh.

#### Verifikasi email

```bash
curl -X GET "localhost:8080/api/auth/verify-email?token=xxx"
```

Endpoint publik — user klik link dari email.

#### Kirim ulang verifikasi (butuh login)

```bash
curl -X POST localhost:8080/api/auth/resend-verification \
  -H "Authorization: Bearer <access_token>"
```

#### Verifikasi tautan password Google (dari email merge)

```bash
curl -X GET "localhost:8080/api/auth/verify-merge-password?token=xxx"
```

Endpoint publik — user klik link dari email merge. Password pending diaktifkan.

### Mobile API (Flutter)

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

#### Logout mobile

```bash
curl -X POST localhost:8080/api/auth/logout-mobile \
  -H "Content-Type: application/json" \
  -d '{"refresh_token":"a1b2c3d4e5f6..."}'
```

### Keamanan password (login)

- Password di-hash pakai **bcrypt** (cost default = 10) — setiap hash mengandung salt acak 16 byte.
- Verifikasi: server mencari user by email, lalu membandingkan input password dengan hash dari baris yang sama.
- Bcrypt berbeda dengan Argon2id yang dipakai untuk enkripsi note private.

### Refresh token vs Access token

| Token | Umur | Penyimpanan DB | Cara hash |
|---|---|---|---|
| Access token (JWT) | 15 menit | Tidak | HMAC-SHA256 (secret server) |
| Refresh token | 30 hari | Hash SHA-256 di tabel `sessions` | SHA-256 (token random 32 byte) |

## API Notes

Semua endpoint di bawah prefix `/api/notes` membutuhkan **Authorization header** kecuali:

- `GET /api/notes/:id` — share link publik
- `POST /api/notes/:id/unlock` — unlock via password (publik)

Selain itu, user **harus sudah verifikasi email** untuk bisa create/update/delete/list notes (dicek oleh `RequireVerified` middleware).

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

> Hanya menampilkan note milik user yang sedang login (isolasi per user).

### Get note (share link — publik)

```bash
curl localhost:8080/api/notes/aB3xQ9Kd1mZp
```

- Mode public → langsung dapat `title` dan `content`.
- Mode private → `{"locked":true}`, harus unlock dulu.

### Unlock note private (publik)

```bash
curl -X POST localhost:8080/api/notes/aB3xQ9Kd1mZp/unlock \
  -H "Content-Type: application/json" \
  -d '{"password":"secret123"}'
```

### Update note (hanya pemilik, verified)

```bash
curl -X PUT localhost:8080/api/notes/aB3xQ9Kd1mZp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <access_token>" \
  -d '{"title":"Judul baru","content":"Isi baru","password":"secret123"}'
```

### Delete note (hanya pemilik, verified)

```bash
curl -X DELETE localhost:8080/api/notes/aB3xQ9Kd1mZp \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <access_token>" \
  -d '{"password":"secret123"}'
```

## Environment Variables

Lihat `.env.example` untuk daftar lengkap.

| Variable | Wajib | Keterangan |
|---|---|---|
| `DATABASE_URL` | ✅ | Connection string Neon atau PostgreSQL |
| `JWT_SECRET` | ✅ | Secret key JWT (min 32 karakter) |
| `RESEND_API_KEY` | ✅ | API key dari Resend.com |
| `GOOGLE_CLIENT_ID` | ❌ (untuk Google OAuth) | Dari Google Cloud Console |
| `GOOGLE_CLIENT_SECRET` | ❌ (untuk Google OAuth) | Dari Google Cloud Console |
| `PORT` | ❌ | Default `8080` |
| `FROM_EMAIL` | ❌ | Default `onboarding@resend.dev` |
| `BASE_URL` | ❌ | Default `http://localhost:{PORT}` |
| `APP_ENV` | ❌ | Isi `production` untuk JSON logging & cookie secure |

## Catatan desain & keamanan

- **Verifikasi password note private** dilakukan dengan mencoba dekripsi konten pakai AES-GCM. Kalau password salah, authentication tag gagal → dianggap password salah.
- **Salt & key derivation (private note)**: tiap note private punya salt unik (16 byte), key diturunkan pakai Argon2id.
- **Tiga algoritma hash berbeda**: bcrypt (password login), SHA-256 (refresh & verification token), Argon2id (enkripsi note).
- **Email verification**: user baru harus verifikasi email sebelum bisa mengelola note. Unverified users otomatis dihapus setelah 2 hari.
- **Google OAuth merge**: pengguna Google bisa tambah password via link email (membuktikan kepemilikan inbox). Reverse-nya (pengguna password tautkan Google ID) langsung aman karena Google sudah buktikan email.
- **Rate limiting**: endpoint auth dilindungi rate limiter (10 req/menit per IP).
- **Background cleaners**: session expired/revoked, unverified users, dan pending password merge dibersihkan periodik oleh goroutine latar.
- **Structured logging**: `log/slog` (JSON di production, text di development).
- **Graceful shutdown**: server menunggu request selesai (timeout 10 detik) sebelum berhenti.
- **Refresh token via HttpOnly cookie**: aman dari XSS untuk client web. Mobile pakai endpoint body-based.
- **CORS**: development allow all origins; production allowlist terbatas dengan credentials.
- **404 handler**: return JSON, bukan serve index.html.
- **Ini MVP/prototype**: belum ada TTL/auto-expire note.
```

## 3. **Docker Compose - Tambahkan Volume untuk Init**

```yaml
version: "3.8"

services:
  postgres:
    image: postgres:16.3-alpine
    container_name: postgres_db
    restart: unless-stopped
    environment:
      POSTGRES_USER: myuser
      POSTGRES_PASSWORD: mypassword
      POSTGRES_DB: mydatabase
      PGDATA: /var/lib/postgresql/data/pgdata
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - postgres_backup:/backup
      # Auto-run migration saat container pertama kali dibuat
      - ./internal/db/migrations:/docker-entrypoint-initdb.d
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U myuser -d mydatabase"]
      interval: 10s
      timeout: 5s
      retries: 5

volumes:
  postgres_data:
    driver: local
  postgres_backup:
    driver: local
```

## 4. **Buat `.env.example` yang Hilang**

Buat file `.env.example`:

```bash
# Database
DATABASE_URL=postgres://myuser:mypassword@localhost:5432/mydatabase?sslmode=disable

# JWT
JWT_SECRET=your-secret-key-at-least-32-characters-long-here

# Server
PORT=8080
APP_ENV=development

# Resend (Email)
RESEND_API_KEY=re_xxxxxxxxxxxx
FROM_EMAIL=onboarding@resend.dev
BASE_URL=http://localhost:8080

# Google OAuth (Opsional)
GOOGLE_CLIENT_ID=your-google-client-id.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=GOCSPX-xxxxxxxxxxxx
GOOGLE_REDIRECT_URL=http://localhost:8080/api/auth/google/callback
GOOGLE_FRONTEND_REDIRECT_URL=http://localhost:3000
```

## 5. **Update `.gitignore`**

Pastikan file `.gitignore` ada:

```gitignore
# Binaries
*.exe
*.exe~
*.dll
*.so
*.dylib
notepad-sharelink

# Test binary
*.test

# Output of the go coverage tool
*.out

# Dependency directories
vendor/

# Go workspace file
go.work

# Environment variables
.env

# IDE
.idea/
.vscode/
*.swp
*.swo
*~

# OS
.DS_Store
Thumbs.db

# Database
*.db
*.sqlite

# Docker volumes
postgres_data/
postgres_backup/
```
