package staging

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"gezyclass/ai-bot/internal/pocketbase"
)

type Document struct {
	Path     string
	Kind     string
	SourceID string
	Meta     map[string]any
	Body     string
	Hash     string
}

type State struct {
	Records map[string]StateRecord `json:"records"`
}

type StateRecord struct {
	Collection string    `json:"collection"`
	RecordID   string    `json:"record_id"`
	Hash       string    `json:"hash"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type SeedOptions struct {
	StagingDir string
	StatePath  string
	DryRun     bool
	BuildHugo  bool
	HugoDir    string
	PublicDir  string
}

type SeedResult struct {
	Processed int
	Created   int
	Updated   int
	Skipped   int
}

var kindOrder = map[string]int{
	"category":      10,
	"tag":           10,
	"class":         20,
	"chapter":       30,
	"subchapter":    40,
	"material":      50,
	"article":       50,
	"question":      50,
	"example":       60,
	"exercise":      60,
	"exam":          70,
	"exam_question": 80,
	"exam_token":    80,
}

func LoadDocuments(root string) ([]Document, error) {
	var docs []Document
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		if strings.EqualFold(d.Name(), "README.md") {
			return nil
		}
		doc, err := ParseDocument(path)
		if err != nil {
			return err
		}
		docs = append(docs, doc)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(docs, func(i, j int) bool {
		li := kindOrder[docs[i].Kind]
		lj := kindOrder[docs[j].Kind]
		if li != lj {
			return li < lj
		}
		if docs[i].Kind != docs[j].Kind {
			return docs[i].Kind < docs[j].Kind
		}
		return docs[i].SourceID < docs[j].SourceID
	})
	return docs, nil
}

func ParseDocument(path string) (Document, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Document{}, err
	}
	text := string(raw)
	if !strings.HasPrefix(text, "---\n") {
		return Document{}, fmt.Errorf("%s: missing YAML front matter", path)
	}
	rest := strings.TrimPrefix(text, "---\n")
	metaEnd := strings.Index(rest, "\n---\n")
	if metaEnd < 0 {
		return Document{}, fmt.Errorf("%s: unterminated YAML front matter", path)
	}
	metaText := rest[:metaEnd]
	body := rest[metaEnd+len("\n---\n"):]

	meta := make(map[string]any)
	if err := yaml.Unmarshal([]byte(metaText), &meta); err != nil {
		return Document{}, fmt.Errorf("%s: invalid YAML front matter: %w", path, err)
	}
	kind := stringValue(meta["kind"])
	if kind == "" {
		return Document{}, fmt.Errorf("%s: kind is required", path)
	}
	sourceID := stringValue(meta["source_id"])
	if sourceID == "" {
		sourceID = stringValue(meta["id"])
	}
	if sourceID == "" {
		sourceID = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if sourceID == "" {
		return Document{}, fmt.Errorf("%s: source_id is required", path)
	}

	return Document{
		Path:     path,
		Kind:     kind,
		SourceID: sourceID,
		Meta:     meta,
		Body:     body,
		Hash:     hashDocument(metaText + "\n---\n" + body),
	}, nil
}

func LoadState(path string) (State, error) {
	state := State{Records: map[string]StateRecord{}}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return state, err
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return State{}, err
	}
	if state.Records == nil {
		state.Records = map[string]StateRecord{}
	}
	return state, nil
}

func SaveState(path string, state State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

type Importer struct {
	Client *pocketbase.Client
	State  State
	Opts   SeedOptions
}

func NewImporter(client *pocketbase.Client, opts SeedOptions) (*Importer, error) {
	if opts.StagingDir == "" {
		opts.StagingDir = "content-staging"
	}
	if opts.StatePath == "" {
		opts.StatePath = filepath.Join(opts.StagingDir, ".seeder-state.json")
	}
	state, err := LoadState(opts.StatePath)
	if err != nil {
		return nil, err
	}
	return &Importer{Client: client, State: state, Opts: opts}, nil
}

func (i *Importer) Run() (SeedResult, error) {
	docs, err := LoadDocuments(i.Opts.StagingDir)
	if err != nil {
		return SeedResult{}, err
	}
	result := SeedResult{}
	for _, doc := range docs {
		result.Processed++
		if err := i.importDocument(doc, &result); err != nil {
			return result, err
		}
	}
	if !i.Opts.DryRun {
		if err := SaveState(i.Opts.StatePath, i.State); err != nil {
			return result, err
		}
	}
	if i.Opts.BuildHugo && !i.Opts.DryRun {
		if err := BuildHugo(i.Opts.HugoDir, i.Opts.PublicDir); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (i *Importer) importDocument(doc Document, result *SeedResult) error {
	switch doc.Kind {
	case "class":
		return i.upsert(doc, "classes", i.classPayload(doc), result)
	case "category":
		return i.upsert(doc, "categories", i.categoryPayload(doc), result)
	case "tag":
		return i.upsert(doc, "tags", i.tagPayload(doc), result)
	case "chapter":
		return i.upsert(doc, "chapters", i.chapterPayload(doc), result)
	case "subchapter":
		return i.upsert(doc, "subchapters", i.subchapterPayload(doc), result)
	case "material":
		return i.upsert(doc, "materials", i.materialPayload(doc), result)
	case "example":
		return i.upsert(doc, "examples", i.examplePayload(doc), result)
	case "exercise":
		return i.upsert(doc, "exercises", i.exercisePayload(doc), result)
	case "article":
		return i.upsert(doc, "articles", i.articlePayload(doc), result)
	case "question":
		return i.upsert(doc, "questions", i.questionPayload(doc), result)
	case "exam":
		return i.upsertExam(doc, result)
	default:
		return fmt.Errorf("%s: unsupported kind %q", doc.Path, doc.Kind)
	}
}

func (i *Importer) upsert(doc Document, collection string, payload map[string]any, result *SeedResult) error {
	key := stateKey(doc.Kind, doc.SourceID)
	if existing, ok := i.State.Records[key]; ok && existing.Collection == collection && existing.RecordID != "" {
		if i.Opts.DryRun {
			fmt.Printf("[dry-run] update %s %s\n", collection, existing.RecordID)
			result.Skipped++
			return nil
		}
		if _, err := i.Client.Update(collection, existing.RecordID, payload); err != nil {
			return fmt.Errorf("%s: update %s failed: %w", doc.Path, collection, err)
		}
		i.State.Records[key] = StateRecord{Collection: collection, RecordID: existing.RecordID, Hash: doc.Hash, UpdatedAt: time.Now()}
		result.Updated++
		fmt.Printf("updated %-12s %s\n", collection, doc.SourceID)
		return nil
	}
	if i.Opts.DryRun {
		fmt.Printf("[dry-run] create %s\n", collection)
		result.Skipped++
		return nil
	}
	record, err := i.Client.Create(collection, payload)
	if err != nil {
		return fmt.Errorf("%s: create %s failed: %w", doc.Path, collection, err)
	}
	recordID := stringValue(record["id"])
	if recordID == "" {
		return fmt.Errorf("%s: %s create returned empty id", doc.Path, collection)
	}
	i.State.Records[key] = StateRecord{Collection: collection, RecordID: recordID, Hash: doc.Hash, UpdatedAt: time.Now()}
	result.Created++
	fmt.Printf("created %-12s %s\n", collection, doc.SourceID)
	return nil
}

func (i *Importer) upsertExam(doc Document, result *SeedResult) error {
	payload := i.examPayload(doc)
	key := stateKey(doc.Kind, doc.SourceID)
	if existing, ok := i.State.Records[key]; ok && existing.RecordID != "" {
		if i.Opts.DryRun {
			fmt.Printf("[dry-run] update exams %s\n", existing.RecordID)
			result.Skipped++
			return nil
		}
		// PocketBase treats false as blank for this required legacy field. Keep
		// the source setting authoritative in the result hook, but omit the
		// field on updates so an explicit false can still be synchronized.
		examFields := payload.examFields
		if !boolValue(doc.Meta["show_explanation_after"], true) {
			examFields = make(map[string]any, len(payload.examFields))
			for key, value := range payload.examFields {
				examFields[key] = value
			}
			delete(examFields, "show_explanation_after")
		}
		if _, err := i.Client.Update("exams", existing.RecordID, examFields); err != nil {
			return fmt.Errorf("%s: update exam failed: %w", doc.Path, err)
		}
		if err := i.clearExamRelations(existing.RecordID); err != nil {
			return err
		}
		if err := i.syncExamRelations(existing.RecordID, payload); err != nil {
			return err
		}
		i.State.Records[key] = StateRecord{Collection: "exams", RecordID: existing.RecordID, Hash: doc.Hash, UpdatedAt: time.Now()}
		result.Updated++
		fmt.Printf("updated %-12s %s\n", "exams", doc.SourceID)
		return nil
	}
	if i.Opts.DryRun {
		fmt.Printf("[dry-run] create exams\n")
		result.Skipped++
		return nil
	}
	record, err := i.Client.Create("exams", payload.examFields)
	if err != nil {
		return fmt.Errorf("%s: create exam failed: %w", doc.Path, err)
	}
	examID := stringValue(record["id"])
	if examID == "" {
		return fmt.Errorf("%s: exam create returned empty id", doc.Path)
	}
	if err := i.syncExamRelations(examID, payload); err != nil {
		return err
	}
	i.State.Records[key] = StateRecord{Collection: "exams", RecordID: examID, Hash: doc.Hash, UpdatedAt: time.Now()}
	result.Created++
	fmt.Printf("created %-12s %s\n", "exams", doc.SourceID)
	return nil
}

func (i *Importer) clearExamRelations(examID string) error {
	examQuestions, err := i.Client.List("exam_questions", fmt.Sprintf("exam_id='%s'", examID))
	if err != nil {
		return fmt.Errorf("list exam_questions failed: %w", err)
	}
	for _, row := range examQuestions {
		if id := stringValue(row["id"]); id != "" {
			if _, err := i.Client.Delete("exam_questions", id); err != nil {
				return fmt.Errorf("delete exam_question %s failed: %w", id, err)
			}
		}
	}
	examTokens, err := i.Client.List("exam_tokens", fmt.Sprintf("exam_id='%s' && is_used=false", examID))
	if err != nil {
		return fmt.Errorf("list exam_tokens failed: %w", err)
	}
	for _, row := range examTokens {
		if id := stringValue(row["id"]); id != "" {
			if _, err := i.Client.Delete("exam_tokens", id); err != nil {
				return fmt.Errorf("delete exam_token %s failed: %w", id, err)
			}
		}
	}
	return nil
}

type examImport struct {
	examFields   map[string]any
	questions    []string
	tokens       []string
	points       int
	sharedTokens bool
}

func (i *Importer) syncExamRelations(examID string, payload examImport) error {
	for idx, qRef := range payload.questions {
		qID, err := i.resolveRecordID("question", qRef)
		if err != nil {
			// resolveSourceIDList may already have converted a source_id to
			// a PocketBase record ID. It may also contain a direct record ID.
			qID = qRef
		}
		if _, err := i.Client.Create("exam_questions", map[string]any{
			"exam_id":     examID,
			"question_id": qID,
			"order_num":   idx + 1,
			"points":      payload.points,
		}); err != nil {
			return fmt.Errorf("exam_questions link failed for %s: %w", qRef, err)
		}
	}
	for _, token := range payload.tokens {
		existing, err := i.Client.List("exam_tokens", fmt.Sprintf("exam_id='%s' && token='%s'", examID, strings.ReplaceAll(token, "'", "''")))
		if err != nil {
			return fmt.Errorf("list exam token %s failed: %w", token, err)
		}
		if len(existing) > 0 {
			if payload.sharedTokens {
				id := stringValue(existing[0]["id"])
				if _, err := i.Client.Update("exam_tokens", id, map[string]any{"is_shared": true}); err != nil {
					return fmt.Errorf("update exam token %s failed: %w", token, err)
				}
			}
			continue
		}
		if _, err := i.Client.Create("exam_tokens", map[string]any{
			"exam_id":   examID,
			"token":     token,
			"is_used":   false,
			"is_shared": payload.sharedTokens,
		}); err != nil {
			return fmt.Errorf("exam token %s failed: %w", token, err)
		}
	}
	return nil
}

func (i *Importer) resolveRecordID(kind, sourceID string) (string, error) {
	key := stateKey(kind, sourceID)
	entry, ok := i.State.Records[key]
	if !ok || entry.RecordID == "" {
		return "", fmt.Errorf("missing imported record for %s:%s", kind, sourceID)
	}
	return entry.RecordID, nil
}

func stateKey(kind, sourceID string) string {
	return kind + ":" + sourceID
}

func hashDocument(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func stringValue(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case fmt.Stringer:
		return strings.TrimSpace(t.String())
	default:
		return ""
	}
}

func intValue(v any, fallback int) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case float32:
		return int(t)
	case string:
		var n int
		_, err := fmt.Sscanf(t, "%d", &n)
		if err == nil {
			return n
		}
	}
	return fallback
}

func boolValue(v any, fallback bool) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "true", "1", "yes", "y":
			return true
		case "false", "0", "no", "n":
			return false
		}
	}
	return fallback
}

func stringSlice(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s := stringValue(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func payloadMeta(doc Document) map[string]any {
	_ = doc
	return nil
}

func requireString(meta map[string]any, key string) (string, error) {
	v := stringValue(meta[key])
	if v == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return v, nil
}

func requireRef(meta map[string]any, key string) (string, error) {
	v := stringValue(meta[key])
	if v == "" {
		return "", fmt.Errorf("%s reference is required", key)
	}
	return v, nil
}

func (i *Importer) classPayload(doc Document) map[string]any {
	name := stringValue(doc.Meta["name"])
	if name == "" {
		name = "Kelas " + stringValue(doc.Meta["order"])
	}
	return map[string]any{
		"name":        name,
		"slug":        stringValue(doc.Meta["slug"]),
		"description": stringValue(doc.Meta["description"]),
		"order":       intValue(doc.Meta["order"], 0),
	}
}

func (i *Importer) categoryPayload(doc Document) map[string]any {
	return map[string]any{
		"name": stringValue(doc.Meta["name"]),
		"slug": stringValue(doc.Meta["slug"]),
	}
}

func (i *Importer) tagPayload(doc Document) map[string]any {
	return map[string]any{
		"name": stringValue(doc.Meta["name"]),
		"slug": stringValue(doc.Meta["slug"]),
	}
}

func (i *Importer) chapterPayload(doc Document) map[string]any {
	return map[string]any{
		"class_id":    i.resolveRelation(doc.Meta, "class_source_id", "class_id", "class"),
		"name":        stringValue(doc.Meta["name"]),
		"slug":        stringValue(doc.Meta["slug"]),
		"order":       intValue(doc.Meta["order"], 1),
		"description": stringValue(doc.Meta["description"]),
	}
}

func (i *Importer) subchapterPayload(doc Document) map[string]any {
	return map[string]any{
		"chapter_id": i.resolveRelation(doc.Meta, "chapter_source_id", "chapter_id", "chapter"),
		"name":       stringValue(doc.Meta["name"]),
		"slug":       stringValue(doc.Meta["slug"]),
		"order":      intValue(doc.Meta["order"], 1),
	}
}

func (i *Importer) materialPayload(doc Document) map[string]any {
	content := strings.TrimSpace(doc.Body)
	if content == "" {
		content = stringValue(doc.Meta["content"])
	}
	return map[string]any{
		"subchapter_id": i.resolveRelation(doc.Meta, "subchapter_source_id", "subchapter_id", "subchapter"),
		"title":         stringValue(doc.Meta["title"]),
		"content":       content,
		"order":         intValue(doc.Meta["order"], 1),
	}
}

func (i *Importer) examplePayload(doc Document) map[string]any {
	return map[string]any{
		"material_id": i.resolveRelation(doc.Meta, "material_source_id", "material_id", "material"),
		"title":       stringValue(doc.Meta["title"]),
		"question":    strings.TrimSpace(doc.Body),
		"solution":    stringValue(doc.Meta["solution"]),
		"order":       intValue(doc.Meta["order"], 1),
	}
}

func (i *Importer) exercisePayload(doc Document) map[string]any {
	return map[string]any{
		"subchapter_id": i.resolveRelation(doc.Meta, "subchapter_source_id", "subchapter_id", "subchapter"),
		"title":         stringValue(doc.Meta["title"]),
		"question":      strings.TrimSpace(doc.Body),
		"solution":      stringValue(doc.Meta["solution"]),
		"order":         intValue(doc.Meta["order"], 1),
	}
}

func (i *Importer) articlePayload(doc Document) map[string]any {
	excerpt := stringValue(doc.Meta["excerpt"])
	if excerpt == "" {
		excerpt = autoExcerpt(doc.Body, 200)
	}
	payload := map[string]any{
		"title":        stringValue(doc.Meta["title"]),
		"slug":         stringValue(doc.Meta["slug"]),
		"excerpt":      excerpt,
		"content":      strings.TrimSpace(doc.Body),
		"category_id":  i.resolveRelation(doc.Meta, "category_source_id", "category_id", "category"),
		"published":    boolValue(doc.Meta["published"], false),
		"published_at": stringValue(doc.Meta["published_at"]),
	}
	if tags := i.resolveSourceIDList(doc.Meta, "tag_source_ids", "tag_ids", "tag"); len(tags) > 0 {
		payload["tags"] = tags
	}
	return payload
}

func (i *Importer) questionPayload(doc Document) map[string]any {
	payload := map[string]any{
		"class_id":      i.resolveRelation(doc.Meta, "class_source_id", "class_id", "class"),
		"subchapter_id": i.resolveRelation(doc.Meta, "subchapter_source_id", "subchapter_id", "subchapter"),
		"type":          stringValue(doc.Meta["type"]),
		"question":      strings.TrimSpace(doc.Body),
		"answer_json":   doc.Meta["answer_json"],
		"explanation":   stringValue(doc.Meta["explanation"]),
		"difficulty":    stringValue(doc.Meta["difficulty"]),
	}
	if opts, ok := doc.Meta["options_json"]; ok {
		payload["options_json"] = opts
	}
	return payload
}

func (i *Importer) examPayload(doc Document) examImport {
	questions := i.resolveSourceIDList(doc.Meta, "question_source_ids", "question_ids", "question")
	tokens := stringSlice(doc.Meta["tokens"])
	points := intValue(doc.Meta["points_per_question"], 10)
	if points <= 0 {
		points = 10
	}
	duration := intValue(doc.Meta["duration_min"], 30) * 60
	if duration == 0 {
		duration = 1800
	}
	return examImport{
		examFields: map[string]any{
			"title":                  stringValue(doc.Meta["title"]),
			"class_id":               i.resolveRelation(doc.Meta, "class_source_id", "class_id", "class"),
			"description":            stringValue(doc.Meta["description"]),
			"duration":               duration,
			"random_questions":       boolValue(doc.Meta["random_questions"], true),
			"random_options":         boolValue(doc.Meta["random_options"], true),
			"show_score_after":       boolValue(doc.Meta["show_score_after"], true),
			"show_explanation_after": boolValue(doc.Meta["show_explanation_after"], true),
			"is_active":              boolValue(doc.Meta["is_active"], false),
		},
		questions:    questions,
		tokens:       tokens,
		points:       points,
		sharedTokens: boolValue(doc.Meta["shared_tokens"], false),
	}
}

func (i *Importer) resolveRelation(meta map[string]any, sourceKey, directKey, kind string) string {
	if sourceID := stringValue(meta[sourceKey]); sourceID != "" {
		if recordID, err := i.resolveRecordID(kind, sourceID); err == nil {
			return recordID
		}
	}
	if direct := stringValue(meta[directKey]); direct != "" {
		if recordID, err := i.resolveRecordID(kind, direct); err == nil {
			return recordID
		}
		return direct
	}
	return ""
}

func (i *Importer) resolveSourceIDList(meta map[string]any, sourceKey, directKey, kind string) []string {
	values := stringSlice(meta[sourceKey])
	if len(values) == 0 {
		values = stringSlice(meta[directKey])
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if recordID, err := i.resolveRecordID(kind, value); err == nil {
			out = append(out, recordID)
			continue
		}
		out = append(out, value)
	}
	return out
}

func autoExcerpt(body string, maxLen int) string {
	text := strings.TrimSpace(body)
	if len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}

func BuildHugo(hugoDir, publicDir string) error {
	if hugoDir == "" {
		return errors.New("hugo dir is required")
	}
	cmd := exec.Command("hugo", "--minify")
	cmd.Dir = hugoDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("hugo build failed: %w", err)
	}
	src := filepath.Join(hugoDir, "public")
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("hugo output missing: %w", err)
	}
	if publicDir == "" {
		return errors.New("public dir is required")
	}
	if err := swapDirContents(src, publicDir); err != nil {
		return err
	}
	return nil
}

func swapDirContents(src, dst string) error {
	tmp := dst + ".next"
	old := dst + ".prev"
	if err := os.RemoveAll(tmp); err != nil {
		return err
	}
	if err := os.RemoveAll(old); err != nil {
		return err
	}
	if err := copyDir(src, tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, old); err != nil {
			_ = os.RemoveAll(tmp)
			return err
		}
	}
	if err := os.Rename(tmp, dst); err != nil {
		if _, statErr := os.Stat(old); statErr == nil {
			_ = os.Rename(old, dst)
		}
		_ = os.RemoveAll(tmp)
		_ = os.RemoveAll(old)
		return err
	}
	return os.RemoveAll(old)
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		return os.Chmod(target, info.Mode())
	})
}
