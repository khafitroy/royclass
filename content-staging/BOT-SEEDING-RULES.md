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
- **CBT baru wajib ditulis dalam satu file `kind: exam`.** Seluruh soal ditulis
  di field `questions` pada file exam tersebut; bot membuat record soal dan
  relasinya secara internal.
- Bot dilarang membuat satu file Markdown terpisah untuk setiap soal CBT baru.
- Format satu-file-per-question hanya legacy untuk data lama dan tidak boleh
  dipakai untuk ujian CBT baru.
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

### `question` (legacy, bukan untuk CBT baru)

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

Format ini hanya dipertahankan agar data lama tetap dapat disinkronkan. Untuk
CBT baru, jangan membuat file `kind: question`; gunakan `questions` di dalam
satu file `kind: exam` seperti contoh berikut.

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
source_id: exam-kelas-9-bahasa-inggris-komunikasi-opini
class_source_id: class-7
title: Asesmen Bahasa Inggris Kelas 9
description: Ujian Bahasa Inggris tentang komunikasi dan opini.
duration_min: 80
points_per_question: 5
tokens:
  - TOKEN-BARU
shared_tokens: true
random_questions: true
random_options: true
show_score_after: true
show_explanation_after: false
is_active: true
questions:
  - source_id: question-kelas9-bahasa-inggris-001
    type: pg
    difficulty: mudah
    question: |
      Which sentence expresses an opinion?
    options:
      - text: "I think the library needs more books."
        score: 1
      - text: "The library opens at seven."
        score: 0
    answer:
      selected_index: 0
    explanation: "The phrase 'I think' introduces an opinion."
  - source_id: question-kelas9-bahasa-inggris-002
    type: bs
    difficulty: mudah
    question: |
      A caption can explain the message of an image.
    options:
      - text: "Benar"
        score: 1
      - text: "Salah"
        score: 0
    answer:
      selected_index: 0
```

`points_per_question` bersifat opsional dan menentukan bobot setiap soal dalam
ujian. Jika tidak diisi, nilainya 10. `shared_tokens` juga opsional; jika
bernilai `true`, token dapat dipakai oleh lebih dari satu peserta.

Catatan:

- Field `questions` wajib berisi semua soal untuk CBT baru.
- Setiap item `questions` memakai `type`: `pg`, `mr`, atau `bs`.
- `options` berisi objek `text` dan `score`; kunci penilaian berasal dari
  opsi dengan `score: 1`.
- `answer.selected_index` berupa angka untuk `pg`/`bs` dan array angka untuk
  `mr`.
- `source_id` pada setiap item soal sebaiknya ditulis eksplisit dan stabil.
  Jika kosong, bot membuat ID berdasarkan source ID exam dan nomor urut.
- `question_ids` dan `question_source_ids` hanya boleh dipakai untuk menjaga
  kompatibilitas exam lama, bukan untuk CBT baru.
- Token baru boleh dibuat di staging.
- Token yang sudah dipakai tidak boleh dihapus.

Contoh lengkap 20 soal satu-file ada di
`examples/exam-inline.md.example`.

## Validasi Konten

Sebelum `sync`, bot wajib memeriksa:

- Semua file punya front matter valid.
- Semua `kind` dikenal.
- Semua `source_id` unik.
- Semua relasi `*_source_id` punya file sumber atau sudah ada di `.seeder-state.json`.
- Tidak ada field kosong untuk `title`, `slug`, atau relasi wajib.
- CBT baru memiliki tepat satu file `kind: exam` dan tidak memiliki file
  `kind: question` terpisah.
- Field `questions` tidak kosong dan setiap item memiliki `type`, `question`,
  minimal 2 `options`, dan `answer`.
- Soal `pg` dan `bs` memiliki tepat satu opsi dengan `score: 1`; soal `mr`
  memiliki semua opsi benar yang ditandai `score: 1`.
- Jumlah item `questions`, distribusi tipe, dan total bobot sesuai permintaan.
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
