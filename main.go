package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/google/go-github/v31/github"
	"github.com/kelseyhightower/envconfig"
	"github.com/posener/goaction"
	"github.com/posener/goaction/actionutil"
	"github.com/posener/goaction/log"
	"gopkg.in/yaml.v3"
)

type (
	// Config is the data structure used to define the action settings.
	Config struct {
		GitHubToken      string `envconfig:"GITHUB_TOKEN" required:"true"`
		Thresholds       Thresholds
		ThresholdsYAML   string `envconfig:"THRESHOLDS"`
		ExcludePaths     []string
		ExcludePathsYAML string `envconfig:"EXCLUDE_PATHS"`
	}

	// Size defines a specific thresholds and label to be used.
	Size struct {
		LessThan int    `yaml:"less_than"`
		Label    string `yaml:"label"`
	}

	// Thresholds defines a set of thresholds.
	Thresholds struct {
		XS          Size   `yaml:"xs"`
		S           Size   `yaml:"s"`
		M           Size   `yaml:"m"`
		L           Size   `yaml:"l"`
		FailIfXL    bool   `yaml:"fail_if_xl"`
		MessageIfXL string `yaml:"message_if_xl"`
	}
)

func main() {
	if !goaction.CI {
		log.Warnf("Not in GitHub Action mode, quitting...")
		return
	}

	if goaction.Event != goaction.EventPullRequest {
		log.Debugf("Not a pull request action, nothing to do here...")
		return
	}

	config, err := getConfig()
	if err != nil {
		log.Fatalf("Required configuration is missing: %s", err)
	}

	event, err := goaction.GetPullRequest()
	if err != nil {
		log.Fatalf("Error happened while getting event info: %s", err)
	}

	ctx := context.Background()
	gh := actionutil.NewClientWithToken(ctx, config.GitHubToken)

	label, isXL, err := GetPrLabel(config, event)
	if err != nil {
		log.Fatalf("Error happened while getting label: %s", err)
	}
	log.Printf("Label: %s, isXL: %t", label, isXL)

	err = replaceLabels(ctx, gh, config, label)
	if err != nil {
		log.Fatalf("Error happened while adding label: %s", err)
	}

	if !isXL {
		log.Debugf("Pull request successfully labeled")
		return
	}

	_, _, err = gh.PullRequestsCreateComment(ctx, goaction.PrNum(), &github.PullRequestComment{
		Body: github.String(config.Thresholds.MessageIfXL),
	})

	if err != nil {
		log.Fatalf("Error happened while adding comment: %s", err)
	}

	if config.Thresholds.FailIfXL {
		log.Fatalf("PR size is XL, make it shorter, please!")
	}

	log.Debugf("Pull request successfully labeled")
}

// CalculateModifications calls out to `git diff | diffstat` and parses the output to determine the number of changed lines.
func CalculateModifications(config Config, event *github.PullRequestEvent) int {
	output := getDiffstatCSV(event.GetPullRequest().GetBase().GetSHA())

	stats := parseDiffstatOutput(output)

	applyExclusions := func(stat diffstat) bool {
		for _, excludeGlob := range config.ExcludePaths {
			if matched, _ := doublestar.Match(excludeGlob, stat.FileName); matched {
				log.Printf("Excluded file: %s", stat.FileName)
				return false
			}
		}
		return true
	}
	filtered := filter(applyExclusions, stats)

	accumChangedLines := func(size int, file diffstat) int {
		return size + file.Modified + file.Deleted + file.Inserted
	}
	return reduce(accumChangedLines, 0, filtered)
}

func getDiffstatCSV(target string) []byte {
	if err := exec.Command("bash", "-c", `git config --global --add safe.directory "$GITHUB_WORKSPACE"`).Run(); err != nil {
		log.Fatalf("Error happened while running git config: %s", err)
	}

	command := exec.Command("bash", "-c", fmt.Sprintf("git diff %s | diffstat -mbqt", target))
	command.Stderr = os.Stderr
	output, err := command.Output()
	if err != nil {
		log.Fatalf("Error happened while running diffstat: %s", err)
	}
	log.Printf("Diffstat output: \n%s\n", string(output))
	return output
}

func parseDiffstatOutput(output []byte) []diffstat {
	data, err := csv.NewReader(bytes.NewReader(output)).ReadAll()
	if err != nil {
		log.Fatalf("Error happened while reading diffstat output: %s", err)
	}

	stats := []diffstat{}
	for i, datum := range data {
		if i == 0 {
			continue
		}
		stats = append(stats, diffstat{
			Inserted: must(strconv.Atoi(datum[0])),
			Deleted:  must(strconv.Atoi(datum[1])),
			Modified: must(strconv.Atoi(datum[2])),
			FileName: datum[3],
		})
	}
	return stats
}

func replaceLabels(ctx context.Context, gh *actionutil.Client, config Config, label string) error {
	ghLabels, _, err := gh.IssuesListLabelsByIssue(ctx, goaction.PrNum(), nil)
	if err != nil {
		return err
	}

	confLabels := config.Thresholds.GetLabels()

	for _, ghLabel := range ghLabels {
		if contains(confLabels, ghLabel.GetName()) {
			_, err = gh.IssuesRemoveLabelForIssue(ctx, goaction.PrNum(), ghLabel.GetName())
			if err != nil {
				return err
			}
		}
	}

	_, _, err = gh.IssuesAddLabelsToIssue(ctx, goaction.PrNum(), []string{label})
	return err
}

func getConfig() (Config, error) {
	config := Config{
		Thresholds: defaultThresholds(),
	}
	err := envconfig.Process("input", &config)
	if err != nil {
		return Config{}, err
	}

	if err := yaml.Unmarshal([]byte(config.ThresholdsYAML), &config.Thresholds); err != nil {
		return Config{}, err
	}

	if err := yaml.Unmarshal([]byte(config.ExcludePathsYAML), &config.ExcludePaths); err != nil {
		return Config{}, err
	}

	return config, err
}

func GetPrLabel(config Config, event *github.PullRequestEvent) (string, bool, error) {
	totalChanges := CalculateModifications(config, event)

	log.Printf("Total changes: %d lines", totalChanges)

	return config.Thresholds.DetermineLabel(totalChanges), totalChanges > config.Thresholds.L.LessThan, nil
}

func (t Thresholds) GetLabels() []string {
	return []string{
		t.XS.Label,
		t.S.Label,
		t.M.Label,
		t.L.Label,
	}
}
func defaultThresholds() Thresholds {
	return Thresholds{
		XS: Size{
			LessThan: 10,
			Label:    "size/xs",
		},
		S: Size{
			LessThan: 100,
			Label:    "size/s",
		},
		M: Size{
			LessThan: 500,
			Label:    "size/m",
		},
		L: Size{
			LessThan: 1000,
			Label:    "size/l",
		},
		FailIfXL:    false,
		MessageIfXL: "This PR is too big. Please, split it.",
	}
}

func (t Thresholds) DetermineLabel(size int) string {
	switch {
	case size < t.XS.LessThan:
		return t.XS.Label
	case size < t.S.LessThan:
		return t.S.Label
	case size < t.M.LessThan:
		return t.M.Label
	case size < t.L.LessThan:
		return t.L.Label
	default:
		return "XL"
	}
}

func contains(a []string, x string) bool {
	for _, n := range a {
		if x == n {
			return true
		}
	}
	return false
}

func filter[A any](f func(A) bool, list []A) []A {
	res := make([]A, 0, len(list))
	for _, v := range list {
		if f(v) {
			res = append(res, v)
		}
	}
	return res
}

func reduce[T any, R any](accumulator func(agg R, item T) R, initial R, collection []T) R {
	for _, item := range collection {
		initial = accumulator(initial, item)
	}

	return initial
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

type diffstat struct {
	Inserted int
	Deleted  int
	Modified int
	FileName string
}
