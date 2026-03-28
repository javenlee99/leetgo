package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
	"time"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	"github.com/j178/leetgo/config"
	"github.com/j178/leetgo/leetcode"
)

var solutionCmd = &cobra.Command{
	Use:   "solution [qid]",
	Short: "Fetch solutions from followed users",
	Long: `Fetch and save high-quality solutions from configured followed users.

Supports question ID, slug, "today", or "last".`,
	Example: `leetgo solution 53
leetgo solution maximum-subarray
leetgo solution today
leetgo solution last`,
	Args:      cobra.MaximumNArgs(1),
	Aliases:   []string{"sol"},
	ValidArgs: []string{"today", "last"},
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Get()
		c := leetcode.NewClient(leetcode.ReadCredentials())

		// Parse question ID
		var qid string
		if len(args) > 0 {
			qid = args[0]
		} else {
			qid = "last"
		}

		qs, err := leetcode.ParseQID(qid, c)
		if err != nil {
			return err
		}
		if len(qs) > 1 {
			return fmt.Errorf("`leetgo solution` cannot handle multiple contest questions")
		}
		q := qs[0]

		log.Info("Fetching solutions", "question", q.TitleSlug)

		// Fetch solution list
		solutionList, err := c.GetSolutionList(q.TitleSlug, 0, 15)
		if err != nil {
			return fmt.Errorf("fetch solution list: %w", err)
		}

		if solutionList.TotalNum == 0 {
			log.Warn("No solutions found for this question")
			return nil
		}

		log.Info("Found solutions", "total", solutionList.TotalNum)

		// Filter by followed users
		filtered := leetcode.FilterSolutionsByUsers(
			solutionList.Edges,
			cfg.Solution.FollowedUsers,
		)

		if len(filtered) == 0 {
			log.Warn("No solutions found from followed users", "users", cfg.Solution.FollowedUsers)
			log.Info("Hint: Add usernames to solution.followed_users in your config")
			return nil
		}

		log.Info("Fetching solution details", "count", len(filtered))

		// Create output directory
		outDir := getQuestionOutDir(q, cfg)
		solutionDir := filepath.Join(outDir, cfg.Solution.OutputDir)
		if err := os.MkdirAll(solutionDir, 0o755); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}

		// Fetch and save each solution
		saved := 0
		for i, edge := range filtered {
			log.Info("Processing solution",
				"progress", fmt.Sprintf("%d/%d", i+1, len(filtered)),
				"author", edge.Node.Author.Username,
			)

			solution, err := c.GetSolutionDetail(edge.Node.Slug)
			if err != nil {
				log.Error("Failed to fetch solution detail",
					"author", edge.Node.Author.Username,
					"error", err,
				)
				continue
			}

			// Generate filename
			filename, err := generateFilename(cfg.Solution.FilenameTemplate, q, &edge.Node)
			if err != nil {
				log.Error("Failed to generate filename", "error", err)
				continue
			}
			filepath := filepath.Join(solutionDir, filename)

			// Format content
			content := formatSolutionMarkdown(solution, q.TitleSlug)
			if err := os.WriteFile(filepath, []byte(content), 0o644); err != nil {
				log.Error("Failed to save solution", "file", filepath, "error", err)
				continue
			}

			log.Info("Saved solution", "file", filepath)
			saved++
		}

		if saved > 0 {
			log.Info("Done", "saved", saved, "solutions")
		} else {
			log.Warn("No solutions were saved")
		}
		return nil
	},
}

// getQuestionOutDir returns the output directory for a question
func getQuestionOutDir(q *leetcode.QuestionData, cfg *config.Config) string {
	// Determine base directory based on language
	var baseDir string
	switch cfg.Code.Lang {
	case "go":
		baseDir = cfg.Code.Go.OutDir
	case "python", "python3":
		baseDir = cfg.Code.Python.OutDir
	case "cpp":
		baseDir = cfg.Code.Cpp.OutDir
	case "rust":
		baseDir = cfg.Code.Rust.OutDir
	case "java":
		baseDir = cfg.Code.Java.OutDir
	default:
		baseDir = cfg.Code.Lang
	}

	if baseDir == "" {
		baseDir = cfg.Code.Lang
	}

	// Construct full path: projectRoot/baseDir/questionSlug
	return filepath.Join(cfg.ProjectRoot(), baseDir, q.TitleSlug)
}

// generateFilename generates filename from template
func generateFilename(tmpl string, q *leetcode.QuestionData, solution *leetcode.SolutionMetadata) (string, error) {
	t, err := template.New("filename").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	data := map[string]string{
		"QuestionId":     q.QuestionFrontendId,
		"AuthorSlug":     solution.Author.Profile.UserSlug,
		"AuthorUsername": solution.Author.Username,
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}

// formatSolutionMarkdown formats solution as markdown
func formatSolutionMarkdown(solution *leetcode.Solution, questionSlug string) string {
	var buf bytes.Buffer

	// Header
	fmt.Fprintf(&buf, "# %s\n\n", solution.Title)

	// Metadata
	realName := solution.Author.Profile.RealName
	if realName == "" {
		realName = solution.Author.Username
	}
	fmt.Fprintf(&buf, "**Author:** %s (@%s)\n",
		realName,
		solution.Author.Username,
	)
	fmt.Fprintf(&buf, "**Created:** %s\n", solution.CreatedAt)
	fmt.Fprintf(&buf, "**Upvotes:** %d | **Favorites:** %d\n",
		solution.UpvoteCount,
		solution.FavoriteCount,
	)

	// Tags
	if len(solution.Tags) > 0 {
		fmt.Fprintf(&buf, "**Tags:** ")
		for i, tag := range solution.Tags {
			if i > 0 {
				buf.WriteString(", ")
			}
			tagName := tag.NameTranslated
			if tagName == "" {
				tagName = tag.Name
			}
			buf.WriteString(tagName)
		}
		buf.WriteString("\n")
	}

	// Link
	siteURL := config.Get().LeetCode.Site
	fmt.Fprintf(&buf, "**Link:** %s/problems/%s/solutions/%s/%s\n\n",
		siteURL,
		questionSlug,
		solution.Author.Profile.UserSlug,
		solution.Slug,
	)

	buf.WriteString("---\n\n")

	// Content
	buf.WriteString(solution.Content)

	buf.WriteString("\n\n---\n\n")

	// Footer
	fmt.Fprintf(&buf, "*Fetched by leetgo on %s*\n", time.Now().Format("2006-01-02"))

	return buf.String()
}
