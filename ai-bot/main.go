package main

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"gezyclass/ai-bot/internal/pocketbase"
	reportcmd "gezyclass/ai-bot/internal/report"
	"gezyclass/ai-bot/internal/staging"
)

func main() {
	client := pocketbase.NewClient()

	rootCmd := &cobra.Command{
		Use:   "ai-bot",
		Short: "AI Bot tooling for staging and publishing GezyClass content",
	}
	var stagingDir, statePath string
	rootCmd.PersistentFlags().StringVar(&stagingDir, "staging-dir", getEnv("STAGING_DIR", "../content-staging"), "Markdown staging directory")
	rootCmd.PersistentFlags().StringVar(&statePath, "state-file", "", "Seeder state file path")

	// material command
	materialCmd := &cobra.Command{
		Use:   "material",
		Short: "Manage learning materials",
	}
	var classID int
	var chapterName, subchapterName, title string
	materialCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "Stage a new material package",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := staging.WriteMaterialPackage(staging.MaterialPackageInput{
				StagingDir:     stagingDir,
				ClassID:        classID,
				ChapterName:    chapterName,
				SubchapterName: subchapterName,
				Title:          title,
			})
			if err != nil {
				return err
			}
			printStaged(paths)
			return nil
		},
	}
	materialCreateCmd.Flags().IntVar(&classID, "class", 0, "Class ID (7/8/9)")
	materialCreateCmd.Flags().StringVar(&chapterName, "chapter", "", "Chapter name")
	materialCreateCmd.Flags().StringVar(&subchapterName, "subchapter", "", "Subchapter name")
	materialCreateCmd.Flags().StringVar(&title, "title", "", "Material title")
	materialCreateCmd.MarkFlagRequired("class")
	materialCreateCmd.MarkFlagRequired("chapter")
	materialCreateCmd.MarkFlagRequired("subchapter")
	materialCmd.AddCommand(materialCreateCmd)
	rootCmd.AddCommand(materialCmd)

	// article command
	articleCmd := &cobra.Command{
		Use:   "article",
		Short: "Manage blog articles",
	}
	var categoryName string
	articleCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "Stage a new article",
		RunE: func(cmd *cobra.Command, args []string) error {
			paths, err := staging.WriteArticlePackage(staging.ArticlePackageInput{
				StagingDir:   stagingDir,
				Title:        title,
				CategoryName: categoryName,
			})
			if err != nil {
				return err
			}
			printStaged(paths)
			return nil
		},
	}
	articleCreateCmd.Flags().StringVar(&title, "title", "", "Article title")
	articleCreateCmd.Flags().StringVar(&categoryName, "category", "", "Category name")
	articleCreateCmd.MarkFlagRequired("title")
	articleCreateCmd.MarkFlagRequired("category")
	articleCmd.AddCommand(articleCreateCmd)
	rootCmd.AddCommand(articleCmd)

	// exam command
	examCmd := &cobra.Command{
		Use:   "exam",
		Short: "Manage CBT exams and tokens",
	}

	var examClassID, examQuestionCount, examTokenCount, examDuration int
	examCreateCmd := &cobra.Command{
		Use:   "create",
		Short: "Stage a new exam with questions and tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			return stageExam(client, stagingDir, title, examClassID, examQuestionCount, examTokenCount, examDuration)
		},
	}
	examCreateCmd.Flags().StringVar(&title, "title", "", "Exam title")
	examCreateCmd.Flags().IntVar(&examClassID, "class", 0, "Class (7/8/9)")
	examCreateCmd.Flags().IntVar(&examQuestionCount, "questions", 10, "Number of questions")
	examCreateCmd.Flags().IntVar(&examTokenCount, "tokens", 5, "Number of tokens")
	examCreateCmd.Flags().IntVar(&examDuration, "duration", 30, "Duration in minutes")
	examCreateCmd.MarkFlagRequired("title")
	examCreateCmd.MarkFlagRequired("class")
	examCmd.AddCommand(examCreateCmd)

	var examID, tokenValue string
	examTokenCmd := &cobra.Command{
		Use:   "token",
		Short: "Stage tokens for an existing synced exam",
		RunE: func(cmd *cobra.Command, args []string) error {
			tokens, err := staging.GenerateTokens(examTokenCount)
			if err != nil {
				return err
			}
			if err := staging.AddTokensToExamByRecordID(stagingDir, defaultStatePath(stagingDir, statePath), examID, tokens); err != nil {
				return err
			}
			fmt.Println("Tokens staged:")
			for _, token := range tokens {
				fmt.Printf("  - %s\n", token)
			}
			return nil
		},
	}
	examTokenCmd.Flags().StringVar(&examID, "exam-id", "", "Exam ID")
	examTokenCmd.Flags().IntVar(&examTokenCount, "count", 5, "Number of tokens")
	examTokenCmd.MarkFlagRequired("exam-id")
	examCmd.AddCommand(examTokenCmd)

	examDeleteTokenCmd := &cobra.Command{
		Use:   "delete-token",
		Short: "Remove a token from staging",
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceID, err := staging.RemoveTokenFromAnyExam(stagingDir, tokenValue)
			if err != nil {
				return err
			}
			fmt.Printf("Token removed from staging exam %s: %s\n", sourceID, tokenValue)
			return nil
		},
	}
	examDeleteTokenCmd.Flags().StringVar(&tokenValue, "token", "", "Token value to delete")
	examDeleteTokenCmd.MarkFlagRequired("token")
	examCmd.AddCommand(examDeleteTokenCmd)

	rootCmd.AddCommand(examCmd)

	// sync command
	var syncHugoDir, syncPublicDir string
	var syncDryRun, syncBuildHugo bool
	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Import markdown staging into PocketBase and rebuild Hugo",
		RunE: func(cmd *cobra.Command, args []string) error {
			importer, err := staging.NewImporter(client, staging.SeedOptions{
				StagingDir: stagingDir,
				StatePath:  defaultStatePath(stagingDir, statePath),
				DryRun:     syncDryRun,
				BuildHugo:  syncBuildHugo,
				HugoDir:    syncHugoDir,
				PublicDir:  syncPublicDir,
			})
			if err != nil {
				return err
			}
			result, err := importer.Run()
			if err != nil {
				return err
			}
			fmt.Printf("sync complete: %d processed, %d created, %d updated, %d skipped\n", result.Processed, result.Created, result.Updated, result.Skipped)
			return nil
		},
	}
	syncCmd.Flags().StringVar(&syncHugoDir, "hugo-dir", getEnv("HUGO_DIR", "/var/www/class.gezytech.web.id/hugo"), "Hugo project directory")
	syncCmd.Flags().StringVar(&syncPublicDir, "public-dir", getEnv("HUGO_PUBLIC_PATH", "/var/www/class.gezytech.web.id/public"), "Live public directory")
	syncCmd.Flags().BoolVar(&syncDryRun, "dry-run", false, "Print actions without writing")
	syncCmd.Flags().BoolVar(&syncBuildHugo, "build-hugo", true, "Rebuild Hugo after import")
	rootCmd.AddCommand(syncCmd)

	// backup command
	var backupDir string
	backupCmd := &cobra.Command{
		Use:   "backup",
		Short: "Backup PocketBase database and auxiliary files",
		RunE: func(cmd *cobra.Command, args []string) error {
			if backupDir == "" {
				backupDir = getEnv("BACKUP_DIR", "/var/www/class.gezytech.web.id/backups")
			}
			return backupPocketBase(backupDir)
		},
	}
	backupCmd.Flags().StringVar(&backupDir, "backup-dir", "", "Backup destination directory")
	rootCmd.AddCommand(backupCmd)

	// report command
	rootCmd.AddCommand(reportcmd.NewCommand(client))

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func defaultStatePath(stagingDir, statePath string) string {
	if statePath != "" {
		return statePath
	}
	return filepath.Join(stagingDir, ".seeder-state.json")
}

func printStaged(paths []string) {
	fmt.Println("Staged files:")
	for _, path := range paths {
		fmt.Printf("  - %s\n", path)
	}
	fmt.Println("Run sync to import them into PocketBase.")
}

func stageExam(client *pocketbase.Client, stagingDir, title string, classNum, questionCount, tokenCount, durationMin int) error {
	classSlug := fmt.Sprintf("kelas-%d", classNum)
	classes, err := client.List("classes", fmt.Sprintf("slug='%s'", classSlug))
	if err != nil {
		return fmt.Errorf("failed to list classes: %w", err)
	}
	if len(classes) == 0 {
		return fmt.Errorf("class not found in PocketBase: %s", classSlug)
	}
	classID, ok := classes[0]["id"].(string)
	if !ok || classID == "" {
		return fmt.Errorf("class %s has empty PocketBase id", classSlug)
	}
	questions, err := client.List("questions", fmt.Sprintf("class_id='%s' && type='pg'", classID))
	if err != nil {
		return fmt.Errorf("failed to list questions: %w", err)
	}
	if len(questions) < questionCount {
		return fmt.Errorf("not enough questions: need %d, have %d", questionCount, len(questions))
	}
	rand.Shuffle(len(questions), func(i, j int) {
		questions[i], questions[j] = questions[j], questions[i]
	})
	questionIDs := make([]string, 0, questionCount)
	for _, question := range questions[:questionCount] {
		id, ok := question["id"].(string)
		if !ok || id == "" {
			continue
		}
		questionIDs = append(questionIDs, id)
	}
	if len(questionIDs) != questionCount {
		return fmt.Errorf("selected only %d valid question ids out of %d", len(questionIDs), questionCount)
	}
	tokens, err := staging.GenerateTokens(tokenCount)
	if err != nil {
		return err
	}
	paths, err := staging.WriteExamPackage(staging.ExamPackageInput{
		StagingDir:      stagingDir,
		Title:           title,
		ClassID:         classNum,
		QuestionIDs:     questionIDs,
		TokenValues:     tokens,
		DurationMin:     durationMin,
		QuestionCount:   questionCount,
		SelectedClassID: classID,
	})
	if err != nil {
		return err
	}
	printStaged(paths)
	fmt.Println("Tokens staged:")
	for _, token := range tokens {
		fmt.Printf("  - %s\n", token)
	}
	return nil
}

func backupPocketBase(backupDir string) error {
	pbDir := getEnv("PB_DIR", "/var/www/class.gezytech.web.id/pocketbase/pb_data")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return err
	}
	timestamp := time.Now().Format("20060102-150405")
	files := []string{"data.db", "auxiliary.db"}
	var backedUp []string
	for _, name := range files {
		src := filepath.Join(pbDir, name)
		if _, err := os.Stat(src); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		dst := filepath.Join(backupDir, fmt.Sprintf("%s-%s", timestamp, name))
		if err := copyFile(src, dst); err != nil {
			return err
		}
		backedUp = append(backedUp, dst)
	}
	if len(backedUp) == 0 {
		return errors.New("no PocketBase files found to back up")
	}
	fmt.Printf("backup complete: %s\n", backupDir)
	for _, path := range backedUp {
		fmt.Printf("  - %s\n", path)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
