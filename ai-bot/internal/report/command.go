package report

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"gezyclass/ai-bot/internal/pocketbase"
)

// NewCommand creates the private, server-side report command. It is exposed
// only through the ai-bot binary; no public Hugo or PocketBase endpoint is
// added for reports.
func NewCommand(client *pocketbase.Client) *cobra.Command {
	var options Options
	var activityID string

	cmd := &cobra.Command{
		Use:   "report [activity]",
		Short: "Read-only student reports matched to the /murid master list",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if options.Activity == "" {
				options.Activity = strings.TrimSpace(activityID)
			}
			if options.Activity == "" && len(args) == 1 {
				options.Activity = strings.TrimSpace(args[0])
			}
			if options.List {
				return PrintActivities(client, options.Type)
			}

			result, err := Build(client, options)
			if err != nil {
				return err
			}
			return Write(result, options.Format, options.Output)
		},
	}

	cmd.Flags().StringVar(&options.Type, "type", "cbt", "Source type: cbt or latihan")
	cmd.Flags().StringVar(&options.Activity, "activity", "", "Exam ID/title or latihan slug/title; omit for all activities")
	cmd.Flags().StringVar(&activityID, "id", "", "Alias for --activity")
	cmd.Flags().StringVar(&options.Class, "class", "", "Limit to a class such as 8.1 or all of class 8")
	cmd.Flags().StringVar(&options.Metric, "metric", "all", "Aggregation: all, highest, average, or latest")
	cmd.Flags().StringVar(&options.Format, "format", "md", "Output: md, json, csv, xlsx, docx, or pdf")
	cmd.Flags().StringVar(&options.Date, "date", "", "Optional date filter in YYYY-MM-DD")
	cmd.Flags().StringVar(&options.MasterPath, "master", "", "Path to murid-data.html (defaults to the live Hugo partial)")
	cmd.Flags().StringVarP(&options.Output, "out", "o", "", "Output file; markdown/json/csv use stdout when omitted")
	cmd.Flags().BoolVar(&options.List, "list", false, "List available CBT exams or latihan activities")

	return cmd
}

// PrintActivities gives the bot a compact way to discover the exact activity
// identifier before requesting a report.
func PrintActivities(client *pocketbase.Client, kind string) error {
	activities, err := loadActivities(client, normalizeKind(kind))
	if err != nil {
		return err
	}
	for _, activity := range activities {
		if activity.Kind == "cbt" {
			fmt.Printf("%s\t%s\n", activity.ID, activity.Title)
			continue
		}
		fmt.Printf("%s\t%s\t%s\n", activity.Slug, activity.Title, activity.ID)
	}
	return nil
}
