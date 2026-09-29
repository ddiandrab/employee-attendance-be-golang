# Employee Attendance — Go

Implementasi **Modul A (Attendance)** dari [Spring issue #3](https://github.com/ddiandrab/employee-attendance-be-spring/issues/3), memakai database PostgreSQL dan token login Spring yang sudah ada. Modul notification, audit log, dan Kafka belum diimplementasikan pada tahap ini.

## Menjalankan

Prasyarat: Go 1.27.1+, PostgreSQL dengan skema Spring/NestJS yang sudah tersedia, serta akun dan profil employee yang dibuat melalui Spring.

1. Salin `.env.example` ke `.env`. Isi `DATABASE_URL` dan `JWT_SECRET` sesuai konfigurasi Spring. Secret harus sama persis, minimal 32 byte, tanpa decoding base64. Jika Spring memakai nilai default konfigurasi lokal, salin nilai tersebut secara eksplisit ke `.env`.
2. Jalankan layanan Spring untuk login dan pengelolaan employee. Go tidak menjalankan migrasi atau membuat akun.
3. Ekspor konfigurasi dan jalankan Go:

```sh
set -a
. ./.env
set +a
go run ./cmd/api
```

Go mendengarkan pada `:8081` agar tidak bentrok dengan Spring `:8080`. `.env` tidak dimuat otomatis dan tidak boleh di-commit. Gunakan koneksi PostgreSQL dengan TLS sesuai lingkungan deployment.

Login melalui `POST http://localhost:8080/auth/login` dengan body `{"email":"...","password":"..."}`. Gunakan `accessToken` yang dikembalikan untuk request Go:

```sh
export TOKEN='<accessToken dari Spring>'
curl -X POST http://localhost:8081/attendance/check-in -H "Authorization: Bearer $TOKEN"
curl -X POST http://localhost:8081/attendance/check-out -H "Authorization: Bearer $TOKEN"
curl 'http://localhost:8081/attendance/me?from=2026-09-01&to=2026-09-30' -H "Authorization: Bearer $TOKEN"
# Gunakan token HR/ADMIN:
curl 'http://localhost:8081/attendance?from=2026-09-01&to=2026-09-30' -H "Authorization: Bearer $TOKEN"
```

Arahkan hanya `/attendance` dan `/attendance/*` dari frontend/proxy ke Go. Auth, employee, department, dan endpoint lain tetap menuju Spring. CORS default mengizinkan `http://localhost:5173`; ubah `CORS_ALLOWED_ORIGIN` bila perlu.

## API dan perilaku

| Method | Path | Role | Respons sukses |
| --- | --- | --- | --- |
| POST | `/attendance/check-in` | EMPLOYEE, HR, ADMIN | 200, record hari ini |
| POST | `/attendance/check-out` | EMPLOYEE, HR, ADMIN | 200, record yang diperbarui |
| GET | `/attendance/me` | EMPLOYEE, HR, ADMIN | 200, array record milik user login |
| GET | `/attendance` | HR, ADMIN | 200, array ringkasan seluruh employee |

- Check-in/check-out tidak memerlukan body. Identitas selalu berasal dari JWT `sub` (email), lalu relasi `user` → `employee`; ID dari klien tidak digunakan.
- JWT HMAC HS256/HS384/HS512 diverifikasi dengan ukuran key minimum masing-masing algoritma, signature, expiry wajib, serta `nbf`/`iat` jika ada. Role dibaca ulang dari database setiap request; claim role tidak digunakan untuk otorisasi.
- Tanggal bisnis mengikuti `Asia/Jakarta`, termasuk pergantian bulan/tahun. Waktu check-in/check-out disimpan sebagai timestamp UTC, sesuai implementasi Spring aktual.
- Filter `from`/`to` memakai `YYYY-MM-DD`, inklusif. Parameter kosong memakai awal bulan Jakarta / hari Jakarta saat ini, sesuai Spring. `from > to` ditolak 400. Riwayat diurutkan tanggal terbaru, lalu ID terbaru. Hasil kosong berupa `[]`.
- Check-in ganda, check-out tanpa check-in hari ini, atau check-out ganda menghasilkan 400. Unique constraint dan conditional UPDATE menjaga request Go bersamaan. Check-out tidak menyelesaikan record hari sebelumnya. Bila jam server mundur, waktu check-out dibatasi agar tidak lebih awal dari check-in.
- Profil employee tidak ditemukan: 404. JWT hilang/tidak valid/expired atau user sudah dihapus: 401. Role tidak diizinkan: 403. Metode salah: 405. Kegagalan internal: 500 dengan pesan generik.
- HR/ADMIN dapat membaca seluruh presensi tanpa profil employee; untuk presensi diri sendiri harus memiliki profil, sama seperti Spring.

Record `/attendance/me`, check-in, dan check-out:

```json
{"id":1,"employeeId":10,"attendanceDate":"2026-09-28","checkIn":"2026-09-28T01:00:00Z","checkOut":null,"createdAt":"2026-09-28T01:00:00Z","updatedAt":"2026-09-28T01:00:00Z"}
```

Ringkasan `GET /attendance` berisi `id`, `employeeId`, `employeeNumber`, `employeeName`, `departmentId`, `position`, `attendanceDate`, `checkIn`, `checkOut`. Field department/position/waktu yang belum tersedia berupa `null`. Bentuk respons mengikuti DTO Spring.

Semua error API berupa JSON `timestamp`, `status`, `error`, `message`, `path`.

## Database dan struktur

`net/http` handler → attendance service → PostgreSQL repository (`pgx`). Middleware auth memvalidasi token dan membaca user. Konfigurasi dari environment; startup memeriksa koneksi DB; server memiliki timeout dan graceful shutdown.

Skema dimiliki aplikasi referensi. Go memakai `"user"`, `employee`, dan `"attendanceRecord"`, dengan kolom camelCase bertanda kutip, bukan tabel `attendance_records` yang disebut rencana issue. Ini sengaja mengikuti skema Spring/NestJS aktual untuk berbagi data. Constraint unik `("employeeId", "attendanceDate")` wajib sudah tersedia. Go tidak menambah/mengubah tabel saat startup. Fixture skema di `internal/httpapi/testdata` hanya untuk tes.

Sebelum mengalihkan traffic, pastikan schema tersebut sudah ada dan gunakan `JWT_SECRET` yang sama dengan Spring. Alihkan seluruh traffic attendance ke Go agar perlindungan konkurensi tidak bercampur dengan implementasi check-out Spring yang berbeda. Untuk rollback, arahkan route attendance kembali ke Spring; tidak ada migrasi data.

## Pengujian

```sh
go test ./...
go vet ./...
```

Tes integrasi PostgreSQL akan **skip** jika `TEST_DATABASE_URL` kosong. Untuk menjalankannya pada database uji terpisah:

```sh
docker compose -f compose.test.yml up -d --wait
TEST_DATABASE_URL='postgres://postgres:attendance-test-only@localhost:55432/attendance_test?sslmode=disable' go test -race -count=1 ./...
docker compose -f compose.test.yml down -v
```

Tes membuat schema bernama unik dan menghapus hanya schema itu saat selesai. Jangan arahkan `TEST_DATABASE_URL` ke database produksi. CI menjalankan tes integrasi dengan PostgreSQL terpisah. Cakupan: kompatibilitas bentuk token Spring dan role database, batas hari Jakarta, check-in/out, filter inklusif, isolasi employee, ringkasan staff, error/CORS, serta 20 request bersamaan untuk masing-masing operasi tulis.

Lihat [inventaris API referensi](docs/api-inventory.md) untuk API di luar issue dan batas tahap ini.
