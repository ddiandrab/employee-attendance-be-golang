# Inventaris API referensi Spring

Sumber: [issue #3](https://github.com/ddiandrab/employee-attendance-be-spring/issues/3) dan controller/security pada [proyek Spring](https://github.com/ddiandrab/employee-attendance-be-spring), diperiksa 28 September 2026. Implementasi lokal dipakai untuk mencocokkan kontrak aktual.

## API di luar definisi issue #3

| API Spring yang sudah ada | Hak akses aktual Spring | Status tahap Go ini |
| --- | --- | --- |
| `POST /auth/login` | Publik | Tetap memakai Spring untuk memperoleh JWT |
| `GET /auth/me` | Terautentikasi | Tetap di Spring |
| `GET /employees` | ADMIN, HR | Tetap di Spring |
| `GET /employees/me` | ADMIN, HR, EMPLOYEE | Tetap di Spring |
| `POST /employees` | ADMIN, HR | Tetap di Spring, menyiapkan user + profil untuk attendance |
| `PATCH /employees/me` | ADMIN, HR, EMPLOYEE | Tetap di Spring; terkait notifikasi/audit, di luar tahap A |
| `GET /departments` | ADMIN, EMPLOYEE | Tetap di Spring |
| `GET /departments/{id}` | ADMIN, EMPLOYEE | Tetap di Spring |
| `POST /departments` | ADMIN | Tetap di Spring |
| `PATCH /departments/{id}` | ADMIN | Tetap di Spring |
| `DELETE /departments/{id}` | ADMIN | Tetap di Spring |

Temuan: `Testing.md` Spring masih menyebut POST employee hanya ADMIN, tetapi `SecurityConfig` aktual sudah mengizinkan ADMIN dan HR. GET department aktual tidak mengizinkan HR. Tidak ada controller REST audit pada referensi lokal. Temuan ini dicatat, bukan mengubah akses atau memperluas implementasi Go di PR ini.

## Perbedaan rencana issue dan implementasi aktual

- Empat endpoint attendance sama; tidak ditemukan endpoint attendance tambahan.
- Skema aktual memakai `"attendanceRecord"`, FK `"employeeId"` ke `employee.id`, ID integer, serta timestamp berzona untuk check-in/out. Go mengikuti skema aktual agar dapat berbagi database.
- `GET /attendance` mengembalikan ringkasan employee; `GET /attendance/me` mengembalikan record dengan createdAt/updatedAt.
- Filter tanggal kosong menggunakan awal bulan sampai hari ini di Jakarta, bukan seluruh waktu.
- Go menambahkan validasi rentang terbalik serta operasi atomik untuk request Go bersamaan. Ini mempertahankan status error bisnis 400 seperti Spring.

## Ditunda hingga review dan merge

Porting API auth/employee/department belum dilakukan karena tahap ini memakai layanan dan data Spring yang ada. Modul B (notifikasi) serta C (audit/Kafka) di issue juga belum dikerjakan. Tidak menutup issue Spring #3 karena cakupannya lebih luas daripada PR Attendance ini.
