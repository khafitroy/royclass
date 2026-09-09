package report

import (
	"fmt"
	"sort"
	"strings"

	"gezyclass/ai-bot/internal/pocketbase"
)

func loadActivities(client *pocketbase.Client, kind string) ([]Activity, error) {
	collection := "exams"
	if kind == "latihan" {
		collection = "latihan_soal"
	}
	records, err := client.ListAll(collection, "")
	if err != nil {
		return nil, fmt.Errorf("gagal membaca daftar aktivitas %s: %w", kind, err)
	}
	activities := make([]Activity, 0, len(records))
	for _, record := range records {
		activity := Activity{
			Kind:  kind,
			ID:    valueString(record, "id"),
			Title: valueString(record, "title"),
			Slug:  valueString(record, "slug"),
		}
		if kind == "latihan" {
			activity.Title = valueString(record, "judul")
			if activity.Title == "" {
				activity.Title = activity.Slug
			}
		}
		if activity.Title == "" {
			activity.Title = activity.ID
		}
		activities = append(activities, activity)
	}
	sort.SliceStable(activities, func(i, j int) bool {
		left := strings.ToLower(activityTitle(activities[i]))
		right := strings.ToLower(activityTitle(activities[j]))
		return left < right
	})
	return activities, nil
}

func resolveActivity(input string, activities []Activity, metric string) (Activity, error) {
	clean := parseActivityInput(input)
	if clean == "" || strings.EqualFold(clean, "all") || clean == "*" {
		if metric != "all" {
			return Activity{}, errorsForActivityAggregate()
		}
		kind := "aktivitas"
		if len(activities) > 0 {
			kind = activities[0].Kind
		}
		return Activity{Kind: kind, ID: "*", Title: "Semua aktivitas " + kind}, nil
	}

	var exact []Activity
	for _, activity := range activities {
		if strings.EqualFold(clean, activity.ID) ||
			strings.EqualFold(clean, activity.Slug) ||
			strings.EqualFold(clean, activity.Title) {
			exact = append(exact, activity)
		}
	}
	if len(exact) == 1 {
		return exact[0], nil
	}
	if len(exact) > 1 {
		return Activity{}, fmt.Errorf("aktivitas %q cocok dengan beberapa record; gunakan ID atau slug yang tepat", input)
	}

	var partial []Activity
	needle := strings.ToLower(clean)
	for _, activity := range activities {
		if strings.Contains(strings.ToLower(activity.Title), needle) || strings.Contains(strings.ToLower(activity.Slug), needle) {
			partial = append(partial, activity)
		}
	}
	if len(partial) == 1 {
		return partial[0], nil
	}
	if len(partial) > 1 {
		return Activity{}, fmt.Errorf("aktivitas %q cocok dengan %d record; gunakan ID/slug lengkap atau --list", input, len(partial))
	}

	return Activity{}, fmt.Errorf("aktivitas %q tidak ditemukan; jalankan ai-bot report --type %s --list", input, activitiesKind(activities))
}

func errorsForActivityAggregate() error {
	return fmt.Errorf("--activity wajib diisi untuk mode highest, average, atau latest")
}

func activitiesKind(activities []Activity) string {
	if len(activities) > 0 && activities[0].Kind == "latihan" {
		return "latihan"
	}
	return "cbt"
}

func loadAttempts(client *pocketbase.Client, kind string, activities []Activity, selected Activity, date string) ([]Attempt, error) {
	collection := "exam_sessions"
	if kind == "latihan" {
		collection = "latihan_sesi"
	}
	records, err := client.ListAll(collection, "")
	if err != nil {
		return nil, fmt.Errorf("gagal membaca record %s: %w", collection, err)
	}

	byID := make(map[string]Activity)
	bySlug := make(map[string]Activity)
	for _, activity := range activities {
		byID[activity.ID] = activity
		if activity.Slug != "" {
			bySlug[strings.ToLower(activity.Slug)] = activity
		}
	}

	var attempts []Attempt
	for _, record := range records {
		activityID := valueString(record, "exam_id")
		activitySlug := valueString(record, "latihan_slug")
		activity := Activity{Kind: kind}
		if kind == "cbt" {
			activity = byID[activityID]
			if activity.ID == "" {
				activity = Activity{Kind: kind, ID: activityID, Title: activityID}
			}
			if selected.ID != "*" && !strings.EqualFold(activityID, selected.ID) {
				continue
			}
		} else {
			activity = bySlug[strings.ToLower(activitySlug)]
			if activity.Slug == "" {
				activity = Activity{Kind: kind, Slug: activitySlug, Title: activitySlug}
			}
			if selected.ID != "*" && !strings.EqualFold(activitySlug, selected.Slug) {
				continue
			}
		}

		status := rawStatus(valueString(record, "status"))
		score, hasScore := numberValue(record, "total_score")
		maxScore, hasMaxScore := numberValue(record, "max_score")
		var scorePointer, maxPointer, percentPointer *float64
		// Ongoing sessions often contain zero placeholders. A score is shown
		// only for submitted records.
		if status == "submitted" && hasScore {
			scorePointer = floatPointer(score)
			if hasMaxScore {
				maxPointer = floatPointer(maxScore)
				if maxScore > 0 {
					percentPointer = floatPointer(round(score / maxScore * 100))
				}
			}
		}
		classRaw := valueString(record, "kelas")
		noRaw := valueString(record, "no_absen")
		attempt := 0
		if number, ok := numberValue(record, "attempt"); ok {
			attempt = int(number)
		}
		current := Attempt{
			ID:            valueString(record, "id"),
			Kind:          kind,
			ActivityID:    activity.ID,
			ActivityTitle: activityTitle(activity),
			RawClass:      classRaw,
			Class:         canonicalClass(classRaw),
			RawNo:         noRaw,
			No:            normalizeNo(noRaw),
			EnteredName:   valueString(record, "nama"),
			School:        valueString(record, "sekolah"),
			Status:        status,
			StartedAt:     valueString(record, "started_at"),
			EndedAt:       valueString(record, "ended_at"),
			Attempt:       attempt,
			Score:         scorePointer,
			MaxScore:      maxPointer,
			Percent:       percentPointer,
		}
		current.Key = ""
		if current.Class != "" && current.No != "" {
			current.Key = studentKey(current.Class, current.No)
		}
		if dateMatches(current, date) {
			attempts = append(attempts, current)
		}
	}
	return attempts, nil
}

func reviewForUnmatched(attempt Attempt) ReviewRecord {
	reason := "tidak ditemukan di master kelas + nomor absen"
	if attempt.Class == "" {
		reason = "kelas tidak dikenali"
	} else if attempt.No == "" {
		reason = "nomor absen kosong"
	}
	return ReviewRecord{
		Reason:         reason,
		RecordID:       attempt.ID,
		Activity:       attempt.ActivityTitle,
		RawClass:       attempt.RawClass,
		CanonicalClass: attempt.Class,
		RawNo:          attempt.RawNo,
		EnteredName:    attempt.EnteredName,
		Status:         displayStatus(attempt.Status),
	}
}

func reviewForNameMismatch(attempt Attempt, student Student) ReviewRecord {
	return ReviewRecord{
		Reason:         "nama isian berbeda dari master; pencocokan tetap memakai kelas + nomor absen",
		RecordID:       attempt.ID,
		Activity:       attempt.ActivityTitle,
		RawClass:       attempt.RawClass,
		CanonicalClass: attempt.Class,
		RawNo:          attempt.RawNo,
		EnteredName:    attempt.EnteredName,
		MasterName:     student.Name,
		Status:         displayStatus(attempt.Status),
	}
}

func absentRow(student Student, activity string) ReportRow {
	return ReportRow{
		Status:   "Belum mengerjakan",
		Class:    student.Class,
		No:       student.No,
		Name:     student.Name,
		NISN:     student.NISN,
		Gender:   student.Gender,
		Activity: activity,
	}
}

func nameCheck(attempt Attempt, student Student) string {
	if strings.TrimSpace(attempt.EnteredName) == "" {
		return "tidak diisi"
	}
	if normalizeName(attempt.EnteredName) == normalizeName(student.Name) {
		return "cocok"
	}
	return "berbeda"
}

func attemptRow(attempt Attempt, student Student, position, total int) ReportRow {
	return ReportRow{
		Status:       displayStatus(attempt.Status),
		Class:        student.Class,
		No:           student.No,
		Name:         student.Name,
		NISN:         student.NISN,
		Gender:       student.Gender,
		Activity:     attempt.ActivityTitle,
		RecordID:     attempt.ID,
		Attempt:      position,
		AttemptCount: total,
		Score:        attempt.Score,
		MaxScore:     attempt.MaxScore,
		Percent:      attempt.Percent,
		StartedAt:    attempt.StartedAt,
		EndedAt:      attempt.EndedAt,
		EnteredName:  attempt.EnteredName,
		NameCheck:    nameCheck(attempt, student),
		RawClass:     attempt.RawClass,
		RawNo:        attempt.RawNo,
	}
}

func aggregateRow(attempts []Attempt, student Student, metric string) ReportRow {
	submitted := make([]Attempt, 0, len(attempts))
	for _, attempt := range attempts {
		if attempt.Status == "submitted" {
			submitted = append(submitted, attempt)
		}
	}
	if len(submitted) == 0 {
		// There is no completed score. Showing the latest ongoing session keeps
		// the requested "sedang mengerjakan" state visible.
		chosen := attempts[len(attempts)-1]
		row := attemptRow(chosen, student, positionOf(attempts, chosen)+1, len(attempts))
		row.Status = displayStatus(chosen.Status)
		return row
	}

	chosen := submitted[len(submitted)-1]
	chosenPosition := positionOf(attempts, chosen) + 1
	if metric == "highest" {
		chosen = submitted[0]
		for _, candidate := range submitted[1:] {
			if scoreRank(candidate) > scoreRank(chosen) {
				chosen = candidate
			}
		}
		chosenPosition = positionOf(attempts, chosen) + 1
	}
	if metric == "latest" {
		chosen = submitted[len(submitted)-1]
		chosenPosition = positionOf(attempts, chosen) + 1
	}

	row := attemptRow(chosen, student, chosenPosition, len(attempts))
	row.Status = "Sudah mengerjakan"
	if metric == "average" {
		row.RecordID = ""
		row.Attempt = 0
		row.Score, row.MaxScore, row.Percent = averageScores(submitted)
		row.StartedAt = submitted[0].StartedAt
		row.EndedAt = submitted[len(submitted)-1].EndedAt
	}
	return row
}

func positionOf(attempts []Attempt, wanted Attempt) int {
	for index, attempt := range attempts {
		if attempt.ID == wanted.ID {
			return index
		}
	}
	return len(attempts) - 1
}

func scoreRank(attempt Attempt) float64 {
	if attempt.Percent != nil {
		return *attempt.Percent
	}
	if attempt.Score != nil {
		return *attempt.Score
	}
	return -1
}

func averageScores(attempts []Attempt) (*float64, *float64, *float64) {
	var scoreSum, maxSum, percentSum float64
	var scoreCount, maxCount, percentCount int
	for _, attempt := range attempts {
		if attempt.Score != nil {
			scoreSum += *attempt.Score
			scoreCount++
		}
		if attempt.MaxScore != nil {
			maxSum += *attempt.MaxScore
			maxCount++
		}
		if attempt.Percent != nil {
			percentSum += *attempt.Percent
			percentCount++
		}
	}
	var score, maxScore, percent *float64
	if scoreCount > 0 {
		value := round(scoreSum / float64(scoreCount))
		score = &value
	}
	if maxCount > 0 {
		value := round(maxSum / float64(maxCount))
		maxScore = &value
	}
	if percentCount > 0 {
		value := round(percentSum / float64(percentCount))
		percent = &value
	}
	return score, maxScore, percent
}
