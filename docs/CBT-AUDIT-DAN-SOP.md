# Audit CBT RoyClass dan SOP Publikasi Soal

Tanggal audit: 18 September 2026
Target publik: https://lms.royzy.web.id/

Dokumen ini menjelaskan mengapa pembuatan asesmen Bahasa Inggris Kelas 9
memerlukan beberapa kali debugging dan prosedur yang harus dipakai agar soal
baru benar-benar muncul di CBT.

## Ringkasan hasil audit

Masalah utamanya bukan satu bug pada soal Bahasa Inggris. Ada beberapa lapisan
yang belum memiliki satu sumber kebenaran dan belum dipublikasikan melalui satu
pipeline:

~~~
Markdown staging
    -> ai-bot importer
    -> PocketBase (data runtime/API)
    -> Hugo build
    -> /var/www/class.gezytech.web.id/public (yang disajikan nginx)
    -> lms.royzy.web.id
~~~

Repo Git dan instalasi produksi masih berada di lokasi berbeda. Karena itu,
commit atau perubahan pada repo tidak otomatis mengubah runtime produksi.

Pada akhir audit, kondisi live terverifikasi sebagai berikut:

- PocketBase health: HTTP 200.
- Asesmen e31b5ex1n310lck: 20 soal publik.
- Sesi hasil ujian tidak mengirim review dan show_explanation bernilai false.
- Nginx menyajikan lms.royzy.web.id dari
  /var/www/class.gezytech.web.id/public.

## Temuan dan akar masalah

| Temuan | Dampak | Penyebab teknis |
| --- | --- | --- |
| Soal sudah dibuat tetapi belum tampak di laman | Halaman bisa menampilkan kategori kosong | File staging baru masuk ke PocketBase setelah ai-bot sync; laman statis baru berubah setelah Hugo build dan swap public/. |
| Dokumentasi menunjuk ke path lama | Command dijalankan di direktori yang salah | README lama masih memakai /home/pgun/dev/gezy/gezyclass dan domain class.gezytech.web.id, sedangkan runtime yang benar adalah /home/pakgun/royclass dan lms.royzy.web.id. |
| Perubahan repo tidak selalu sama dengan produksi | Perbaikan terlihat di source tetapi tidak live | PocketBase dijalankan dari /var/www/class.gezytech.web.id/pocketbase; Hugo dibuild dari /var/www/class.gezytech.web.id/hugo; tidak ada service/timer deploy Git otomatis. |
| Token dianggap tidak valid atau sudah digunakan | Peserta tidak dapat memulai ujian kedua | Token lama bersifat sekali pakai (is_shared=false) dan sudah memiliki sesi/penggunaan. Token bersama harus diatur eksplisit. |
| Ujian muncul tetapi daftar soal atau relasi tidak benar | Soal tidak terhubung ke exam | question_source_ids harus di-resolve ke record PocketBase sebelum membuat exam_questions. ID PocketBase langsung dan source_id staging adalah dua jenis identifier yang berbeda. |
| Nilai maksimum tidak sesuai | 20 soal dapat menjadi 200, bukan 100 | Importer lama memakai bobot default 10; untuk 20 soal dengan skor maksimum 100 harus memakai points_per_question: 5. |
| show_explanation_after: false gagal disimpan | Sync berhenti dengan HTTP 400 Cannot be blank | Skema PocketBase lama mendeklarasikan boolean tersebut sebagai required. Pada instalasi ini PocketBase menolak nilai boolean false pada update. |
| Klik pilihan jawaban terlihat tidak merespons | Peserta tidak dapat memilih jawaban | Template halaman, format tipe soal, dan API harus selaras. Frontend harus menerima pg, mr/pgk, dan bs, serta menyisipkan ID soal dengan benar pada handler JavaScript. |
| Sync bisa berhenti setelah sebagian perubahan | Database dapat berada pada keadaan setengah tersinkron | Importer belum transaksional. Untuk exam, relasi lama dapat dihapus sebelum semua relasi baru berhasil dibuat. |
| Validasi yang dijanjikan belum lengkap | Kesalahan baru ketahuan saat import atau saat dibuka di browser | Parser saat ini memeriksa front matter dasar, kind, dan source_id, tetapi belum sepenuhnya memeriksa duplikasi ID, semua relasi, jumlah kunci, dan kecocokan bobot. |

## Pelajaran khusus dari asesmen Bahasa Inggris Kelas 9

### Format ringkas yang sekarang didukung

Untuk ujian baru, seluruh soal dapat ditulis di satu file `kind: exam` melalui
field `questions`. Importer akan membuat atau memperbarui record PocketBase
question secara internal dan menghubungkannya ke exam. Jadi penulis tidak perlu
membuat satu file Markdown untuk setiap soal. Lihat contoh praktis di
`content-staging/examples/exam-inline.md.example`.

Format satu-file-per-question tetap dipertahankan hanya untuk kompatibilitas
dengan data lama.

Format yang berhasil dipakai adalah:

~~~yaml
kind: question
source_id: question-kelas9-bahasa-inggris-tp1-001
class_source_id: class-9
type: pg
options_json:
  - text: "Jawaban benar"
    score: 1
  - text: "Distraktor"
    score: 0
answer_json:
  selected_index: 0
~~~

Aturan isi:

- pg: tepat satu opsi memiliki score 1; selected_index berupa angka.
- mr: dua atau lebih opsi boleh memiliki score 1; selected_index berupa array
  angka. Pada UI lama tipe ini juga disebut pgk; gunakan mr untuk file baru
  karena itu yang dipakai asesmen ini.
- bs: gunakan dua opsi Benar dan Salah, tepat satu memiliki score 1.
- Kunci penilaian server berasal dari options_json[].score; jangan hanya
  mengandalkan answer_json.
- explanation boleh ditulis untuk arsip guru, tetapi tidak boleh dikirim ke
  peserta apabila ujian memakai show_explanation_after: false.
- Untuk 20 soal dan skor maksimum 100, setiap relasi exam_questions harus
  memiliki points: 5.

## SOP yang harus dipakai pada permintaan berikutnya

### 1. Tetapkan identitas dan kontrak ujian

Sebelum menulis file, pastikan sudah ada:

- kelas dan mapel;
- judul ujian yang unik;
- jumlah soal dan distribusi tipe;
- durasi dalam menit;
- skor maksimum dan bobot per soal;
- token baru atau keputusan token bersama;
- apakah nilai, pembahasan, dan kunci boleh tampil setelah submit;
- source_id stabil untuk exam dan setiap question.

Jangan menggunakan token lama yang sudah dipakai kecuali token tersebut memang
dikonfigurasi sebagai shared token. Jangan mengganti source_id hanya untuk
memperbaiki isi soal; mengganti ID akan terlihat sebagai record baru.

### 2. Gunakan satu staging package

Untuk ujian Kelas 9 yang sekarang, package yang sudah memiliki state relasi
adalah:

~~~
/home/pakgun/royclass/content-staging/kelas9-bahasa-inggris/
~~~

Jangan membuat salinan soal yang sama di root dan di subdirektori. Duplikasi
file dengan source_id sama dapat membuat urutan import dan state sulit
ditelusuri. Untuk ujian baru, pilih satu staging directory dan satu
.seeder-state.json, lalu gunakan pilihan tersebut secara konsisten.

### 3. Tulis dan periksa Markdown

Setiap file harus memiliki front matter YAML yang valid, kind, dan source_id.
Buat question lebih dahulu, kemudian exam yang merujuk ke
question_source_ids. Contoh exam minimal:

~~~yaml
kind: exam
source_id: exam-kelas9-bahasa-inggris-baru
class_source_id: class-9
title: "Asesmen Bahasa Inggris Kelas 9"
duration_min: 80
points_per_question: 5
question_source_ids:
  - question-kelas9-bahasa-inggris-baru-001
tokens:
  - TOKEN-BARU
shared_tokens: false
random_questions: true
random_options: true
show_score_after: true
show_explanation_after: false
is_active: true
~~~

Sebelum menyentuh produksi, lakukan pemeriksaan manual berikut:

~~~bash
cd /home/pakgun/royclass
grep -R "^source_id:" content-staging/PAKET-UJIAN
grep -R "^type:\|^points_per_question:\|^show_explanation_after:" content-staging/PAKET-UJIAN
~~~

Periksa bahwa jumlah file, distribusi pg/mr/bs, kunci, dan total bobot sesuai
permintaan. Untuk soal Bahasa Inggris, lakukan proofreading terpisah: grammar,
ejaan, kejelasan instruksi, kecocokan TP, dan apakah hanya ada satu
interpretasi untuk kunci jawaban.

### 4. Jalankan dry-run dengan environment produksi

ai-bot memerlukan kredensial PocketBase dan path Hugo. Jangan menaruh password
di command atau dokumentasi.

~~~bash
cd /home/pakgun/royclass
set -a
. /var/www/class.gezytech.web.id/.env
set +a

STAGE=./content-staging/PAKET-UJIAN
STATE="$STAGE/.seeder-state.json"

./ai-bot/ai-bot sync \
  --staging-dir "$STAGE" \
  --state-file "$STATE" \
  --dry-run \
  --build-hugo=false
~~~

Dry-run wajib berhasil. Jika gagal, jangan menjalankan sync nyata dan jangan
mengubah pb_data/data.db secara manual.

### 5. Backup lalu import dan build sekali

~~~bash
cd /home/pakgun/royclass
set -a
. /var/www/class.gezytech.web.id/.env
set +a

STAGE=./content-staging/PAKET-UJIAN
STATE="$STAGE/.seeder-state.json"

./ai-bot/ai-bot backup

./ai-bot/ai-bot sync \
  --staging-dir "$STAGE" \
  --state-file "$STATE" \
  --hugo-dir /var/www/class.gezytech.web.id/hugo \
  --public-dir /var/www/class.gezytech.web.id/public
~~~

Perintah sync harus berakhir dengan sync complete tanpa error. Perintah ini
mengimpor ke PocketBase, menjalankan Hugo, lalu mengganti isi public/ dengan
hasil build yang baru.

### 6. Verifikasi live, bukan hanya file source

~~~bash
curl -fsS https://lms.royzy.web.id/api/health
curl -I https://lms.royzy.web.id/cbt/kelas9/
curl -I https://lms.royzy.web.id/cbt/ujian/
~~~

Untuk exam baru, verifikasi pula:

1. daftar ujian muncul pada kategori kelas yang benar;
2. token diterima;
3. jumlah soal sesuai;
4. tipe pg, mr, dan bs dapat dipilih;
5. soal acak dan durasi sesuai;
6. submit menampilkan nilai sesuai kebijakan;
7. jika show_explanation_after: false, endpoint hasil tidak mengirim
   review atau kunci jawaban.

Jangan memakai sesi siswa yang sedang berjalan sebagai sesi uji dan jangan
men-submit jawaban siswa untuk keperluan verifikasi.

## Daftar perbaikan sistem yang disarankan

### Prioritas tinggi

1. Buat satu script deploy resmi yang menyalin source repo ke direktori
   runtime, membangun binary ai-bot, menyalin hook/migration, me-restart
   PocketBase jika hook berubah, lalu menjalankan health check. Saat ini
   perubahan repo dan runtime harus disalin manual.
2. Tambahkan command ai-bot validate yang gagal sebelum import jika ada
   duplicate source_id, relasi yang tidak ditemukan, tipe soal tidak dikenal,
   jumlah kunci salah, atau total bobot tidak sesuai.
3. Tambahkan migration baru untuk menjadikan field boolean hasil ujian tidak
   required, lalu hilangkan workaround hard-coded berdasarkan ID/title exam
   dari hook. Migration yang sudah pernah diterapkan jangan diedit; buat file
   migration baru.
4. Jadikan import exam atomic atau lakukan preflight semua relasi sebelum
   menghapus relasi lama. Ini mencegah exam kosong jika pembuatan salah satu
   relasi gagal.

### Prioritas menengah

1. Tambahkan log deploy dengan commit hash, staging directory, jumlah soal,
   jumlah relasi, dan URL hasil verifikasi.
2. Jadikan GitHub sebagai sumber deploy yang benar-benar memiliki akses write,
   atau gunakan deploy key/CI dengan izin minimal. Pada audit ini VPS dapat
   membaca repo tetapi key yang tersedia tidak memiliki izin push.
3. Perbarui README lama agar hanya menyebut /home/pakgun/royclass dan
   https://lms.royzy.web.id; hilangkan contoh path/domain yang sudah tidak
   aktif.

## Definisi selesai untuk setiap ujian baru

- [ ] Semua soal lolos proofreading dan kunci telah diaudit.
- [ ] Distribusi tipe, jumlah soal, durasi, bobot, dan TP cocok dengan
      permintaan.
- [ ] source_id unik dan stabil.
- [ ] Semua question_source_ids ditemukan.
- [ ] Token baru atau shared token telah dipilih dengan sengaja.
- [ ] Dry-run berhasil.
- [ ] Backup berhasil.
- [ ] Sync nyata dan Hugo build berhasil.
- [ ] API health, daftar exam, halaman soal, pilihan jawaban, dan hasil akhir
      telah diverifikasi di lms.royzy.web.id.
- [ ] Tidak ada pembahasan/kunci pada respons jika kebijakan ujian melarangnya.
- [ ] Commit dibuat setelah verifikasi; push memerlukan kredensial GitHub
      dengan izin write.
