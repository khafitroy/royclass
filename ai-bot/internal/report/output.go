package report

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Write renders a report directly to stdout or to the requested file. Office
// formats are generated locally on the VPS by LibreOffice; no public download
// route is created.
func Write(result Report, format, output string) error {
	format = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(format), "."))
	switch format {
	case "md", "markdown":
		return writeText(renderMarkdown(result), output, ".md")
	case "json":
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		return writeText(string(data)+"\n", output, ".json")
	case "csv":
		return writeText(renderCSV(result), output, ".csv")
	case "html":
		return writeText(renderHTML(result), output, ".html")
	case "xlsx":
		return writeXLSX(result, output)
	case "docx", "pdf":
		return writeOffice(result, format, output)
	default:
		return fmt.Errorf("format tidak didukung %q; gunakan md, json, csv, xlsx, docx, atau pdf", format)
	}
}

func writeText(content, output, defaultExtension string) error {
	if output == "" {
		_, err := io.WriteString(os.Stdout, content)
		return err
	}
	path := output
	if filepath.Ext(path) == "" {
		path += defaultExtension
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("gagal membuat folder laporan: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("gagal menulis laporan %s: %w", path, err)
	}
	fmt.Printf("Laporan dibuat: %s\n", path)
	return nil
}

func renderMarkdown(result Report) string {
	var builder strings.Builder
	builder.WriteString("# Laporan Hasil ")
	builder.WriteString(markdownText(result.Source))
	builder.WriteString("\n\n")
	builder.WriteString("- Aktivitas: **" + markdownText(activityTitle(result.Activity)) + "**\n")
	builder.WriteString("- Mode nilai: **" + markdownText(result.Metric) + "**\n")
	if result.ClassFilter != "" {
		builder.WriteString("- Filter kelas: **" + markdownText(result.ClassFilter) + "**\n")
	}
	if result.DateFilter != "" {
		builder.WriteString("- Filter tanggal: **" + markdownText(result.DateFilter) + "**\n")
	}
	builder.WriteString("- Dibuat: " + markdownText(result.GeneratedAt) + "\n\n")

	builder.WriteString("## Ringkasan\n\n")
	builder.WriteString(fmt.Sprintf("Master: **%d** siswa; sudah mengerjakan: **%d**; sedang mengerjakan: **%d**; belum mengerjakan: **%d**.\n\n", result.Summary.MasterStudents, result.Summary.SudahMengerjakan, result.Summary.SedangMengerjakan, result.Summary.BelumMengerjakan))
	builder.WriteString(fmt.Sprintf("Record submitted: **%d**; ongoing: **%d**; tidak tercocokkan: **%d**; nama berbeda: **%d**.\n\n", result.Summary.SubmittedRecords, result.Summary.OngoingRecords, result.Summary.UnmatchedRecords, result.Summary.NameMismatches))

	builder.WriteString("## Data siswa\n\n")
	builder.WriteString("| Status | Kelas | No | Nama master | NISN | Nilai | Skor | Percobaan | Mulai | Selesai | Nama isian | Cek nama | Record |\n")
	builder.WriteString("|---|---:|---:|---|---|---:|---:|---:|---|---|---|---|---|\n")
	for _, row := range result.Rows {
		builder.WriteString("| " + strings.Join([]string{
			markdownText(row.Status),
			markdownText(row.Class),
			markdownText(row.No),
			markdownText(row.Name),
			markdownText(row.NISN),
			markdownText(numberText(row.Percent)),
			markdownText(scoreText(row.Score, row.MaxScore)),
			markdownText(attemptText(row)),
			markdownText(row.StartedAt),
			markdownText(row.EndedAt),
			markdownText(row.EnteredName),
			markdownText(row.NameCheck),
			markdownText(row.RecordID),
		}, " | ") + " |\n")
	}
	if len(result.Rows) == 0 {
		builder.WriteString("\nTidak ada baris yang sesuai filter.\n")
	}

	writeReviewMarkdown(&builder, "Record yang perlu peninjauan", result.Unmatched, true)
	writeReviewMarkdown(&builder, "Pengecekan nama", result.NameReview, false)
	return builder.String()
}

func writeReviewMarkdown(builder *strings.Builder, heading string, reviews []ReviewRecord, unmatched bool) {
	if len(reviews) == 0 {
		return
	}
	builder.WriteString("\n## " + heading + "\n\n")
	if unmatched {
		builder.WriteString("| Alasan | Kelas isian | No | Nama isian | Status | Aktivitas | Record |\n")
		builder.WriteString("|---|---|---:|---|---|---|---|\n")
		for _, review := range reviews {
			builder.WriteString("| " + strings.Join([]string{
				markdownText(review.Reason), markdownText(review.RawClass), markdownText(review.RawNo),
				markdownText(review.EnteredName), markdownText(review.Status), markdownText(review.Activity), markdownText(review.RecordID),
			}, " | ") + " |\n")
		}
		return
	}
	builder.WriteString("| Kelas | No | Nama master | Nama isian | Status | Aktivitas | Record |\n")
	builder.WriteString("|---|---:|---|---|---|---|---|\n")
	for _, review := range reviews {
		builder.WriteString("| " + strings.Join([]string{
			markdownText(review.CanonicalClass), markdownText(review.RawNo), markdownText(review.MasterName),
			markdownText(review.EnteredName), markdownText(review.Status), markdownText(review.Activity), markdownText(review.RecordID),
		}, " | ") + " |\n")
	}
}

func renderCSV(result Report) string {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	_ = writer.Write([]string{"Status", "Kelas", "No Absen", "Nama Master", "NISN", "Nilai (%)", "Skor", "Percobaan", "Mulai", "Selesai", "Nama Isian", "Pengecekan Nama", "Record ID", "Aktivitas"})
	for _, row := range result.Rows {
		_ = writer.Write([]string{
			row.Status, row.Class, row.No, row.Name, row.NISN, numberText(row.Percent), scoreText(row.Score, row.MaxScore),
			attemptText(row), row.StartedAt, row.EndedAt, row.EnteredName, row.NameCheck, row.RecordID, row.Activity,
		})
	}
	writer.Flush()
	return buffer.String()
}

func renderHTML(result Report) string {
	var builder strings.Builder
	builder.WriteString(`<!doctype html><html lang="id"><head><meta charset="utf-8"><title>Laporan hasil</title><style>
body{font-family:Arial,sans-serif;font-size:11px;color:#222;margin:24px}h1{font-size:20px}h2{font-size:15px;margin-top:24px}.meta{line-height:1.6}.summary{display:flex;gap:10px;flex-wrap:wrap}.card{border:1px solid #bbb;border-radius:5px;padding:8px 12px}.card strong{display:block;font-size:16px}table{border-collapse:collapse;width:100%;margin-top:10px}th,td{border:1px solid #aaa;padding:4px 5px;text-align:left;vertical-align:top}th{background:#eee}td.num,th.num{text-align:right}.muted{color:#666}
</style></head><body>`)
	builder.WriteString("<h1>Laporan Hasil " + html.EscapeString(result.Source) + "</h1>")
	builder.WriteString("<div class=\"meta\"><b>Aktivitas:</b> " + html.EscapeString(activityTitle(result.Activity)) + "<br><b>Mode nilai:</b> " + html.EscapeString(result.Metric) + "<br>")
	if result.ClassFilter != "" {
		builder.WriteString("<b>Filter kelas:</b> " + html.EscapeString(result.ClassFilter) + "<br>")
	}
	if result.DateFilter != "" {
		builder.WriteString("<b>Filter tanggal:</b> " + html.EscapeString(result.DateFilter) + "<br>")
	}
	builder.WriteString("<b>Dibuat:</b> " + html.EscapeString(result.GeneratedAt) + "</div>")
	builder.WriteString("<div class=\"summary\">")
	writeCard(&builder, "Master", result.Summary.MasterStudents)
	writeCard(&builder, "Sudah", result.Summary.SudahMengerjakan)
	writeCard(&builder, "Sedang", result.Summary.SedangMengerjakan)
	writeCard(&builder, "Belum", result.Summary.BelumMengerjakan)
	writeCard(&builder, "Tidak cocok", result.Summary.UnmatchedRecords)
	builder.WriteString("</div>")
	builder.WriteString("<h2>Data siswa</h2><table><thead><tr>")
	for _, header := range []string{"Status", "Kelas", "No", "Nama master", "NISN", "Nilai (%)", "Skor", "Percobaan", "Mulai", "Selesai", "Nama isian", "Cek nama", "Record"} {
		builder.WriteString("<th>" + html.EscapeString(header) + "</th>")
	}
	builder.WriteString("</tr></thead><tbody>")
	for _, row := range result.Rows {
		builder.WriteString("<tr>")
		for _, value := range []string{row.Status, row.Class, row.No, row.Name, row.NISN, numberText(row.Percent), scoreText(row.Score, row.MaxScore), attemptText(row), row.StartedAt, row.EndedAt, row.EnteredName, row.NameCheck, row.RecordID} {
			builder.WriteString("<td>" + html.EscapeString(value) + "</td>")
		}
		builder.WriteString("</tr>")
	}
	if len(result.Rows) == 0 {
		builder.WriteString("<tr><td colspan=\"13\" class=\"muted\">Tidak ada baris yang sesuai filter.</td></tr>")
	}
	builder.WriteString("</tbody></table>")
	writeReviewHTML(&builder, "Record yang perlu peninjauan", result.Unmatched, true)
	writeReviewHTML(&builder, "Pengecekan nama", result.NameReview, false)
	builder.WriteString("</body></html>")
	return builder.String()
}

func writeCard(builder *strings.Builder, label string, value int) {
	builder.WriteString("<div class=\"card\"><span>" + html.EscapeString(label) + "</span><strong>" + strconv.Itoa(value) + "</strong></div>")
}

func writeReviewHTML(builder *strings.Builder, heading string, reviews []ReviewRecord, unmatched bool) {
	if len(reviews) == 0 {
		return
	}
	builder.WriteString("<h2>" + html.EscapeString(heading) + "</h2><table><thead><tr>")
	headers := []string{"Alasan", "Kelas isian", "No", "Nama isian", "Status", "Aktivitas", "Record"}
	if !unmatched {
		headers = []string{"Kelas", "No", "Nama master", "Nama isian", "Status", "Aktivitas", "Record"}
	}
	for _, header := range headers {
		builder.WriteString("<th>" + html.EscapeString(header) + "</th>")
	}
	builder.WriteString("</tr></thead><tbody>")
	for _, review := range reviews {
		values := []string{review.Reason, review.RawClass, review.RawNo, review.EnteredName, review.Status, review.Activity, review.RecordID}
		if !unmatched {
			values = []string{review.CanonicalClass, review.RawNo, review.MasterName, review.EnteredName, review.Status, review.Activity, review.RecordID}
		}
		builder.WriteString("<tr>")
		for _, value := range values {
			builder.WriteString("<td>" + html.EscapeString(value) + "</td>")
		}
		builder.WriteString("</tr>")
	}
	builder.WriteString("</tbody></table>")
}

func markdownText(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "|", "\\|"), "\n", " ")
}

func numberText(value *float64) string {
	if value == nil {
		return "-"
	}
	return strconv.FormatFloat(*value, 'f', 2, 64)
}

func scoreText(score, maxScore *float64) string {
	if score == nil {
		return "-"
	}
	if maxScore == nil {
		return numberText(score)
	}
	return numberText(score) + "/" + numberText(maxScore)
}

func attemptText(row ReportRow) string {
	if row.Attempt <= 0 {
		if row.AttemptCount > 0 {
			return fmt.Sprintf("%d", row.AttemptCount)
		}
		return "-"
	}
	if row.AttemptCount > 0 {
		return fmt.Sprintf("%d/%d", row.Attempt, row.AttemptCount)
	}
	return strconv.Itoa(row.Attempt)
}

func writeOffice(result Report, format, output string) error {
	if output == "" {
		output = "laporan-" + safeFilename(activityTitle(result.Activity)) + "-" + time.Now().UTC().Format("20060102-150405") + "." + format
	} else if filepath.Ext(output) == "" {
		output += "." + format
	}
	outputPath, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("gagal membuat folder laporan: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "gezy-report-source-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	profileDir, err := os.MkdirTemp("", "gezy-report-libreoffice-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(profileDir)

	sourceExtension := ".html"
	content := renderHTML(result)
	if format == "xlsx" {
		sourceExtension = ".csv"
		content = renderCSV(result)
	}
	sourcePath := filepath.Join(tempDir, "gezy-report-source"+sourceExtension)
	if err := os.WriteFile(sourcePath, []byte(content), 0644); err != nil {
		return err
	}

	libreoffice, err := exec.LookPath("libreoffice")
	if err != nil {
		return fmt.Errorf("LibreOffice tidak ditemukan untuk membuat %s: %w", format, err)
	}
	profileURI := "file://" + profileDir
	convertFormat := format
	if format == "docx" {
		convertFormat = "docx:Office Open XML Text"
	}
	args := []string{"--headless", "-env:UserInstallation=" + profileURI, "--convert-to", convertFormat, "--outdir", filepath.Dir(outputPath), sourcePath}
	command := exec.Command(libreoffice, args...)
	combined, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("LibreOffice gagal membuat %s: %w (%s)", format, err, strings.TrimSpace(string(combined)))
	}

	generated := filepath.Join(filepath.Dir(outputPath), "gezy-report-source."+format)
	if _, err := os.Stat(generated); err != nil {
		return fmt.Errorf("LibreOffice selesai tetapi file hasil tidak ditemukan: %w (%s)", err, strings.TrimSpace(string(combined)))
	}
	if err := os.Rename(generated, outputPath); err != nil {
		return fmt.Errorf("gagal menyimpan %s: %w", outputPath, err)
	}
	fmt.Printf("Laporan dibuat: %s\n", outputPath)
	return nil
}

// writeXLSX writes a small standards-compliant XLSX workbook directly. The
// VPS LibreOffice installation can create PDF/DOCX, but its CSV import filter
// is unavailable, so Excel output does not depend on that filter.
func writeXLSX(result Report, output string) error {
	if output == "" {
		output = "laporan-" + safeFilename(activityTitle(result.Activity)) + "-" + time.Now().UTC().Format("20060102-150405") + ".xlsx"
	} else if filepath.Ext(output) == "" {
		output += ".xlsx"
	}
	outputPath, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("gagal membuat folder laporan: %w", err)
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("gagal membuat XLSX %s: %w", outputPath, err)
	}
	zipWriter := zip.NewWriter(file)
	entries := map[string]string{
		"[Content_Types].xml":        `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`,
		"_rels/.rels":                `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml":            `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Laporan" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
	}
	for name, content := range entries {
		if err := writeZipEntry(zipWriter, name, []byte(content)); err != nil {
			_ = zipWriter.Close()
			_ = file.Close()
			return err
		}
	}
	if err := writeZipEntry(zipWriter, "xl/worksheets/sheet1.xml", xlsxSheet(result)); err != nil {
		_ = zipWriter.Close()
		_ = file.Close()
		return err
	}
	if err := zipWriter.Close(); err != nil {
		_ = file.Close()
		return fmt.Errorf("gagal menutup XLSX: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("gagal menutup file XLSX: %w", err)
	}
	fmt.Printf("Laporan dibuat: %s\n", outputPath)
	return nil
}

func writeZipEntry(writer *zip.Writer, name string, content []byte) error {
	entry, err := writer.Create(name)
	if err != nil {
		return fmt.Errorf("gagal membuat entry XLSX %s: %w", name, err)
	}
	if _, err := entry.Write(content); err != nil {
		return fmt.Errorf("gagal menulis entry XLSX %s: %w", name, err)
	}
	return nil
}

func xlsxSheet(result Report) []byte {
	rows := [][]string{
		{"Laporan hasil", result.Source},
		{"Aktivitas", activityTitle(result.Activity)},
		{"Mode nilai", result.Metric},
		{"Master", strconv.Itoa(result.Summary.MasterStudents)},
		{"Sudah mengerjakan", strconv.Itoa(result.Summary.SudahMengerjakan)},
		{"Sedang mengerjakan", strconv.Itoa(result.Summary.SedangMengerjakan)},
		{"Belum mengerjakan", strconv.Itoa(result.Summary.BelumMengerjakan)},
		{},
		{"Status", "Kelas", "No Absen", "Nama Master", "NISN", "Nilai (%)", "Skor", "Percobaan", "Mulai", "Selesai", "Nama Isian", "Pengecekan Nama", "Record ID", "Aktivitas"},
	}
	for _, row := range result.Rows {
		rows = append(rows, []string{
			row.Status, row.Class, row.No, row.Name, row.NISN, numberText(row.Percent), scoreText(row.Score, row.MaxScore),
			attemptText(row), row.StartedAt, row.EndedAt, row.EnteredName, row.NameCheck, row.RecordID, row.Activity,
		})
	}
	if len(result.Unmatched) > 0 {
		rows = append(rows, []string{}, []string{"Record yang perlu peninjauan"})
		rows = append(rows, []string{"Alasan", "Kelas Isian", "No", "Nama Isian", "Status", "Aktivitas", "Record"})
		for _, review := range result.Unmatched {
			rows = append(rows, []string{review.Reason, review.RawClass, review.RawNo, review.EnteredName, review.Status, review.Activity, review.RecordID})
		}
	}
	if len(result.NameReview) > 0 {
		rows = append(rows, []string{}, []string{"Pengecekan nama"})
		rows = append(rows, []string{"Kelas", "No", "Nama Master", "Nama Isian", "Status", "Aktivitas", "Record"})
		for _, review := range result.NameReview {
			rows = append(rows, []string{review.CanonicalClass, review.RawNo, review.MasterName, review.EnteredName, review.Status, review.Activity, review.RecordID})
		}
	}

	var builder strings.Builder
	builder.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for rowIndex, row := range rows {
		builder.WriteString(`<row r="` + strconv.Itoa(rowIndex+1) + `">`)
		for columnIndex, value := range row {
			builder.WriteString(`<c r="` + columnName(columnIndex+1) + strconv.Itoa(rowIndex+1) + `" t="inlineStr"><is><t xml:space="preserve">`)
			_ = xml.EscapeText(&builder, []byte(value))
			builder.WriteString(`</t></is></c>`)
		}
		builder.WriteString(`</row>`)
	}
	builder.WriteString(`</sheetData></worksheet>`)
	return []byte(builder.String())
}

func columnName(number int) string {
	var builder strings.Builder
	for number > 0 {
		number--
		builder.WriteByte(byte('A' + number%26))
		number /= 26
	}
	result := []byte(builder.String())
	for left, right := 0, len(result)-1; left < right; left, right = left+1, right-1 {
		result[left], result[right] = result[right], result[left]
	}
	return string(result)
}

func safeFilename(value string) string {
	var builder strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			builder.WriteRune('-')
			lastDash = true
		}
	}
	clean := strings.Trim(builder.String(), "-")
	if clean == "" {
		return "hasil"
	}
	return clean
}
