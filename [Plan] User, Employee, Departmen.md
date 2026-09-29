# [Plan] User, Employee, Departmen

## Tujuan dan alur pengerjaan

Tambahkan CRUD user, employee, department, login, dan informasi user login pada layanan Go, menggunakan database PostgreSQL yang sama dengan Spring.

Referensi: https://github.com/ddiandrab/employee-attendance-be-spring dan proyek lokal `/Users/ddiandrab/git-belajar/employee-attendance-be-spring`.

- Dokumen ini dikirim melalui branch `docs/user-employee-plan` dan PR dokumen terpisah ke `main`, serta disalin menjadi GitHub Issue.
- PR Attendance #2 tetap terpisah. Implementasi fitur dimulai setelah PR Attendance direview dan di-merge, mengikuti batas pengerjaan sebelumnya.
- Implementasi nantinya memakai branch `feat/user-employee` dari `main` terbaru dan PR tersendiri.

## API dan hak akses

| API | Hak akses |
| --- | --- |
| `POST /auth/login` | Publik |
| `GET /auth/me` | Semua user terautentikasi |
| `POST /users`, `GET /users` | ADMIN |
| `GET /users/{id}`, `PATCH /users/{id}`, `DELETE /users/{id}` | ADMIN |
| `POST /employees`, `GET /employees` | ADMIN, HR |
| `GET /employees/{id}`, `PATCH /employees/{id}`, `DELETE /employees/{id}` | ADMIN, HR |
| `GET /employees/me`, `PATCH /employees/me` | ADMIN, HR, EMPLOYEE; profil sendiri |
| `GET /departments`, `GET /departments/{id}` | ADMIN, HR, EMPLOYEE |
| `POST /departments`, `PATCH /departments/{id}`, `DELETE /departments/{id}` | ADMIN |

Respons create memakai `201`, read/update/login `200`, dan delete `204`. Daftar berupa array dengan urutan ID menaik, tanpa pagination pada tahap ini. Error mengikuti format Go yang sudah tersedia: `timestamp`, `status`, `error`, `message`, `path`.

CRUD user dan endpoint detail/update/delete employee merupakan penambahan atas API Spring saat rencana disusun. Akses baca department oleh HR juga merupakan perubahan yang disepakati.

## Implementasi dan aturan data

### Struktur dan database

- Pertahankan pola handler → service → repository menggunakan `net/http` dan `pgx`.
- Gunakan tabel `"user"`, `employee`, dan `department` beserta kolom camelCase aktual. Tidak membuat database baru atau menjalankan migrasi otomatis.
- Pertahankan seluruh endpoint Attendance. Perluas konfigurasi CORS untuk `PATCH` dan `DELETE`.
- Gunakan transaksi untuk perubahan yang menyentuh user dan employee sekaligus. Konflik uniqueness atau relasi database dikembalikan sebagai `409`; resource tidak ditemukan sebagai `404`.

### Auth dan user

- `POST /auth/login` menerima `email` dan `password`, memverifikasi hash Argon2id yang sudah tersimpan di Spring, lalu mengembalikan `{"accessToken":"...","tokenType":"Bearer"}`.
- Hash password baru menggunakan Argon2id kompatibel Spring: versi 19, memory 65536 KiB, iteration 3, parallelism 4, salt acak 16 byte, hash 32 byte. Parser menerima urutan parameter hash yang berbeda dan membatasi biaya verifikasi.
- JWT menggunakan secret yang sama dengan Spring, HS256, subject email, claim role, `iat`, dan masa berlaku satu jam. Verifikasi token Spring HS256/HS384/HS512 tetap didukung; otorisasi selalu membaca role terbaru dari database.
- `GET /auth/me` mengembalikan `id`, `email`, dan `role`, tanpa memerlukan profil employee.
- Create user menerima `email`, `password`, dan `role`; role hanya `ADMIN`, `HR`, atau `EMPLOYEE`. PATCH menerima perubahan parsial ketiga field tersebut.
- Respons user hanya berisi `id`, `email`, `role`, `createdAt`, dan `updatedAt`. Password maupun hash tidak boleh muncul pada respons atau log.
- Perubahan email user menyinkronkan email employee terkait dalam transaksi yang sama. Perubahan password tidak mencabut JWT yang sudah terbit; token tetap mengikuti masa berlaku Spring.
- Login dengan password salah atau email tidak ditemukan menghasilkan pesan generik yang sama dan status `401`. Tidak ada registrasi publik.

### Employee

- Pertahankan bentuk respons Spring, termasuk data profil, `userId`, `departmentId`, `departmentName`, dan timestamps.
- Create employee menerima `employeeNumber`, `firstName`, `email`, `password`, serta field opsional Spring: `lastName`, `phone`, `photoUrl`, `departmentId`, `position`, `joinDate`.
- Create employee membuat user ber-role `EMPLOYEE` dan profil `isActive=true` dalam satu transaksi. Email user yang sudah ada menghasilkan `409`; pengaitan profil ke user lama belum masuk tahap ini.
- PATCH berdasarkan ID menerima field profil tersebut selain password, ditambah `isActive`. `userId` dan role tidak dapat diubah melalui endpoint employee. Perubahan email menyinkronkan user terkait.
- `PATCH /employees/me` hanya menerima `phone` dan `photoUrl`, sesuai Spring. Notifikasi dan event Kafka belum dikirim pada tahap ini.
- `isActive=false` tetap hanya informasi profil, sesuai keputusan pengguna; tidak memblokir login atau token lama.

### Department, validasi, dan delete

- Department memakai `name`, `description`, dan bentuk respons Spring. Nama wajib, maksimal 100 karakter; deskripsi maksimal 255 karakter. Nama duplikat menghasilkan `409`, termasuk pada update.
- Email harus valid; password baru minimal delapan karakter; nomor employee dan nama depan tidak boleh kosong; tanggal memakai `YYYY-MM-DD`; ID harus bilangan positif.
- PATCH mempertahankan field yang tidak dikirim. `null` boleh mengosongkan field nullable; field wajib menolak `null`. Field yang tidak dikenal ditolak `400`.
- DELETE menghapus fisik hanya jika data tidak direferensikan. Employee dengan attendance, user dengan employee/notifikasi, dan department dengan employee menghasilkan `409`.
- Menghapus employee tidak otomatis menghapus user terkait. Tidak ada cascade penghapusan riwayat.
- Validasi dan transaksi harus menjaga duplikasi serta konsistensi relasi saat request bersamaan, tanpa mengubah skema database bersama.

## Pengujian dan kriteria selesai

- [ ] Unit test untuk Argon2id, JWT, validasi, aturan role, PATCH, serta pemetaan error.
- [ ] Integration test pada PostgreSQL terpisah untuk seluruh CRUD, transaksi rollback, sinkronisasi email, konflik duplikasi, relasi delete, dan akses profil sendiri.
- [ ] Uji bahwa HR tidak dapat mengelola user atau menaikkan role melalui endpoint employee; EMPLOYEE tidak dapat membaca/mengubah profil orang lain.
- [ ] Jalankan ulang tes Attendance, `go test -race ./...`, `go vet ./...`, build, dan CI.
- [ ] Perbarui README, inventaris API, serta koleksi Postman dengan variabel `baseUrl` dan token yang diisi otomatis setelah login.

Pengujian penerimaan terakhir dilakukan terhadap aplikasi Go lokal:

1. Kirim `POST http://localhost:8081/auth/login`:

   ```json
   {
     "email": "employee.test@example.com",
     "password": "TestPassword123!"
   }
   ```

2. Harus mendapat `200`, `tokenType: Bearer`, dan `accessToken` yang tidak kosong.
3. Gunakan token tersebut untuk `GET http://localhost:8081/auth/me`.
4. Harus mendapat `200` dengan ID, email `employee.test@example.com`, dan role sesuai database.
5. Catat hasil tanpa memublikasikan token atau hash. Akun uji sudah ada saat inspeksi; jika password tidak cocok, laporkan blocker tanpa reset password.

Fitur baru baru dinyatakan selesai setelah pengujian penerimaan berhasil. Setelah commit, push, dan PR implementasi dibuat, tunggu review pengguna sebelum melanjutkan notifikasi atau audit/Kafka.
