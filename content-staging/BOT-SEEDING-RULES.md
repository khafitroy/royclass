# Bot Seeding Rules

Dokumen ini wajib dibaca dan diikuti sebelum bot membuat, mengubah, atau melakukan seed konten GezyClass.

## Tujuan

Bot boleh membantu mengelola konten GezyClass dengan cara aman:

1. Menulis file Markdown ke `content-staging/`.
2. Menjalankan validasi atau `sync --dry-run`.
3. Menjalankan `sync` hanya setelah staging valid.
4. Tidak pernah mengedit database SQLite atau folder `public/` secara langsung.

## Prinsip Wajib

- `content-staging/` adalah satu-satunya tempat bot menulis konten sumber.
- `PocketBase` tetap menjadi sumber data runtime.
- `sync` adalah satu-satunya proses yang boleh mengimpor staging ke PocketBase.
- `hugo --minify` dan swap `public/` hanya boleh dilakukan melalui command `ai-bot sync`.
- Setiap file harus idempotent: menjalankan `sync` berulang tidak boleh membuat duplikasi.
- Semua file harus memakai `source_id` stabil.
- Semua relasi baru harus memakai `*_source_id`, bukan ID PocketBase, kecuali data itu memang berasal dari record lama di PocketBase.
- `order` wajib ada untuk setiap `chapter`, `subchapter`, `material`, `example`, dan `exercise`.
- `order` menentukan urutan baca dan navigasi `sebelumnya / berikutnya` di Hugo.
- Saat membuat item baru di bab yang sama, bot harus melanjutkan `order` terakhir, bukan mengulang `1`.
- URL publik materi wajib mengikuti struktur `kelas/bab/submateri`, misalnya `/materi/kelas-7/bilangan-bulat/konsep-dan-garis-bilangan-bulat/`.
- Bot tidak boleh membuat atau membagikan URL submateri langsung di bawah kelas seperti `/materi/kelas-7/konsep-dan-garis-bilangan-bulat/`, kecuali sebagai alias kompatibilitas untuk link lama.

## Larangan Keras

Bot tidak boleh:

- Mengedit `/var/www/class.gezytech.web.id/pocketbase/pb_data/data.db`.
- Mengedit `/var/www/class.gezytech.web.id/pocketbase/pb_migrations/`.
- Mengedit `/var/www/class.gezytech.web.id/public/` secara langsung.
- Menghapus record `exam_sessions`.
- Menghapus record `exam_answers`.
- Mengubah data user, auth, atau `_superusers`.
- Menghapus token ujian yang sudah dipakai.
- Menulis file `.md` tanpa front matter.
- Membuat `source_id` acak untuk konten yang seharusnya bisa diperbarui.
- Menjalankan command destructive seperti `rm -rf` pada folder produksi.

## Folder Kerja

Di VPS:

```text
/var/www/class.gezytech.web.id/
├── ai-bot/
├── content-staging/
├── pocketbase/
├── hugo/
├── public/
└── backups/
```

Bot harus menjalankan command dari:

```bash
cd /home/pakgun/royclass/ai-bot
```

Default staging dir:

```text
../content-staging
```

## Workflow Wajib

### 1. Buat atau ubah staging

Gunakan command `ai-bot` atau tulis Markdown langsung di `content-staging/`.

Contoh:

```bash
./ai-bot material create --class 7 --chapter "Bilangan Bulat" --subchapter "Pengenalan Bilangan Bulat" --title "Pengenalan Bilangan Bulat"
```

### 2. Validasi dry-run

Wajib jalankan:

```bash
./ai-bot sync --dry-run --build-hugo=false
```

Jika dry-run gagal, bot wajib berhenti dan memperbaiki staging. Jangan lanjut ke `sync` nyata.

### 3. Backup

Sebelum import nyata, jalankan:

```bash
./ai-bot backup
```

### 4. Import dan build

Setelah dry-run berhasil dan backup berhasil:

```bash
./ai-bot sync
```

### 5. Verifikasi

Setelah `sync`, bot harus memeriksa:

```bash
curl -I https://lms.royzy.web.id/
curl -I https://lms.royzy.web.id/api/health
```

Jika endpoint tidak sehat, bot wajib melaporkan error dan tidak melakukan perubahan lanjutan.

## Format Markdown Umum

Setiap file wajib:

```markdown
---
kind: material
source_id: material-kelas-7-bilangan-bulat-pengenalan
title: Pengenalan Bilangan Bulat
---

Isi markdown...
```

Field wajib:

- `kind`
- `source_id`

Aturan `source_id`:

- Gunakan `kebab-case`.
- Stabil antar update.
- Jangan pakai timestamp kecuali konten memang event sekali jalan.
- Prefix dengan jenis konten.

Contoh:

```text
class-7
chapter-kelas-7-bilangan-bulat
subchapter-kelas-7-bilangan-bulat-pengenalan
material-kelas-7-bilangan-bulat-pengenalan
question-kelas-7-bilangan-bulat-001
article-mengenal-aljabar
exam-kelas-7-uh1-bilangan-bulat
```

## Kind Yang Didukung

### `class`

```yaml
kind: class
source_id: class-7
name: Kelas 7
slug: kelas-7
order: 7
```

### `chapter`

```yaml
kind: chapter
source_id: chapter-kelas-7-bilangan-bulat
class_source_id: class-7
name: Bilangan Bulat
slug: bilangan-bulat
order: 1
```

### `subchapter`

```yaml
kind: subchapter
source_id: subchapter-kelas-7-bilangan-bulat-pengenalan
chapter_source_id: chapter-kelas-7-bilangan-bulat
name: Pengenalan Bilangan Bulat
slug: pengenalan-bilangan-bulat
order: 1
```

### `material`

```yaml
kind: material
source_id: material-kelas-7-bilangan-bulat-pengenalan
subchapter_source_id: subchapter-kelas-7-bilangan-bulat-pengenalan
title: Pengenalan Bilangan Bulat
order: 1
```

Body Markdown menjadi field `content`.

### `example`

```yaml
kind: example
source_id: example-kelas-7-bilangan-bulat-001
material_source_id: material-kelas-7-bilangan-bulat-pengenalan
title: Contoh Soal 1
solution: "Pembahasan..."
order: 1
```

Body Markdown menjadi field `question`.

### `exercise`

```yaml
kind: exercise
source_id: exercise-kelas-7-bilangan-bulat-001
subchapter_source_id: subchapter-kelas-7-bilangan-bulat-pengenalan
title: Latihan 1
solution: "Pembahasan..."
order: 1
```

### Aturan navigasi bab

- Satu `chapter` berisi beberapa `subchapter`.
- Satu `subchapter` berisi satu atau lebih `material`.
- Halaman Hugo menampilkan navigasi `prev/next` berdasarkan `order`.
- Struktur URL Hugo untuk submateri adalah `/materi/{kelas}/{chapter_slug}/{subchapter_slug}/`.
- Contoh benar: `/materi/kelas-7/bilangan-bulat/konsep-dan-garis-bilangan-bulat/`.
- Contoh salah: `/materi/kelas-7/konsep-dan-garis-bilangan-bulat/`.
- Jika bot menambah subchapter baru dalam chapter yang sudah ada, gunakan `order` berikutnya yang kosong.
- Jika bot menambah material baru dalam subchapter yang sudah ada, gunakan `order` berikutnya yang kosong.
- Jangan mengganti `source_id` hanya untuk mengubah urutan.

Body Markdown menjadi field `question`.

### `question`

```yaml
kind: question
source_id: question-kelas-7-bilangan-bulat-001
class_source_id: class-7
subchapter_source_id: subchapter-kelas-7-bilangan-bulat-pengenalan
type: pg
difficulty: sedang
options_json:
  - text: "4"
    score: 1
  - text: "-4"
    score: 0
answer_json:
  selected_index: 0
explanation: "Karena -8 + 12 = 4."
```

Body Markdown menjadi field `question`.

Tipe soal yang boleh dipakai:

- `pg`
- `mr`
- `bs`
- `isian`
- `essay`
- `mc`

Difficulty yang boleh dipakai:

- `mudah`
- `sedang`
- `sulit`

### `article`

```yaml
kind: article
source_id: article-mengenal-aljabar
title: Mengenal Aljabar
slug: mengenal-aljabar
category_source_id: category-aljabar
tag_source_ids:
  - tag-kelas-7
published: false
```

Body Markdown menjadi field `content`.

### `exam`

```yaml
kind: exam
source_id: exam-kelas-7-uh1-bilangan-bulat
class_source_id: class-7
title: UH 1 - Bilangan Bulat
description: Ulangan harian bilangan bulat.
duration_min: 30
points_per_question: 10
question_source_ids:
  - question-kelas-7-bilangan-bulat-001
tokens:
  - UH1-BIL-001
shared_tokens: false
is_active: true
```

`points_per_question` bersifat opsional dan menentukan bobot setiap soal dalam
ujian. Jika tidak diisi, nilainya 10. `shared_tokens` juga opsional; jika
bernilai `true`, token dapat dipakai oleh lebih dari satu peserta.

Catatan:

- Untuk soal lama yang sudah ada di PocketBase, `question_ids` boleh berisi ID PocketBase langsung.
- Untuk soal yang dibuat dari staging, gunakan `question_source_ids`.
- Token baru boleh dibuat di staging.
- Token yang sudah dipakai tidak boleh dihapus.

#### Format ringkas satu file

Untuk ujian baru, soal dapat ditulis langsung di front matter `questions` pada
satu file exam. Lihat `examples/exam-inline.md.example`. Setiap item dapat
memakai `source_id`; jika dikosongkan, importer membuat ID stabil berdasarkan
source ID exam dan nomor urut soal. Gunakan format ini agar tidak perlu membuat
satu file Markdown untuk setiap soal.

Format lama dengan `question_source_ids` dan file `kind: question` terpisah
tetap didukung untuk data yang sudah ada.

## Validasi Konten

Sebelum `sync`, bot wajib memeriksa:

- Semua file punya front matter valid.
- Semua `kind` dikenal.
- Semua `source_id` unik.
- Semua relasi `*_source_id` punya file sumber atau sudah ada di `.seeder-state.json`.
- Tidak ada field kosong untuk `title`, `slug`, atau relasi wajib.
- Soal pilihan ganda punya minimal 2 opsi.
- Soal pilihan ganda punya tepat satu opsi dengan `score: 1`, kecuali tipe memang mengizinkan multi jawaban.
- LaTeX tidak rusak secara jelas: delimiter `$...$` dan `$$...$$` berpasangan.

## Kebijakan Publish

- Artikel default harus `published: false`.
- Materi boleh langsung aktif setelah `sync`, tetapi wajib dry-run dulu.
- Ujian boleh `is_active: true` hanya jika token dan daftar soal sudah final.
- Bot tidak boleh mengubah ujian aktif jika sudah ada sesi ujian berjalan, kecuali atas instruksi eksplisit.

## Command Yang Boleh Dipakai

```bash
./ai-bot material create ...
./ai-bot article create ...
./ai-bot exam create ...
./ai-bot exam token ...
./ai-bot exam delete-token ...
./ai-bot sync --dry-run --build-hugo=false
./ai-bot backup
./ai-bot sync
```

Command manual untuk inspeksi aman:

```bash
find ../content-staging -type f
sed -n '1,160p' ../content-staging/path/to/file.md
curl -I https://lms.royzy.web.id/
curl -I https://lms.royzy.web.id/api/health
```

## Jika Terjadi Error

Bot harus:

1. Berhenti.
2. Laporkan command yang gagal.
3. Laporkan pesan error.
4. Jangan mencoba memperbaiki dengan mengedit `pb_data/data.db`.
5. Jangan menghapus folder produksi.
6. Jika import sudah terjadi, gunakan backup terakhir sebagai rujukan rollback manual.

## Checklist Sebelum Sync Nyata

Bot wajib memastikan semua poin berikut terpenuhi:

- [ ] File staging dibuat di `content-staging/`.
- [ ] Tidak ada perubahan langsung ke `public/`.
- [ ] Tidak ada perubahan langsung ke `pb_data/data.db`.
- [ ] `./ai-bot sync --dry-run --build-hugo=false` berhasil.
- [ ] `./ai-bot backup` berhasil.
- [ ] Relasi `*_source_id` valid.
- [ ] Token ujian tidak menghapus token yang sudah dipakai.
- [ ] Konten sudah sesuai permintaan pengguna.

Jika ada satu poin gagal, jangan jalankan `./ai-bot sync`.
