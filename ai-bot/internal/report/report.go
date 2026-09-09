package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"gezyclass/ai-bot/internal/pocketbase"
)

// Options controls one read-only report request.
type Options struct {
	Type       string
	Activity   string
	Class      string
	Metric     string
	Format     string
	Date       string
	MasterPath string
	Output     string
	List       bool
}

type Activity struct {
	Kind  string `json:"type"`
	ID    string `json:"id,omitempty"`
	Title string `json:"title"`
	Slug  string `json:"slug,omitempty"`
}

type Student struct {
	Class  string `json:"kelas"`
	No     string `json:"no_absen"`
	Name   string `json:"nama"`
	Gender string `json:"jenis_kelamin,omitempty"`
	NISN   string `json:"nisn,omitempty"`
	Key    string `json:"-"`
}

type Attempt struct {
	ID            string
	Kind          string
	ActivityID    string
	ActivityTitle string
	RawClass      string
	Class         string
	RawNo         string
	No            string
	EnteredName   string
	School        string
	Status        string
	StartedAt     string
	EndedAt       string
	Attempt       int
	Score         *float64
	MaxScore      *float64
	Percent       *float64
	Key           string
}

type ReportRow struct {
	Status       string   `json:"status"`
	Class        string   `json:"kelas"`
	No           string   `json:"no_absen"`
	Name         string   `json:"nama"`
	NISN         string   `json:"nisn,omitempty"`
	Gender       string   `json:"jenis_kelamin,omitempty"`
	Activity     string   `json:"aktivitas,omitempty"`
	RecordID     string   `json:"record_id,omitempty"`
	Attempt      int      `json:"percobaan,omitempty"`
	AttemptCount int      `json:"jumlah_percobaan,omitempty"`
	Score        *float64 `json:"skor,omitempty"`
	MaxScore     *float64 `json:"skor_maksimum,omitempty"`
	Percent      *float64 `json:"nilai_persen,omitempty"`
	StartedAt    string   `json:"mulai,omitempty"`
	EndedAt      string   `json:"selesai,omitempty"`
	EnteredName  string   `json:"nama_isian,omitempty"`
	NameCheck    string   `json:"pengecekan_nama,omitempty"`
	RawClass     string   `json:"kelas_isian,omitempty"`
	RawNo        string   `json:"no_absen_isian,omitempty"`
}

type ReviewRecord struct {
	Reason         string `json:"alasan"`
	RecordID       string `json:"record_id,omitempty"`
	Activity       string `json:"aktivitas,omitempty"`
	RawClass       string `json:"kelas_isian,omitempty"`
	CanonicalClass string `json:"kelas_terbaca,omitempty"`
	RawNo          string `json:"no_absen_isian,omitempty"`
	EnteredName    string `json:"nama_isian,omitempty"`
	MasterName     string `json:"nama_master,omitempty"`
	Status         string `json:"status,omitempty"`
}

type Summary struct {
	MasterStudents     int `json:"jumlah_master"`
	SudahMengerjakan   int `json:"sudah_mengerjakan"`
	SedangMengerjakan  int `json:"sedang_mengerjakan"`
	BelumMengerjakan   int `json:"belum_mengerjakan"`
	OtherStudentStatus int `json:"status_lainnya,omitempty"`
	SubmittedRecords   int `json:"record_submitted"`
	OngoingRecords     int `json:"record_ongoing"`
	OtherRecords       int `json:"record_lainnya,omitempty"`
	UnmatchedRecords   int `json:"record_tidak_tercocok"`
	NameMismatches     int `json:"nama_tidak_cocok"`
	ReportRows         int `json:"baris_laporan"`
}

type Report struct {
	GeneratedAt string         `json:"dibuat_pada"`
	Source      string         `json:"sumber"`
	Activity    Activity       `json:"aktivitas"`
	Metric      string         `json:"mode_nilai"`
	ClassFilter string         `json:"filter_kelas,omitempty"`
	DateFilter  string         `json:"filter_tanggal,omitempty"`
	Summary     Summary        `json:"ringkasan"`
	Rows        []ReportRow    `json:"baris"`
	Unmatched   []ReviewRecord `json:"perlu_peninjauan,omitempty"`
	NameReview  []ReviewRecord `json:"pengecekan_nama,omitempty"`
}

type rawMasterEntry [][]json.RawMessage

var dateLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02 15:04:05.999Z07:00",
	"2006-01-02 15:04:05Z07:00",
	"2006-01-02 15:04:05.999",
	"2006-01-02 15:04:05",
}

var classRomanPattern = regexp.MustCompile(`^(VIII|VII|IX)`)

// Build reads the roster and PocketBase records, then builds a report in
// memory. It never writes to PocketBase and never changes CBT/Latihan data.
func Build(client *pocketbase.Client, options Options) (Report, error) {
	kind := strings.ToLower(strings.TrimSpace(options.Type))
	if kind == "" {
		kind = "cbt"
	}
	if kind == "exercise" || kind == "practice" {
		kind = "latihan"
	}
	if kind != "cbt" && kind != "latihan" {
		return Report{}, fmt.Errorf("jenis sumber tidak valid %q; gunakan cbt atau latihan", options.Type)
	}
	metric, err := normalizeMetric(options.Metric)
	if err != nil {
		return Report{}, err
	}
	if options.Date != "" {
		if _, err := time.Parse("2006-01-02", options.Date); err != nil {
			return Report{}, fmt.Errorf("tanggal harus berformat YYYY-MM-DD: %w", err)
		}
	}
	if metric != "all" && strings.TrimSpace(options.Activity) == "" {
		return Report{}, errors.New("--activity wajib diisi untuk mode highest, average, atau latest; mode all dapat membaca semua aktivitas")
	}

	masterPath := options.MasterPath
	if masterPath == "" {
		hugoDir := envOr("HUGO_DIR", "/home/pgun/gezyclass/hugo")
		masterPath = filepath.Join(hugoDir, "layouts", "partials", "murid-data.html")
	}
	students, err := loadMaster(masterPath)
	if err != nil {
		return Report{}, err
	}
	classFilter := strings.TrimSpace(options.Class)
	if classFilter != "" {
		canonicalFilter := canonicalClass(classFilter)
		if canonicalFilter == "" {
			return Report{}, fmt.Errorf("kelas tidak dikenali: %q", classFilter)
		}
		students = filterStudentsByClass(students, canonicalFilter)
	}

	activities, err := loadActivities(client, kind)
	if err != nil {
		return Report{}, err
	}
	selected, err := resolveActivity(options.Activity, activities, metric)
	if err != nil {
		return Report{}, err
	}

	attempts, err := loadAttempts(client, kind, activities, selected, options.Date)
	if err != nil {
		return Report{}, err
	}
	if classFilter != "" {
		attempts = filterAttemptsByClass(attempts, canonicalClass(classFilter))
	}

	result := Report{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Source:      "PocketBase " + kind + " + master /murid",
		Activity:    selected,
		Metric:      metric,
		ClassFilter: classFilter,
		DateFilter:  options.Date,
		Summary: Summary{
			MasterStudents: len(students),
		},
	}

	masterByKey := make(map[string]Student, len(students))
	for _, student := range students {
		masterByKey[student.Key] = student
	}
	byKey := make(map[string][]Attempt)
	for _, attempt := range attempts {
		if student, ok := masterByKey[attempt.Key]; ok {
			byKey[attempt.Key] = append(byKey[attempt.Key], attempt)
			if attempt.Status == "submitted" {
				result.Summary.SubmittedRecords++
			} else if attempt.Status == "ongoing" {
				result.Summary.OngoingRecords++
			} else {
				result.Summary.OtherRecords++
			}
			if attempt.EnteredName != "" && normalizeName(attempt.EnteredName) != normalizeName(student.Name) {
				result.Summary.NameMismatches++
				result.NameReview = append(result.NameReview, reviewForNameMismatch(attempt, student))
			}
		} else {
			result.Summary.UnmatchedRecords++
			result.Unmatched = append(result.Unmatched, reviewForUnmatched(attempt))
		}
	}

	for key, rows := range byKey {
		sort.SliceStable(rows, func(i, j int) bool {
			return attemptTime(rows[i]).Before(attemptTime(rows[j]))
		})
		byKey[key] = rows
	}

	for _, student := range students {
		rows := byKey[student.Key]
		if len(rows) == 0 {
			result.Summary.BelumMengerjakan++
			result.Rows = append(result.Rows, absentRow(student, selected.Title))
			continue
		}
		if hasStatus(rows, "submitted") {
			result.Summary.SudahMengerjakan++
		} else if hasStatus(rows, "ongoing") {
			result.Summary.SedangMengerjakan++
		} else {
			result.Summary.OtherStudentStatus++
		}

		if metric == "all" {
			for index, attempt := range rows {
				result.Rows = append(result.Rows, attemptRow(attempt, student, index+1, len(rows)))
			}
		} else {
			result.Rows = append(result.Rows, aggregateRow(rows, student, metric))
		}
	}

	// Keep review sections deterministic for both the bot and exported files.
	sort.SliceStable(result.Unmatched, func(i, j int) bool {
		return reviewSortKey(result.Unmatched[i]) < reviewSortKey(result.Unmatched[j])
	})
	sort.SliceStable(result.NameReview, func(i, j int) bool {
		return reviewSortKey(result.NameReview[i]) < reviewSortKey(result.NameReview[j])
	})
	result.Summary.ReportRows = len(result.Rows)
	return result, nil
}

func normalizeKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "latihan", "exercise", "practice":
		return "latihan"
	default:
		return "cbt"
	}
}

func normalizeMetric(metric string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(metric)) {
	case "", "all", "semua", "semua-percobaan", "semua percobaan":
		return "all", nil
	case "highest", "tertinggi", "max", "maksimum":
		return "highest", nil
	case "average", "rata-rata", "rata", "avg":
		return "average", nil
	case "latest", "terakhir", "last":
		return "latest", nil
	default:
		return "", fmt.Errorf("mode nilai tidak valid %q; gunakan all, highest, average, atau latest", metric)
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func loadMaster(path string) ([]Student, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("gagal membaca master /murid %s: %w", path, err)
	}
	marker := []byte("var STUDENTS=")
	start := strings.Index(string(data), string(marker))
	if start < 0 {
		return nil, fmt.Errorf("var STUDENTS tidak ditemukan di %s", path)
	}
	payload := strings.TrimSpace(string(data[start+len(marker):]))
	payload = strings.TrimSuffix(payload, ";")
	var raw map[string]rawMasterEntry
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return nil, fmt.Errorf("master /murid bukan JSON yang valid: %w", err)
	}

	var students []Student
	for className, entries := range raw {
		class := canonicalClass(className)
		if class == "" {
			continue
		}
		for _, entry := range entries {
			if len(entry) < 2 {
				continue
			}
			student := Student{
				Class: class,
				No:    normalizeNo(rawValueString(entry[0])),
				Name:  strings.TrimSpace(rawValueString(entry[1])),
			}
			if len(entry) > 2 {
				student.Gender = strings.TrimSpace(rawValueString(entry[2]))
			}
			if len(entry) > 3 {
				student.NISN = strings.TrimSpace(rawValueString(entry[3]))
			}
			if student.No == "" || student.Name == "" {
				continue
			}
			student.Key = studentKey(student.Class, student.No)
			students = append(students, student)
		}
	}
	sort.SliceStable(students, func(i, j int) bool {
		if students[i].Class != students[j].Class {
			return students[i].Class < students[j].Class
		}
		return numberSortValue(students[i].No) < numberSortValue(students[j].No)
	})
	if len(students) == 0 {
		return nil, fmt.Errorf("master /murid di %s tidak berisi siswa", path)
	}
	return students, nil
}

func rawValueString(raw json.RawMessage) string {
	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return fmt.Sprint(typed)
	}
}

func filterStudentsByClass(students []Student, class string) []Student {
	filtered := make([]Student, 0, len(students))
	for _, student := range students {
		if classMatches(student.Class, class) {
			filtered = append(filtered, student)
		}
	}
	return filtered
}

func filterAttemptsByClass(attempts []Attempt, class string) []Attempt {
	filtered := make([]Attempt, 0, len(attempts))
	for _, attempt := range attempts {
		if classMatches(attempt.Class, class) {
			filtered = append(filtered, attempt)
		}
	}
	return filtered
}

func classMatches(value, filter string) bool {
	if value == filter {
		return true
	}
	// A request for class 8 means all sections 8.1, 8.2, etc.
	return len(filter) == 1 && strings.HasPrefix(value, filter+".")
}

func canonicalClass(raw string) string {
	compact := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToUpper(r)
		}
		return -1
	}, strings.TrimSpace(raw))
	compact = strings.TrimPrefix(compact, "KELAS")
	if compact == "" {
		return ""
	}

	grade := ""
	remainder := ""
	if match := classRomanPattern.FindStringSubmatch(compact); match != nil {
		switch match[1] {
		case "VII":
			grade = "7"
		case "VIII":
			grade = "8"
		case "IX":
			grade = "9"
		}
		remainder = strings.TrimPrefix(compact, match[1])
	} else if compact[0] >= '0' && compact[0] <= '9' {
		grade = string(compact[0])
		remainder = compact[1:]
	} else {
		return ""
	}
	if grade != "7" && grade != "8" && grade != "9" {
		return ""
	}
	if remainder == "" {
		return grade
	}

	// Inputs such as "VII A/7." and "VIII2 (8" occur in old records. The
	// first section marker is the useful part; the trailing duplicate grade is
	// ignored only for Roman-numeral forms.
	section := remainder[0]
	if section >= '1' && section <= '8' {
		if len(remainder) > 1 && !strings.HasPrefix(compact, "VII") && !strings.HasPrefix(compact, "VIII") && !strings.HasPrefix(compact, "IX") {
			return ""
		}
		return grade + "." + string(section)
	}
	if section >= 'A' && section <= 'H' {
		return grade + "." + strconv.Itoa(int(section-'A')+1)
	}
	return ""
}

func normalizeNo(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if number, err := strconv.Atoi(value); err == nil {
		return strconv.Itoa(number)
	}
	if number, err := strconv.ParseFloat(value, 64); err == nil && number == float64(int64(number)) {
		return strconv.FormatInt(int64(number), 10)
	}
	return value
}

func studentKey(class, no string) string {
	return canonicalClass(class) + "#" + normalizeNo(no)
}

func normalizeName(value string) string {
	var builder strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func numberSortValue(value string) int {
	if number, err := strconv.Atoi(value); err == nil {
		return number
	}
	return 999999
}

func valueString(record map[string]interface{}, key string) string {
	value, ok := record[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case json.Number:
		return typed.String()
	default:
		return fmt.Sprint(typed)
	}
}

func numberValue(record map[string]interface{}, key string) (float64, bool) {
	value, ok := record[key]
	if !ok || value == nil {
		return 0, false
	}
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case string:
		number, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return number, err == nil
	default:
		return 0, false
	}
}

func activityTitle(activity Activity) string {
	if activity.Title != "" {
		return activity.Title
	}
	if activity.Slug != "" {
		return activity.Slug
	}
	return activity.ID
}

func reviewSortKey(review ReviewRecord) string {
	return review.CanonicalClass + "#" + normalizeNo(review.RawNo) + "#" + review.RecordID
}

func rawStatus(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func displayStatus(status string) string {
	switch rawStatus(status) {
	case "submitted":
		return "Sudah mengerjakan"
	case "ongoing":
		return "Sedang mengerjakan"
	default:
		if status == "" {
			return "Status tidak diketahui"
		}
		return status
	}
}

func hasStatus(attempts []Attempt, status string) bool {
	for _, attempt := range attempts {
		if attempt.Status == status {
			return true
		}
	}
	return false
}

func attemptTime(attempt Attempt) time.Time {
	for _, value := range []string{attempt.EndedAt, attempt.StartedAt} {
		if value == "" {
			continue
		}
		for _, layout := range dateLayouts {
			if parsed, err := time.Parse(layout, value); err == nil {
				return parsed
			}
		}
	}
	return time.Time{}
}

func dateMatches(attempt Attempt, date string) bool {
	if date == "" {
		return true
	}
	return strings.HasPrefix(attempt.StartedAt, date) || strings.HasPrefix(attempt.EndedAt, date)
}

func floatPointer(value float64) *float64 {
	return &value
}

func round(value float64) float64 {
	return mathRound(value, 2)
}

func mathRound(value float64, places int) float64 {
	power := mathPow10(places)
	return float64(int64(value*power+0.5)) / power
}

func mathPow10(places int) float64 {
	result := 1.0
	for i := 0; i < places; i++ {
		result *= 10
	}
	return result
}

func parseActivityInput(input string) string {
	input = strings.TrimSpace(input)
	if parsed, err := url.Parse(input); err == nil && parsed.Path != "" && (parsed.Scheme != "" || strings.HasPrefix(input, "/")) {
		if querySlug := parsed.Query().Get("s"); querySlug != "" {
			input = querySlug
		} else {
			input = parsed.Path
		}
	}
	input = strings.Trim(input, "/")
	input = strings.TrimSuffix(input, "/")
	input = strings.TrimPrefix(input, "materi/")
	input = strings.TrimPrefix(input, "latihan/")
	input, _ = url.PathUnescape(input)
	return input
}
