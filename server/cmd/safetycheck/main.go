// safetycheck is a command-line tool to exercise the safety pipeline and
// prompt builder without involving an LLM. Use it to sanity-check rule
// changes, debug prompt assembly, or demonstrate the system to stakeholders.
//
// Example:
//
//	safetycheck precheck --child-fears "蜘蛛,蛇" "讲个奥特曼睡前故事"
//	safetycheck postcheck --child=小宇 "故事内容..."
//	safetycheck build-prompt --child=小宇 --age=5 --duration=10 \
//	    --style=温馨治愈 --topic=勇敢 "讲个奥特曼睡前故事"
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aibao/server/internal/service/safety"
	"github.com/aibao/server/internal/service/story/prompt"
)

const (
	defaultRulesPath     = "safety/rules.yaml"
	defaultWhitelistPath = "safety/ip_whitelist.yaml"
	defaultBlacklistPath = "safety/ip_blacklist.yaml"
	defaultTemplatePath  = "safety/system_prompt.tmpl"
)

func main() {
	root := &cobra.Command{
		Use:   "safetycheck",
		Short: "Aibao safety pipeline + prompt builder cli demo",
	}
	root.AddCommand(newPrecheckCmd())
	root.AddCommand(newPostcheckCmd())
	root.AddCommand(newBuildPromptCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newPrecheckCmd() *cobra.Command {
	var (
		fearsCSV string
		maxRunes int
	)
	cmd := &cobra.Command{
		Use:   "precheck [user-prompt]",
		Short: "Run PreCheck on a user-supplied story prompt",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			rs, err := safety.LoadRules(defaultRulesPath, defaultWhitelistPath, defaultBlacklistPath)
			if err != nil {
				return err
			}
			pc := safety.NewPreChecker(rs, safety.NewNoopIntentProvider())
			out := pc.Check(context.Background(), safety.PreCheckInput{
				UserPrompt:     args[0],
				ChildFearList:  splitCSV(fearsCSV),
				MaxPromptRunes: maxRunes,
			})
			return printJSON(out)
		},
	}
	cmd.Flags().StringVar(&fearsCSV, "child-fears", "", "comma-separated fear list (e.g. \"蜘蛛,蛇\")")
	cmd.Flags().IntVar(&maxRunes, "max-runes", 0, "max prompt rune count (default 200)")
	return cmd
}

func newPostcheckCmd() *cobra.Command {
	var (
		child    string
		fearsCSV string
	)
	cmd := &cobra.Command{
		Use:   "postcheck [story-text]",
		Short: "Run PostCheck on an LLM-generated story",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			rs, err := safety.LoadRules(defaultRulesPath, defaultWhitelistPath, defaultBlacklistPath)
			if err != nil {
				return err
			}
			pc := safety.NewPostChecker(rs)
			out := pc.Check(safety.PostCheckInput{
				StoryText:     args[0],
				ChildNickname: child,
				ChildFearList: splitCSV(fearsCSV),
			})
			return printJSON(out)
		},
	}
	cmd.Flags().StringVar(&child, "child", "小宇", "child nickname")
	cmd.Flags().StringVar(&fearsCSV, "child-fears", "", "comma-separated fear list")
	return cmd
}

func newBuildPromptCmd() *cobra.Command {
	var (
		child          string
		age            int
		gender         string
		fearsCSV       string
		duration       int
		style          string
		topic          string
		memorySummary  string
		ipInstructions string
	)
	cmd := &cobra.Command{
		Use:   "build-prompt [user-prompt]",
		Short: "Assemble the system prompt with given inputs and print it",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			b, err := prompt.NewBuilder(defaultTemplatePath)
			if err != nil {
				return err
			}
			out := b.Build(prompt.BuildInput{
				ChildNickname:            child,
				ChildAgeYears:            age,
				ChildGender:              gender,
				ChildFearList:            splitCSV(fearsCSV),
				Duration:                 duration,
				Style:                    style,
				Topic:                    topic,
				UserPromptCleaned:        args[0],
				NormalizedIPInstructions: ipInstructions,
				MemorySummary:            memorySummary,
				PromptVersion:            "v1",
			})
			fmt.Println("=== SYSTEM PROMPT ===")
			fmt.Println(out.SystemPrompt)
			fmt.Println("=== USER PROMPT ===")
			fmt.Println(out.UserPrompt)
			return nil
		},
	}
	cmd.Flags().StringVar(&child, "child", "小宇", "child nickname")
	cmd.Flags().IntVar(&age, "age", 5, "child age in years")
	cmd.Flags().StringVar(&gender, "gender", "boy", "boy / girl / unspecified")
	cmd.Flags().StringVar(&fearsCSV, "child-fears", "", "comma-separated fear list")
	cmd.Flags().IntVar(&duration, "duration", 10, "story duration in minutes (5/10/15)")
	cmd.Flags().StringVar(&style, "style", "温馨治愈", "story style")
	cmd.Flags().StringVar(&topic, "topic", "", "educational topic (may be empty)")
	cmd.Flags().StringVar(&memorySummary, "memory", "", "recent story memory summary")
	cmd.Flags().StringVar(&ipInstructions, "ip-instructions", "", "same-character instructions to inject")
	return cmd
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func printJSON(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}
