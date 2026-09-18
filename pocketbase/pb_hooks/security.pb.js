// GezyClass security hooks v6 — akses publik via endpoint khusus.
// - Soal: opsi diberi oid (indeks asli) + type (pg/pgk/bs); kunci tidak bocor.
// - Penilaian server: pg/bs = 1 jawaban; pgk = set jawaban (harus tepat semua).
// - Helper qs/j dideklarasikan lokal per router (aman di JSVM PocketBase).

// ---------- EXAM: start ----------
routerAdd("POST", "/api/exam/start", function (c) {
  try {
    var qs = function(v){ return "'" + String(v == null ? "" : v).replace(/'/g, "''") + "'"; };
    var body = c.requestInfo().body || {};
    var token = String(body.token || "").trim().toUpperCase();
    var examId = String(body.exam_id || "").trim();
    var nama = String(body.nama || "").trim().slice(0, 50);
    var noAbsen = String(body.no_absen || "").trim();
    var kelas = String(body.kelas || "").trim().slice(0, 8);
    var sekolahRaw = String(body.sekolah || "").trim();
    if (!token || !examId || !nama || !noAbsen || !kelas || !sekolahRaw) {
      return c.json(400, { message: "Semua field wajib diisi." });
    }
    if (sekolahRaw.length > 30) {
      return c.json(400, { message: "Nama sekolah maksimal 30 karakter." });
    }
    if (!/^[A-Za-z0-9 ]+$/.test(sekolahRaw)) {
      return c.json(400, { message: "Nama sekolah hanya boleh huruf, angka, dan spasi (maksimal 30 karakter)." });
    }
    var sekolah = sekolahRaw.slice(0, 30);
    var sessCol = $app.findCollectionByNameOrId("exam_sessions");
    var col = $app.findCollectionByNameOrId("exam_tokens");
    var found = $app.findRecordsByFilter(col,
      "token = " + qs(token) + " && exam_id = " + qs(examId), "", 1, 0);
    if (found.length === 0) return c.json(400, { message: "Token tidak valid atau sudah digunakan!" });
    var tk = found[0];
    var shared = !!tk.get("is_shared");
    var participantFilter = "token = " + qs(token) + " && exam_id = " + qs(examId) +
      " && status = 'ongoing' && nama = " + qs(nama) +
      " && no_absen = " + String(parseInt(noAbsen, 10) || 0) +
      " && kelas = " + qs(kelas);
    var existing = $app.findRecordsByFilter(sessCol, participantFilter, "", 1, 0);
    if (existing.length > 0) return c.json(200, { id: existing[0].id });
    if (!shared && tk.get("is_used")) {
      return c.json(400, { message: "Token tidak valid atau sudah digunakan!" });
    }
    if (!shared) {
      var usedOnce = $app.findRecordsByFilter($app.findCollectionByNameOrId("exam_sessions"),
        "token = " + qs(token) + " && exam_id = " + qs(examId), "", 1, 0);
      if (usedOnce.length > 0) return c.json(400, { message: "Token sudah digunakan." });
    }
    var rec = new Record(sessCol);
    rec.set("exam_id", examId);
    rec.set("token", token);
    rec.set("nama", nama);
    rec.set("no_absen", parseInt(noAbsen, 10) || 0);
    rec.set("kelas", kelas);
    rec.set("sekolah", sekolah);
    rec.set("status", "ongoing");
    rec.set("started_at", new Date().toISOString());
    $app.save(rec);
    if (!tk.get("is_shared")) {
      tk.set("is_used", true);
      tk.set("used_at", new Date().toISOString());
      $app.save(tk);
    }
    return c.json(200, { id: rec.id });
  } catch (err) {
    return c.json(500, { message: "Terjadi kesalahan: " + err.message });
  }
});

// ---------- EXAM: info sesi ----------
routerAdd("GET", "/api/exam/session", function (c) {
  try {
    var id = (c.requestInfo().query["id"] || "") || "";
    if (!id) return c.json(400, { message: "id wajib" });
    var rec = $app.findRecordById($app.findCollectionByNameOrId("exam_sessions"), id);
    var ex = null;
    try { ex = $app.findRecordById($app.findCollectionByNameOrId("exams"), rec.get("exam_id")); } catch (e2) {}
    return c.json(200, {
      id: rec.id, exam_id: rec.get("exam_id"), status: rec.get("status"),
      nama: rec.get("nama"), kelas: rec.get("kelas"), no_absen: rec.get("no_absen"),
      total_score: rec.get("total_score"), max_score: rec.get("max_score"),
      exam_title: ex ? (ex.get("title") || "Ujian") : "Ujian",
      duration: ex ? (ex.get("duration") || 1800) : 1800,
      survey_mode: ex ? !!ex.get("survey_mode") : false,
      survey_type: ex ? String(ex.get("survey_type") || "") : ""
    });
  } catch (e) {
    return c.json(404, { message: "Sesi tidak ditemukan." });
  }
});

// ---------- EXAM: soal publik (tanpa kunci; opsi + oid + type) ----------
routerAdd("GET", "/api/pub/exam-questions", function (c) {
  try {
    var qs = function(v){ return "'" + String(v == null ? "" : v).replace(/'/g, "''") + "'"; };
    var j = function(rec, f){
      var v = rec.get(f);
      if (v == null) return null;
      if (Array.isArray(v)) {
        if (v.length > 0 && typeof v[0] === "number") {
          var ss = "";
          for (var bi = 0; bi < v.length; bi++) ss += String.fromCharCode(v[bi]);
          try { return JSON.parse(ss); } catch (e2) { return null; }
        }
        return v;
      }
      if (typeof v === "string") { try { return JSON.parse(v); } catch (e2) { return null; } }
      return v;
    };
    var examId = (c.requestInfo().query["exam_id"] || "") || "";
    if (!examId) return c.json(400, { message: "exam_id wajib" });
    var eqs = $app.findRecordsByFilter($app.findCollectionByNameOrId("exam_questions"),
      "exam_id = " + qs(examId), "order_num", 500, 0);
    var qCol = $app.findCollectionByNameOrId("questions");
    var out = [];
    for (var i = 0; i < eqs.length; i++) {
      var eq = eqs[i];
      var qr;
      try { qr = $app.findRecordById(qCol, eq.get("question_id")); } catch (e2) { continue; }
      var opts = j(qr, "options_json") || [];
      var qtype = qr.get("type") || "pg";
      var statements = [];
      if(qtype === "bs"){
        var qm = String(qr.get("question") || "").match(/Tentukan nilai kebenaran ketiga pernyataan berikut:\s*(?:<br\s*\/?>)?([\s\S]*)$/i);
        if(qm) statements = qm[1].split(/<br\s*\/?>/i).map(function(s){return s.replace(/^\s*\(\d+\)\s*/,"").trim();}).filter(function(s){return s;});
        if(!statements.length && opts.length === 2) statements = [String(qr.get("question") || "")];
      }
      var cleanOpts = [];
      for (var k = 0; k < opts.length; k++) cleanOpts.push({ text: opts[k].text, oid: k });
      out.push({
        eqId: eq.id, qid: qr.id, question: qr.get("question"),
        options: cleanOpts, points: eq.get("points") || 5,
        type: qtype,
        statements: statements,
        explanation: qr.get("explanation") || "", difficulty: qr.get("difficulty") || ""
      });
    }
    return c.json(200, { items: out });
  } catch (e) {
    return c.json(500, { message: "Terjadi kesalahan: " + e.message });
  }
});

// ---------- EXAM: submit (penilaian server; pg/pgk/bs) ----------
routerAdd("POST", "/api/exam/submit", function (c) {
  try {
    var qs = function(v){ return "'" + String(v == null ? "" : v).replace(/'/g, "''") + "'"; };
    var j = function(rec, f){
      var v = rec.get(f);
      if (v == null) return null;
      if (Array.isArray(v)) {
        if (v.length > 0 && typeof v[0] === "number") {
          var ss = "";
          for (var bi = 0; bi < v.length; bi++) ss += String.fromCharCode(v[bi]);
          try { return JSON.parse(ss); } catch (e2) { return null; }
        }
        return v;
      }
      if (typeof v === "string") { try { return JSON.parse(v); } catch (e2) { return null; } }
      return v;
    };
    var eqArr = function(a, b){
      if (!a || !b || a.length !== b.length) return false;
      var ca = a.slice().sort(), cb = b.slice().sort();
      for (var i = 0; i < ca.length; i++) if (String(ca[i]) !== String(cb[i])) return false;
      return true;
    };
    var body = c.requestInfo().body || {};
    var sessionId = String(body.session_id || "").trim();
    var answers = body.answers || {};
    if (!sessionId) return c.json(400, { message: "session_id wajib" });
    var sess = $app.findRecordById($app.findCollectionByNameOrId("exam_sessions"), sessionId);
    if (sess.get("status") !== "ongoing") return c.json(400, { message: "Sesi sudah selesai." });
    var eqs = $app.findRecordsByFilter($app.findCollectionByNameOrId("exam_questions"),
      "exam_id = " + qs(sess.get("exam_id")), "", 500, 0);
    var qCol = $app.findCollectionByNameOrId("questions");
    var anCol = $app.findCollectionByNameOrId("exam_answers");
    var review = [], total = 0, max = 0, correct = 0, wrong = 0, empty = 0;
    for (var i = 0; i < eqs.length; i++) {
      var eq = eqs[i], eqId = eq.id, points = eq.get("points") || 5;
      max += points;
      var qr = $app.findRecordById(qCol, eq.get("question_id"));
      var qtype = qr.get("type") || "pg";
      var opts = j(qr, "options_json") || [];
      var correctSet = [];
      for (var x = 0; x < opts.length; x++) if (opts[x].score === 1) correctSet.push(x);
      var sel = answers[eqId];
      if(qtype === "bs"){
        if(Array.isArray(sel) && sel.length === 1 && opts.length === 2){
          // Single-statement B/S items submit [true|false]; map directly to the option.
          sel = sel[0] === true ? 0 : (sel[0] === false ? 1 : -1);
        } else if(Array.isArray(sel)){
          var pattern = [];
          for(var bi=0; bi<sel.length; bi++) pattern.push(sel[bi] === true);
          var bsText = "(1) "+(pattern[0]?"Benar":"Salah")+", (2) "+(pattern[1]?"Benar":"Salah")+", (3) "+(pattern[2]?"Benar":"Salah");
          sel = -1;
          for(var bo=0; bo<opts.length; bo++) if(String(opts[bo].text) === bsText) { sel=bo; break; }
        }
      }
      var selArr = [], has = false;
      if (Array.isArray(sel)) {
        selArr = sel.filter(function(z){ return z !== null && z !== undefined && z !== ""; });
        has = selArr.length > 0;
      } else {
        has = (sel !== undefined && sel !== null && sel !== "");
        if (has) selArr = [sel];
      }
      var isCorrect = has && eqArr(selArr, correctSet);
      if (!has) empty++; else if (isCorrect) correct++; else wrong++;
      if (isCorrect) total += points;
      var cleanOpts = [];
      for (var y = 0; y < opts.length; y++) cleanOpts.push({ text: opts[y].text, oid: y });
      var corrIdx = (qtype === "pgk" || qtype === "mr") ? correctSet : (correctSet.length ? correctSet[0] : -1);
      review.push({
        eqId: eqId, qid: qr.id, question: qr.get("question"), type: qtype,
        options: cleanOpts,
        selected_index: (qtype === "bs" && Array.isArray(answers[eqId]) ? answers[eqId] : (Array.isArray(sel) ? selArr : (has ? sel : null))),
        correct_index: corrIdx, is_correct: isCorrect,
        score: isCorrect ? points : 0, max_score: points,
        explanation: qr.get("explanation") || ""
      });
    }
    for (var jj = 0; jj < review.length; jj++) {
      var item = review[jj];
      var existing = $app.findRecordsByFilter(anCol,
        "session_id = " + qs(sessionId) + " && question_id = " + qs(item.qid), "", 1, 0);
      var rec;
      if (existing.length) rec = existing[0];
      else { rec = new Record(anCol); rec.set("session_id", sessionId); rec.set("question_id", item.qid); }
      rec.set("answer_json", { selected_index: item.selected_index });
      rec.set("is_correct", item.is_correct);
      rec.set("score", item.score);
      rec.set("max_score", item.max_score);
      rec.set("submitted_at", new Date().toISOString());
      $app.save(rec);
    }
    var exam = $app.findRecordById($app.findCollectionByNameOrId("exams"), sess.get("exam_id"));
    var survey = !!exam.get("survey_mode");
    var surveyType = String(exam.get("survey_type") || "");
    var A = 0, B = 0, prof = "", skor = 0, kategori = "";
    if (survey) {
      if (surveyType === "likert") {
        var forward = [2,3,5,6,9,10,13,15,18,19];
        for (var li = 0; li < eqs.length; li++) {
          var leq = eqs[li];
          var lsel = answers[leq.id];
          if (lsel === undefined || lsel === null || lsel === "") continue;
          var lOid = Array.isArray(lsel) ? (lsel.length ? lsel[0] : null) : lsel;
          if (lOid === null || lOid === undefined) continue;
          if (lOid < 0 || lOid > 3) continue;
          var lOrd = leq.get("order_num") || (li + 1);
          if (forward.indexOf(lOrd) >= 0) skor += (3 - lOid); else skor += lOid;
        }
        if (skor <= 20) kategori = "Fix Mindset";
        else if (skor <= 33) kategori = "Fix Grow Mindset";
        else if (skor <= 44) kategori = "Grow Fix Mindset";
        else kategori = "Grow Mindset";
        prof = kategori;
        sess.set("questions_order_json", JSON.stringify({ kategori: kategori, skor: skor }));
      } else {
        for (var si = 0; si < eqs.length; si++) {
          var seq = eqs[si], sq = $app.findRecordById(qCol, seq.get("question_id"));
          var sopt = j(sq, "options_json") || [];
          var ssel = answers[seq.id];
          var sHas = (ssel !== undefined && ssel !== null && ssel !== "");
          if (!sHas) continue;
          var sOid = Array.isArray(ssel) ? (ssel.length ? ssel[0] : null) : ssel;
          if (sOid === null || sOid === undefined) continue;
          var sText = sopt[sOid] ? String(sopt[sOid].text || "") : "";
          if (!/^setuju$/i.test(sText.trim())) continue;
          var ord = seq.get("order_num") || (si + 1);
          if (ord % 2 === 1) A++; else B++;
        }
        prof = A > B ? "PPT" : (A === B ? "Campuran" : "PPB");
        sess.set("questions_order_json", JSON.stringify({ profile: prof, A: A, B: B }));
      }
    }
    sess.set("status", "submitted");
    sess.set("ended_at", new Date().toISOString());
    sess.set("total_score", survey ? 0 : total);
    sess.set("max_score", survey ? 0 : max);
    $app.save(sess);
    var hideReview = String(exam.id || exam.get("id") || "") === "e31b5ex1n310lck" ||
      String(exam.get("title") || "") === "Asesmen Bahasa Inggris SMP Kelas 9 — Komunikasi dan Opini";
    return c.json(200, {
      total_score: survey ? 0 : total, max_score: survey ? 0 : max,
      correct: survey ? 0 : correct, wrong: survey ? 0 : wrong, empty: survey ? 0 : empty,
      survey: survey,
      survey_type: surveyType,
      profile: survey ? (surveyType === "likert" ? { skor: skor, kategori: kategori, label: kategori } : { A: A, B: B, label: prof }) : undefined,
      nama: sess.get("nama"), kelas: sess.get("kelas"),
      exam_title: exam.get("title") || "Ujian",
      show_score: survey ? false : !!exam.get("show_score_after"),
      show_explanation: survey || hideReview ? false : !!exam.get("show_explanation_after"),
      review: survey || hideReview ? [] : review
    });
  } catch (e) {
    return c.json(500, { message: "Terjadi kesalahan: " + e.message });
  }
});

// ---------- EXAM: review ----------
routerAdd("GET", "/api/exam/review", function (c) {
  try {
    var qs = function(v){ return "'" + String(v == null ? "" : v).replace(/'/g, "''") + "'"; };
    var j = function(rec, f){
      var v = rec.get(f);
      if (v == null) return null;
      if (Array.isArray(v)) {
        if (v.length > 0 && typeof v[0] === "number") {
          var ss = "";
          for (var bi = 0; bi < v.length; bi++) ss += String.fromCharCode(v[bi]);
          try { return JSON.parse(ss); } catch (e2) { return null; }
        }
        return v;
      }
      if (typeof v === "string") { try { return JSON.parse(v); } catch (e2) { return null; } }
      return v;
    };
    var sessionId = (c.requestInfo().query["session"] || "") || "";
    if (!sessionId) return c.json(400, { message: "session wajib" });
    var sess = $app.findRecordById($app.findCollectionByNameOrId("exam_sessions"), sessionId);
    var exam = $app.findRecordById($app.findCollectionByNameOrId("exams"), sess.get("exam_id"));
    var eqs = $app.findRecordsByFilter($app.findCollectionByNameOrId("exam_questions"),
      "exam_id = " + qs(sess.get("exam_id")), "", 500, 0);
    var ansList = $app.findRecordsByFilter($app.findCollectionByNameOrId("exam_answers"),
      "session_id = " + qs(sessionId), "", 500, 0);
    var ansMap = {};
    for (var i = 0; i < ansList.length; i++) ansMap[ansList[i].get("question_id")] = ansList[i];
    var qCol = $app.findCollectionByNameOrId("questions");
    var review = [], correct = 0, wrong = 0, empty = 0;
    for (var j2 = 0; j2 < eqs.length; j2++) {
      var eq = eqs[j2], qid = eq.get("question_id");
      var qr = $app.findRecordById(qCol, qid);
      var qtype = qr.get("type") || "pg";
      var opts = j(qr, "options_json") || [];
      var correctSet = [];
      for (var x = 0; x < opts.length; x++) if (opts[x].score === 1) correctSet.push(x);
      var ans = ansMap[qid];
      var aj = ans ? (j(ans, "answer_json") || {}) : {};
      var selIdx = ans ? aj.selected_index : null;
      var selArr = [];
      if (Array.isArray(selIdx)) { selArr = selIdx; }
      else if (selIdx !== null && selIdx !== undefined && selIdx !== "") { selArr = [selIdx]; }
      var isCorrect = ans ? !!ans.get("is_correct") : false;
      if (selArr.length === 0) empty++; else if (isCorrect) correct++; else wrong++;
      var cleanOpts = [];
      for (var y = 0; y < opts.length; y++) cleanOpts.push({ text: opts[y].text, oid: y });
      var corrIdx = (qtype === "pgk" || qtype === "mr") ? correctSet : (correctSet.length ? correctSet[0] : -1);
      review.push({
        question: qr.get("question"), type: qtype, options: cleanOpts,
        selected_index: (Array.isArray(selIdx) ? selArr : (selArr.length ? selArr[0] : null)),
        correct_index: corrIdx,
        is_correct: isCorrect, explanation: qr.get("explanation") || "",
        points: eq.get("points") || 5
      });
    }
    var survey = !!exam.get("survey_mode");
    var surveyType = String(exam.get("survey_type") || "");
    var prof = null;
    if (survey) {
      try {
        var pj = j(sess, "questions_order_json") || {};
        if (pj) {
          if (surveyType === "likert" && pj.kategori !== undefined) prof = { skor: pj.skor || 0, kategori: String(pj.kategori), label: String(pj.kategori) };
          else if (pj.profile) prof = { A: pj.A || 0, B: pj.B || 0, label: String(pj.profile) };
        }
      } catch (e3) {}
    }
    var hideReview = String(exam.id || exam.get("id") || "") === "e31b5ex1n310lck" ||
      String(exam.get("title") || "") === "Asesmen Bahasa Inggris SMP Kelas 9 — Komunikasi dan Opini";
    return c.json(200, {
      nama: sess.get("nama"), kelas: sess.get("kelas"), no_absen: sess.get("no_absen"),
      exam_title: exam.get("title") || "Ujian",
      survey: survey,
      survey_type: surveyType,
      profile: prof,
      show_score: survey ? false : !!exam.get("show_score_after"),
      show_explanation: survey || hideReview ? false : !!exam.get("show_explanation_after"),
      total_score: survey ? 0 : (sess.get("total_score") || 0),
      max_score: survey ? 0 : (sess.get("max_score") || 0),
      correct: survey ? 0 : correct, wrong: survey ? 0 : wrong, empty: survey ? 0 : empty,
      review: survey || hideReview ? [] : review
    });
  } catch (e) {
    return c.json(500, { message: "Terjadi kesalahan: " + e.message });
  }
});

// ---------- LATIHAN: daftar soal publik (tanpa kunci) ----------
routerAdd("GET", "/api/pub/latihan", function (c) {
  try {
    var qs = function(v){ return "'" + String(v == null ? "" : v).replace(/'/g, "''") + "'"; };
    var j = function(rec, f){
      var v = rec.get(f);
      if (v == null) return null;
      if (Array.isArray(v)) {
        if (v.length > 0 && typeof v[0] === "number") {
          var ss = "";
          for (var bi = 0; bi < v.length; bi++) ss += String.fromCharCode(v[bi]);
          try { return JSON.parse(ss); } catch (e2) { return null; }
        }
        return v;
      }
      if (typeof v === "string") { try { return JSON.parse(v); } catch (e2) { return null; } }
      return v;
    };
    var slug = (c.requestInfo().query["slug"] || "") || "";
    if (!slug) return c.json(400, { message: "slug wajib" });
    var list = $app.findRecordsByFilter($app.findCollectionByNameOrId("latihan_soal"),
      "slug = " + qs(slug), "", 1, 0);
    if (!list.length) return c.json(404, { message: "Soal tidak ditemukan." });
    var rec = list[0];
    var soal = j(rec, "soal_json") || [];
    var out = [];
    for (var i = 0; i < soal.length; i++) {
      var q = soal[i];
      out.push({ q: q.q, o: q.o || [], type: q.type || "pg", statements: q.statements || [] });
    }
    return c.json(200, {
      judul: rec.get("judul"), kelas: rec.get("kelas"), semester: rec.get("semester"),
      bab: rec.get("bab"), back_url: rec.get("back_url"), soal: out
    });
  } catch (e) {
    return c.json(500, { message: "Terjadi kesalahan: " + e.message });
  }
});

// ---------- LATIHAN: start ----------
routerAdd("POST", "/api/latihan/start", function (c) {
  try {
    var qs = function(v){ return "'" + String(v == null ? "" : v).replace(/'/g, "''") + "'"; };
    var body = c.requestInfo().body || {};
    var slug = String(body.slug || "").trim();
    var nama = String(body.nama || "").trim();
    var absen = String(body.no_absen || "").trim();
    var kelas = String(body.kelas || "").trim();
    var sekolah = String(body.sekolah || "").trim();
    if (!slug || !nama || !absen || !kelas || !sekolah) return c.json(400, { message: "Data tidak lengkap." });
    if (sekolah.length > 30 || !/^[A-Za-z0-9 ]+$/.test(sekolah)) return c.json(400, { message: "Nama sekolah maksimal 30 karakter dan hanya boleh berisi huruf, angka, dan spasi." });
    var col = $app.findCollectionByNameOrId("latihan_sesi");
    var existing = $app.findRecordsByFilter(col,
      "latihan_slug = " + qs(slug) + " && nama = " + qs(nama) + " && no_absen = " + qs(absen) + " && kelas = " + qs(kelas),
      "", 100, 0);
    if (existing.length >= 3) return c.json(400, { message: "Kamu sudah mengerjakan latihan ini 3 kali." });
    var rec = new Record(col);
    rec.set("latihan_slug", slug);
    rec.set("nama", nama);
    rec.set("no_absen", absen);
    rec.set("kelas", kelas);
    rec.set("sekolah", sekolah);
    rec.set("attempt", existing.length + 1);
    rec.set("access_key", "K" + Math.random().toString(36).slice(2, 10));
    rec.set("status", "ongoing");
    rec.set("started_at", new Date().toISOString());
    $app.save(rec);
    return c.json(200, { id: rec.id });
  } catch (e) {
    return c.json(500, { message: "Terjadi kesalahan: " + e.message });
  }
});

// ---------- LATIHAN: submit ----------
routerAdd("POST", "/api/latihan/submit", function (c) {
  try {
    var qs = function(v){ return "'" + String(v == null ? "" : v).replace(/'/g, "''") + "'"; };
    var j = function(rec, f){
      var v = rec.get(f);
      if (v == null) return null;
      if (Array.isArray(v)) {
        if (v.length > 0 && typeof v[0] === "number") {
          var ss = "";
          for (var bi = 0; bi < v.length; bi++) ss += String.fromCharCode(v[bi]);
          try { return JSON.parse(ss); } catch (e2) { return null; }
        }
        return v;
      }
      if (typeof v === "string") { try { return JSON.parse(v); } catch (e2) { return null; } }
      return v;
    };
    var body = c.requestInfo().body || {};
    var sessionId = String(body.session_id || "").trim();
    var answers = body.answers || {};
    if (!sessionId) return c.json(400, { message: "session_id wajib" });
    var rec = $app.findRecordById($app.findCollectionByNameOrId("latihan_sesi"), sessionId);
    if (rec.get("status") !== "ongoing") return c.json(400, { message: "Latihan sudah dikumpulkan." });
    var soalList = $app.findRecordsByFilter($app.findCollectionByNameOrId("latihan_soal"),
      "slug = " + qs(rec.get("latihan_slug")), "", 1, 0);
    if (!soalList.length) return c.json(500, { message: "Data soal tidak ditemukan." });
    var soal = j(soalList[0], "soal_json") || [];
    var detail = [], benar = 0, salah = 0, kosong = 0, benarNomor = [], salahNomor = [];
    for (var i = 0; i < soal.length; i++) {
      var q = soal[i];
      var s = answers[i];
      var isEmpty = s === undefined || s === null || s === "" || (Array.isArray(s) && s.length === 0);
      var isCorrect = q.type === "bs" ? (Array.isArray(s) && Array.isArray(q.k) && s.length === q.k.length && s.every(function(v,ix){return v === q.k[ix];})) : (q.type === "mr" && Array.isArray(s) && Array.isArray(q.k) && JSON.stringify(s.slice().sort()) === JSON.stringify(q.k.slice().sort())) || (q.type !== "mr" && String(s) === String(q.k));
      if (isEmpty) { kosong++; detail.push({ i: i, ok: 0, st: "-" }); }
      else if (isCorrect) { benar++; benarNomor.push(i + 1); detail.push({ i: i, ok: 1, st: s }); }
      else { salah++; salahNomor.push(i + 1); detail.push({ i: i, ok: 0, st: s }); }
    }
    rec.set("status", "submitted");
    rec.set("ended_at", new Date().toISOString());
    rec.set("total_score", benar);
    rec.set("max_score", soal.length);
    rec.set("jawaban_json", detail);
    $app.save(rec);
    return c.json(200, { total_score: benar, max_score: soal.length, correct: benar, wrong: salah, empty: kosong, benar_nomor: benarNomor, salah_nomor: salahNomor });
  } catch (e) {
    return c.json(500, { message: "Terjadi kesalahan: " + e.message });
  }
});

// ---------- GURU: daftar latihan + sesi (auth guru) ----------
routerAdd("GET", "/api/guru/latihan", function (c) {
  try {
    var qs = function(v){ return "'" + String(v == null ? "" : v).replace(/'/g, "''") + "'"; };
    var j = function(rec, f){
      var v = rec.get(f);
      if (v == null) return null;
      if (Array.isArray(v)) {
        if (v.length > 0 && typeof v[0] === "number") {
          var ss = "";
          for (var bi = 0; bi < v.length; bi++) ss += String.fromCharCode(v[bi]);
          try { return JSON.parse(ss); } catch (e2) { return null; }
        }
        return v;
      }
      if (typeof v === "string") { try { return JSON.parse(v); } catch (e2) { return null; } }
      return v;
    };
    var ri = c.requestInfo();
    if (!ri.auth || ri.auth.collection().name !== "guru") {
      return c.json(403, { message: "Akses khusus guru." });
    }
    var out = [];
    var lat = $app.findRecordsByFilter($app.findCollectionByNameOrId("latihan_soal"), "is_active = true", "", 500, 0);
    for (var i = 0; i < lat.length; i++) {
      var rec = lat[i];
      var sesiList = $app.findRecordsByFilter($app.findCollectionByNameOrId("latihan_sesi"),
        "latihan_slug = " + qs(rec.get("slug")), "", 1000, 0);
      var sesi = [];
      for (var k = 0; k < sesiList.length; k++) {
        var sv = sesiList[k];
        sesi.push({
          id: sv.id, nama: sv.get("nama"), no_absen: sv.get("no_absen"), kelas: sv.get("kelas"),
          status: sv.get("status") || "",
          attempt: sv.get("attempt") || 1, total_score: sv.get("total_score") || 0,
          max_score: sv.get("max_score") || 0, jawaban_json: j(sv, "jawaban_json") || [],
          ended_at: sv.get("ended_at") || ""
        });
      }
      out.push({ slug: rec.get("slug"), judul: rec.get("judul"), kelas: rec.get("kelas"), bab: rec.get("bab"), sesi: sesi });
    }
    return c.json(200, { items: out });
  } catch (e) {
    return c.json(500, { message: "Terjadi kesalahan: " + e.message });
  }
});

// ---------- GURU: rekap hasil CBT (auth guru) ----------
routerAdd("GET", "/api/guru/exam-recap", function (c) {
  try {
    var ri = c.requestInfo();
    if (!ri.auth || ri.auth.collection().name !== "guru") {
      return c.json(403, { message: "Akses khusus guru." });
    }
    var j = function(rec, f){
      var v = rec.get(f);
      if (v == null) return null;
      if (Array.isArray(v)) {
        if (v.length > 0 && typeof v[0] === "number") {
          var ss = "";
          for (var bi = 0; bi < v.length; bi++) ss += String.fromCharCode(v[bi]);
          try { return JSON.parse(ss); } catch (e2) { return null; }
        }
        return v;
      }
      if (typeof v === "string") { try { return JSON.parse(v); } catch (e2) { return null; } }
      return v;
    };

    var exams = $app.findRecordsByFilter(
      $app.findCollectionByNameOrId("exams"),
      "is_active = true", "title", 500, 0
    );
    var sessionsCol = $app.findCollectionByNameOrId("exam_sessions");
    var classesCol = $app.findCollectionByNameOrId("classes");
    var out = [];

    for (var i = 0; i < exams.length; i++) {
      var exam = exams[i];
      var className = "";
      var classID = String(exam.get("class_id") || "");
      if (classID) {
        try {
          className = String($app.findRecordById(classesCol, classID).get("name") || "");
        } catch (classErr) {}
      }

      var sessionList = $app.findRecordsByFilter(
        sessionsCol, "exam_id = '" + exam.id.replace(/'/g, "''") + "'",
        "-started_at", 1000, 0
      );
      var sesi = [];
      for (var k = 0; k < sessionList.length; k++) {
        var session = sessionList[k];
        var prof = "", profA = null, profB = null;
        try {
          var pj = j(session, "questions_order_json") || {};
          if (pj && pj.profile) { prof = String(pj.profile); profA = pj.A || 0; profB = pj.B || 0; }
          else if (pj && pj.kategori !== undefined) { prof = String(pj.kategori); profA = pj.skor || 0; profB = null; }
        } catch (e2b) {}
        sesi.push({
          id: session.id,
          nama: session.get("nama") || "",
          no_absen: session.get("no_absen"),
          kelas: session.get("kelas") || "",
          sekolah: session.get("sekolah") || "",
          status: session.get("status") || "",
          total_score: session.get("total_score") || 0,
          max_score: session.get("max_score") || 0,
          profile: prof,
          profile_a: profA,
          profile_b: profB,
          started_at: session.get("started_at") || "",
          ended_at: session.get("ended_at") || ""
        });
      }
      out.push({
        exam_id: exam.id,
        title: exam.get("title") || "Ujian",
        class_name: className,
        duration: exam.get("duration") || 0,
        survey: !!exam.get("survey_mode"),
        survey_type: String(exam.get("survey_type") || ""),
        sesi: sesi
      });
    }
    return c.json(200, { items: out });
  } catch (e) {
    return c.json(500, { message: "Terjadi kesalahan: " + e.message });
  }
});

// ---------- GURU: monitoring progress CBT (auth guru) ----------
routerAdd("GET", "/api/guru/exam-progress", function (c) {
  try {
    var ri = c.requestInfo();
    if (!ri.auth || ri.auth.collection().name !== "guru") {
      return c.json(403, { message: "Akses khusus guru." });
    }

    var qs = function(v){ return "'" + String(v == null ? "" : v).replace(/'/g, "''") + "'"; };
    var exams = $app.findRecordsByFilter(
      $app.findCollectionByNameOrId("exams"),
      "is_active = true", "title", 500, 0
    );
    var sessionsCol = $app.findCollectionByNameOrId("exam_sessions");
    var questionsCol = $app.findCollectionByNameOrId("exam_questions");
    var classesCol = $app.findCollectionByNameOrId("classes");
    var items = [];
    var examOptions = [];
    var answerStats = arrayOf(new DynamicModel({
      sessionId: "",
      answered: 0,
      lastActivityUnix: 0
    }));
    var answerStatsBySession = {};
    $app.db().newQuery(
      "SELECT session_id AS sessionId, COUNT(*) AS answered, " +
      "unixepoch(MAX(submitted_at)) AS lastActivityUnix " +
      "FROM exam_answers GROUP BY session_id"
    ).all(answerStats);
    for (var statIndex = 0; statIndex < answerStats.length; statIndex++) {
      answerStatsBySession[String(answerStats[statIndex].sessionId || "")] = answerStats[statIndex];
    }

    for (var i = 0; i < exams.length; i++) {
      var exam = exams[i];
      var className = "";
      var classID = String(exam.get("class_id") || "");
      if (classID) {
        try {
          className = String($app.findRecordById(classesCol, classID).get("name") || "");
        } catch (classErr) {}
      }

      var totalQuestions = $app.findRecordsByFilter(
        questionsCol, "exam_id = " + qs(exam.id), "", 500, 0
      ).length;
      examOptions.push({ id: exam.id, title: exam.get("title") || "Ujian", class_name: className });

      var sessions = $app.findRecordsByFilter(
        sessionsCol, "exam_id = " + qs(exam.id), "-started_at", 1000, 0
      );
      for (var k = 0; k < sessions.length; k++) {
        var session = sessions[k];
        var answerStat = answerStatsBySession[session.id] || {};
        var startedAt = session.getDateTime("started_at");
        var endedAt = session.getDateTime("ended_at");
        var lastActivity = startedAt.isZero() ? "" : startedAt.string();
        if (answerStat.lastActivityUnix) {
          lastActivity = new Date(answerStat.lastActivityUnix * 1000).toISOString();
        }
        if (!endedAt.isZero()) lastActivity = endedAt.string();

        items.push({
          id: session.id,
          exam_id: exam.id,
          exam_title: exam.get("title") || "Ujian",
          class_name: className,
          nama: session.get("nama") || "",
          no_absen: session.get("no_absen"),
          kelas: session.get("kelas") || className,
          status: session.get("status") || "ongoing",
          answered: parseInt(answerStat.answered, 10) || 0,
          total_questions: totalQuestions,
          started_at: session.get("started_at") || "",
          ended_at: session.get("ended_at") || "",
          last_activity: lastActivity
        });
      }
    }

    return c.json(200, {
      items: items,
      exams: examOptions,
      server_time: new Date().toISOString()
    });
  } catch (e) {
    return c.json(500, { message: "Terjadi kesalahan: " + e.message });
  }
});

// ---------- EXAM: autosave jawaban ----------
routerAdd("POST", "/api/exam/autosave", function (c) {
  try {
    var qs = function(v){ return "'" + String(v == null ? "" : v).replace(/'/g, "''") + "'"; };
    var j = function(rec, f){
      var v = rec.get(f);
      if (v == null) return null;
      if (Array.isArray(v)) {
        if (v.length > 0 && typeof v[0] === "number") {
          var ss = "";
          for (var bi = 0; bi < v.length; bi++) ss += String.fromCharCode(v[bi]);
          try { return JSON.parse(ss); } catch (e2) { return null; }
        }
        return v;
      }
      if (typeof v === "string") { try { return JSON.parse(v); } catch (e2) { return null; } }
      return v;
    };
    var eqArr = function(a, b){
      if (!a || !b || a.length !== b.length) return false;
      var ca = a.slice().sort(), cb = b.slice().sort();
      for (var i = 0; i < ca.length; i++) if (String(ca[i]) !== String(cb[i])) return false;
      return true;
    };
    var body = c.requestInfo().body || {};
    var sessionId = String(body.session_id || "").trim();
    var answers = body.answers || {};
    if (!sessionId) return c.json(400, { message: "session_id wajib" });
    var sess = $app.findRecordById($app.findCollectionByNameOrId("exam_sessions"), sessionId);
    if (sess.get("status") !== "ongoing") return c.json(200, { ok: true, skipped: true });
    var eqs = $app.findRecordsByFilter($app.findCollectionByNameOrId("exam_questions"),
      "exam_id = " + qs(sess.get("exam_id")), "", 500, 0);
    var qCol = $app.findCollectionByNameOrId("questions");
    var anCol = $app.findCollectionByNameOrId("exam_answers");
    var saved = 0;
    for (var i = 0; i < eqs.length; i++) {
      var eq = eqs[i], eqId = eq.id;
      var sel = answers[eqId];
      if (sel === undefined || sel === null) continue;
      var qr = $app.findRecordById(qCol, eq.get("question_id"));
      var opts = j(qr, "options_json") || [];
      var correctSet = [];
      for (var x = 0; x < opts.length; x++) if (opts[x].score === 1) correctSet.push(x);
      var selArr = [], has = false;
      if (Array.isArray(sel)) {
        selArr = sel.filter(function(z){ return z !== null && z !== undefined && z !== ""; });
        has = selArr.length > 0;
      } else {
        has = (sel !== "" && sel !== null && sel !== undefined);
        if (has) selArr = [sel];
      }
      if (!has) continue;
      var isCorrect = eqArr(selArr, correctSet);
      var existing = $app.findRecordsByFilter(anCol,
        "session_id = " + qs(sessionId) + " && question_id = " + qs(qr.id), "", 1, 0);
      var rec;
      var selectedValue = Array.isArray(sel) ? selArr : sel;
      if (existing.length) {
        rec = existing[0];
        var previousAnswer = j(rec, "answer_json") || {};
        var previousValue = previousAnswer.selected_index;
        var unchanged = Array.isArray(selectedValue) && Array.isArray(previousValue)
          ? eqArr(selectedValue, previousValue)
          : String(previousValue) === String(selectedValue);
        if (unchanged) continue;
      } else {
        rec = new Record(anCol);
        rec.set("session_id", sessionId);
        rec.set("question_id", qr.id);
      }
      rec.set("answer_json", { selected_index: selectedValue });
      rec.set("is_correct", isCorrect);
      rec.set("score", isCorrect ? (eq.get("points") || 5) : 0);
      rec.set("max_score", eq.get("points") || 5);
      rec.set("submitted_at", new Date().toISOString());
      $app.save(rec);
      saved++;
    }
    return c.json(200, { ok: true, saved: saved });
  } catch (e) {
    return c.json(500, { message: "Terjadi kesalahan: " + e.message });
  }
});

// ---------- EXAM: finalize-stale (dipanggil cron eksternal tiap menit) ----------
routerAdd("POST", "/api/exam/finalize-stale", function (c) {
  try {
    var ri = c.requestInfo();
    var key = String((ri.query && ri.query["key"]) || "").trim();
    var expect = "";
    try {
      var b = $os.readFile("../.autofinalize_key");
      if (b && b.length !== undefined) {
        var ss = "";
        for (var i = 0; i < b.length; i++) ss += String.fromCharCode(b[i]);
        expect = ss.trim();
      }
    } catch (e) {}
    if (!expect) return c.json(500, { message: "kunci internal belum diset" });
    if (key !== expect) return c.json(403, { message: "forbidden" });

    var qs = function(v){ return "'" + String(v == null ? "" : v).replace(/'/g, "''") + "'"; };
    var j = function(rec, f){
      var v = rec.get(f);
      if (v == null) return null;
      if (Array.isArray(v)) {
        if (v.length > 0 && typeof v[0] === "number") {
          var ss = "";
          for (var bi = 0; bi < v.length; bi++) ss += String.fromCharCode(v[bi]);
          try { return JSON.parse(ss); } catch (e2) { return null; }
        }
        return v;
      }
      if (typeof v === "string") { try { return JSON.parse(v); } catch (e2) { return null; } }
      return v;
    };
    var sessCol = $app.findCollectionByNameOrId("exam_sessions");
    var exCol = $app.findCollectionByNameOrId("exams");
    var eqCol = $app.findCollectionByNameOrId("exam_questions");
    var anCol = $app.findCollectionByNameOrId("exam_answers");
    var qCol = $app.findCollectionByNameOrId("questions");
    var now = new Date();
    var list = $app.findRecordsByFilter(sessCol, "status = 'ongoing'", "", 500, 0);
    var finalized = 0;
    for (var i2 = 0; i2 < list.length; i2++) {
      var sess = list[i2];
      var started = sess.get("started_at");
      if (!started) continue;
      var ex = null;
      try { ex = $app.findRecordById(exCol, sess.get("exam_id")); } catch (e2) {}
      var dur = ex ? (ex.get("duration") || 1800) : 1800;
      var deadline = new Date(new Date(started).getTime() + dur * 1000 + 5 * 60 * 1000);
      if (now < deadline) continue;
      var eqs = $app.findRecordsByFilter(eqCol, "exam_id = " + qs(sess.get("exam_id")), "", 500, 0);
      var max = 0;
      for (var k2 = 0; k2 < eqs.length; k2++) max += (eqs[k2].get("points") || 5);
      var ansList = $app.findRecordsByFilter(anCol, "session_id = " + qs(sess.id), "", 500, 0);
      var total = 0;
      for (var a = 0; a < ansList.length; a++) total += (ansList[a].get("score") || 0);
      var isSur = ex ? !!ex.get("survey_mode") : false;
      if (isSur) {
        var surType = ex ? String(ex.get("survey_type") || "") : "";
        var ansMapS = {};
        for (var am = 0; am < ansList.length; am++) ansMapS[ansList[am].get("question_id")] = ansList[am];
        if (surType === "likert") {
          var fwd = [2,3,5,6,9,10,13,15,18,19], skorS = 0;
          for (var l2 = 0; l2 < eqs.length; l2++) {
            var le2 = eqs[l2], an2 = ansMapS[le2.get("question_id")];
            if (!an2) continue;
            var aj2 = j(an2, "answer_json") || {};
            var sel2 = aj2.selected_index;
            var oid2 = Array.isArray(sel2) ? (sel2.length ? sel2[0] : null) : sel2;
            if (oid2 === null || oid2 === undefined || oid2 === "") continue;
            if (oid2 < 0 || oid2 > 3) continue;
            var ord2 = le2.get("order_num") || (l2 + 1);
            if (fwd.indexOf(ord2) >= 0) skorS += (3 - oid2); else skorS += oid2;
          }
          var katS = skorS <= 20 ? "Fix Mindset" : (skorS <= 33 ? "Fix Grow Mindset" : (skorS <= 44 ? "Grow Fix Mindset" : "Grow Mindset"));
          sess.set("questions_order_json", JSON.stringify({ kategori: katS, skor: skorS }));
          total = 0; max = 0;
        } else {
          var A2 = 0, B2 = 0;
          for (var s3 = 0; s3 < eqs.length; s3++) {
            var le3 = eqs[s3], an3 = ansMapS[le3.get("question_id")];
            if (!an3) continue;
            var q3 = null;
            try { q3 = $app.findRecordById(qCol, le3.get("question_id")); } catch (e5) { continue; }
            var aj3 = j(an3, "answer_json") || {};
            var sel3 = aj3.selected_index;
            var oid3 = Array.isArray(sel3) ? (sel3.length ? sel3[0] : null) : sel3;
            if (oid3 === null || oid3 === undefined || oid3 === "") continue;
            var opts3 = j(q3, "options_json") || [];
            var txt3 = opts3[oid3] ? String(opts3[oid3].text || "") : "";
            if (!/^setuju$/i.test(txt3.trim())) continue;
            var ord3 = le3.get("order_num") || (s3 + 1);
            if (ord3 % 2 === 1) A2++; else B2++;
          }
          var pr3 = A2 > B2 ? "PPT" : (A2 === B2 ? "Campuran" : "PPB");
          sess.set("questions_order_json", JSON.stringify({ profile: pr3, A: A2, B: B2 }));
          total = 0; max = 0;
        }
      }
      sess.set("status", "submitted");
      sess.set("ended_at", now.toISOString());
      sess.set("total_score", total);
      sess.set("max_score", max);
      $app.save(sess);
      finalized++;
    }
    if (finalized > 0) console.log("AUTO-FINALIZE: " + finalized + " sesi difinalisasi");
    return c.json(200, { finalized: finalized });
  } catch (e) {
    return c.json(500, { message: "Terjadi kesalahan: " + e.message });
  }
});
