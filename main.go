package main

import (
	"context"

	"github.com/google/go-github/v31/github"
	"github.com/kelseyhightower/envconfig"
	"github.com/kr/pretty"
	"github.com/posener/goaction"
	"github.com/posener/goaction/actionutil"
	"github.com/posener/goaction/log"
	"gopkg.in/yaml.v3"
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

	label, isXL := GetPrLabel(config, event.PullRequest)

	ctx := context.Background()
	gh := actionutil.NewClientWithToken(ctx, config.GitHubToken)

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

	pretty.Print(config)

	return config, err
}

func GetPrLabel(config Config, pr *github.PullRequest) (string, bool) {
	totalModifications := max(pr.GetAdditions(), pr.GetDeletions())

	switch {
	case totalModifications < config.Thresholds.XS.LessThan:
		return config.Thresholds.XS.Label, false
	case totalModifications < config.Thresholds.S.LessThan:
		return config.Thresholds.S.Label, false
	case totalModifications < config.Thresholds.M.LessThan:
		return config.Thresholds.M.Label, false
	case totalModifications < config.Thresholds.L.LessThan:
		return config.Thresholds.L.Label, false
	default:
		return "", true
	}
}

type PrSize string

// Config is the data structure used to define the action settings.
type Config struct {
	GitHubToken      string `envconfig:"GITHUB_TOKEN" required:"true"`
	Thresholds       Thresholds
	ThresholdsYAML   string `envconfig:"THRESHOLDS"`
	ExcludePaths     []string
	ExcludePathsYAML string `envconfig:"EXCLUDE_PATHS"`
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

type Thresholds struct {
	XS          Size   `yaml:"xs"`
	S           Size   `yaml:"s"`
	M           Size   `yaml:"m"`
	L           Size   `yaml:"l"`
	FailIfXL    bool   `yaml:"fail_if_xl"`
	MessageIfXL string `yaml:"message_if_xl"`
}

func (t Thresholds) GetLabels() []string {
	return []string{
		t.XS.Label,
		t.S.Label,
		t.M.Label,
		t.L.Label,
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

type Size struct {
	LessThan int    `yaml:"less_than"`
	Label    string `yaml:"label"`
}
