---
title: "Content Staging"
---

# Content Staging

Wajib baca dulu:

```text
content-staging/BOT-SEEDING-RULES.md
```

Audit arsitektur dan SOP publikasi CBT:

```text
docs/CBT-AUDIT-DAN-SOP.md
```

Runtime produksi menggunakan repo `/home/pakgun/royclass` untuk menjalankan
importer. Target publiknya adalah `https://lms.royzy.web.id/`.

Untuk CBT baru, gunakan format ringkas satu file exam yang berisi `questions`.
Contohnya ada di `content-staging/examples/exam-inline.md.example`. Format
lama dengan satu file untuk setiap soal tetap didukung untuk kompatibilitas,
tetapi tidak perlu dipakai untuk ujian baru.

Letakkan markdown di sini, lalu jalankan:

```bash
cd /home/pakgun/royclass/ai-bot
go run . sync --staging-dir ../content-staging
```

Command legacy sekarang juga menulis ke staging:

```bash
cd /home/pakgun/royclass/ai-bot
go run . material create --class 7 --chapter "Bilangan Bulat" --subchapter "Pengenalan Bilangan Bulat" --title "Pengenalan Bilangan Bulat"
go run . article create --title "Mengenal Aljabar" --category "Aljabar"
go run . exam create --title "UH 1 - Bilangan Bulat" --class 7 --questions 10 --tokens 5 --duration 30
```

Secara default CLI memakai `../content-staging` jika dijalankan dari direktori `ai-bot`.

Aturan:

- Setiap file pakai YAML front matter.
- `source_id` harus stabil.
- Referensi antar record pakai `*_source_id`, bukan ID PocketBase.
- `order` wajib dan dipakai untuk navigasi `prev/next` di halaman materi.
- Untuk bab/subbab baru, lanjutkan `order` terakhir yang sudah ada.
- URL materi harus mengikuti struktur `/materi/{kelas}/{chapter_slug}/{subchapter_slug}/`.
- Jangan membagikan URL submateri langsung di bawah kelas, misalnya `/materi/kelas-7/konsep-dan-garis-bilangan-bulat/`, kecuali sebagai alias kompatibilitas.
- `sync` menyimpan state di `content-staging/.seeder-state.json`.
- `backup` membuat salinan `pb_data/data.db` dan `auxiliary.db`.

Kind yang didukung:

- `class`
- `category`
- `tag`
- `chapter`
- `subchapter`
- `material`
- `example`
- `exercise`
- `article`
- `question`
- `exam`

Contoh file ada di `content-staging/examples/*.md.example`.
