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
- **Avatar & attachment** — upload foto profil user dan lampiran gambar/video ke note. File dienkripsi server-side (AES-256-GCM) untuk mode private. Disimpan di [Cloudflare R2](https://www.cloudflare.com/products/r2/).

## Stack

- Go + [Gin](https://github.com/gin-gonic/gin) — HTTP framework
- [sqlc](https://sqlc.dev/) — generate kode Go type-safe dari SQL (pakai driver `pgx/v5`)
- [Neon](https://neon.tech) — Postgres serverless
- `golang.org/x/crypto` — bcrypt (hash password), argon2 (derive key enkripsi private note)
- `github.com/golang-jwt/jwt/v5` — JWT access token
- [Cloudflare R2](https://www.cloudflare.com/products/r2/) — object storage S3-compatible
- [AWS SDK v2](https://aws.github.io/aws-sdk-go-v2/) — S3 client untuk R2
- `crypto/aes` (GCM) — enkripsi note & attachment private (standard library)
- `crypto/rand` — generate ID & token (standard library)
- [Resend](https://resend.com) — kirim email verifikasi & merge password
- `github.com/ulule/limiter/v3` — rate limiting middleware

## Struktur folder

```
notepad-sharelink/
├── cmd/server/main.go          # entry point
├── internal/
│   ├── authutil/                # JWT manager, hash password (bcrypt), refresh/verification token
│   ├── config/                  # load env var (termasuk R2 & Google OAuth)
│   ├── cryptoutil/              # derive key + encrypt/decrypt
│   ├── fileutil/                # validasi magic bytes file
│   ├── idgen/                   # generator ID slug link
│   ├── oauthutil/               # Google OAuth config & helpers
│   ├── storage/                 # Cloudflare R2 client (S3-compatible)
│   ├── db/
│   │   ├── migrations/          # schema SQL
│   │   ├── query/               # query SQL sumber untuk sqlc generate
│   │   └── sqlc/                # hasil generate sqlc
│   ├── service/                 # business logic (auth, notes, avatar, attachment, mailer, cleaners)
│   ├── handler/                 # HTTP handler (Gin)
│   ├── middleware/              # auth middleware (JWT, rate limiter, verified, logger)
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

Setelah sukses, browser di-redirect ke `GOOGLE_FRONTEND_REDIRECT_URL` dengan cookie refresh_token ter-set. Foto profil Google otomatis di-mirror ke R2.

#### Profile (GET /api/auth/me — butuh login)

```bash
curl localhost:8080/api/auth/me \
  -H "Authorization: Bearer <access_token>"
```

Response:
```json
{
  "id": "abc123",
  "email": "user@example.com",
  "email_verified": true,
  "avatar_url": "https://r2.example.com/avatars/abc123/...",
  "avatar_source": "google",
  "has_password": true,
  "has_google": true
}
```

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

## Avatar (foto profil)

Prefix `/api/users` — semua endpoint butuh `Authorization: Bearer <token>`. Tidak perlu verifikasi email.

### Presign upload avatar

```bash
curl -X POST localhost:8080/api/users/avatar/presign \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <access_token>" \
  -d '{"content_type":"image/jpeg","file_size":500000}'
```

Response:
```json
{
  "upload_url": "https://r2.example.com/...?X-Amz-Signature=...",
  "key": "avatars/user123/1234567890-abc.jpg"
}
```

Client PUT langsung ke `upload_url`, lalu konfirmasi.

### Confirm upload avatar

```bash
curl -X POST localhost:8080/api/users/avatar/confirm \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <access_token>" \
  -d '{"key":"avatars/user123/1234567890-abc.jpg"}'
```

Response:
```json
{
  "avatar_url": "https://r2.example.com/avatars/user123/1234567890-abc.jpg"
}
```

Backend memvalidasi ulang ukuran & magic bytes sebelum menyimpan.

## Attachment (lampiran file)

Endpoint attachment di prefix `/api/notes`:

| Method | Path | Auth | Deskripsi |
|---|---|---|---|
| GET | `/:id/attachments` | Publik | Daftar attachment note |
| POST | `/attachments/:attachmentId/download` | Publik | Download attachment private (butuh password) |
| POST | `/:id/attachments/presign` | Login+Verified | Presigned URL upload (note public) |
| POST | `/:id/attachments/confirm` | Login+Verified | Konfirmasi upload selesai |
| POST | `/:id/attachments/private` | Login+Verified | Upload attachment private (multipart) |
| DELETE | `/attachments/:attachmentId` | Login+Verified | Hapus attachment |

### Flow Public (presigned URL)

```bash
# 1. Presign
curl -X POST localhost:8080/api/notes/<noteId>/attachments/presign \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <access_token>" \
  -d '{"content_type":"image/png","file_size":2000000,"kind":"image"}'

# 2. Client PUT langsung ke upload_url (tidak lewat backend)

# 3. Confirm
curl -X POST localhost:8080/api/notes/<noteId>/attachments/confirm \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <access_token>" \
  -d '{"key":"notes/<noteId>/images/...png","kind":"image"}'
```

### Flow Private (server-side encrypted)

```bash
# Upload
curl -X POST localhost:8080/api/notes/<noteId>/attachments/private \
  -H "Authorization: Bearer <access_token>" \
  -F "file=@foto.jpg" \
  -F "kind=image" \
  -F "password=secret123"

# Download (publik — siapapun dengan password)
curl -X POST localhost:8080/api/notes/attachments/<attachmentId>/download \
  -H "Content-Type: application/json" \
  -d '{"password":"secret123"}' \
  --output foto.jpg
```

## API Notes

Semua endpoint di bawah prefix `/api/notes` membutuhkan **Authorization header** kecuali endpoint publik (GET note, unlock, list attachment, download private). User **harus sudah verifikasi email** untuk create/update/delete/list notes.

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
| `R2_ACCOUNT_ID` | Untuk R2 | Account ID Cloudflare R2 |
| `R2_ACCESS_KEY_ID` | Untuk R2 | S3-compatible access key |
| `R2_SECRET_ACCESS_KEY` | Untuk R2 | S3-compatible secret key |
| `R2_BUCKET_NAME` | Untuk R2 | Nama bucket R2 |
| `R2_PUBLIC_BASE_URL` | Untuk R2 | Public base URL bucket |
| `GOOGLE_CLIENT_ID` | ❌ (Google OAuth) | Dari Google Cloud Console |
| `GOOGLE_CLIENT_SECRET` | ❌ (Google OAuth) | Dari Google Cloud Console |
| `PORT` | ❌ | Default `8080` |
| `APP_ENV` | ❌ | Isi `production` untuk JSON logging & cookie secure |

## Catatan desain & keamanan

- **Verifikasi password note private** dilakukan dengan mencoba dekripsi konten pakai AES-GCM. Kalau password salah, authentication tag gagal → dianggap password salah.
- **Salt & key derivation (private note)**: tiap note private punya salt unik (16 byte), key diturunkan pakai Argon2id.
- **Tiga algoritma hash berbeda**: bcrypt (password login), SHA-256 (refresh & verification token), Argon2id (enkripsi note).
- **Email verification**: user baru harus verifikasi email sebelum bisa mengelola note. Unverified users otomatis dihapus setelah 2 hari.
- **Google OAuth merge**: pengguna Google bisa tambah password via link email (membuktikan kepemilikan inbox). Reverse-nya (pengguna password tautkan Google ID) langsung aman.
- **Avatar mirror Google**: foto profil di-download dari Google lalu di-upload ke R2 (fire-and-forget). Tidak menimpa avatar upload manual.
- **Attachment private**: dienkripsi server-side (AES-256-GCM) dengan key yang sama dengan content note. Metadata tetap publik via List.
- **Validasi file**: magic bytes via `http.DetectContentType`, ukuran via R2 HeadObject, path key diverifikasi per user/note.
- **Rate limiting**: endpoint auth dilindungi rate limiter (10 req/menit per IP).
- **Background cleaners**: session expired/revoked, unverified users, pending password merge dibersihkan periodik.
- **Structured logging**: `log/slog` (JSON di production, text di development).
- **Graceful shutdown**: server menunggu request selesai (timeout 10 detik).
- **Refresh token via HttpOnly cookie**: aman XSS. Mobile pakai endpoint body-based.
- **CORS**: development allow all; production allowlist terbatas.
- **404 handler**: return JSON.
- **Ini MVP/prototype**: belum ada TTL/auto-expire note.

