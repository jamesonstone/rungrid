package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/spf13/cobra"
)

func inputIsTTY(command *cobra.Command) bool {
	input, ok := command.InOrStdin().(*os.File)
	if !ok {
		return false
	}
	info, err := input.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func requireInteractive(command *cobra.Command, opt *options, message string) error {
	if opt.json || !inputIsTTY(command) {
		return errs.New(errs.ExitUsage, "RG1717", message)
	}
	return nil
}

func readPromptLine(command *cobra.Command, message string) (string, error) {
	input, ok := command.InOrStdin().(*os.File)
	if !ok {
		return "", errs.New(errs.ExitUsage, "RG1717", message)
	}
	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil {
		return "", errs.Wrap(errs.ExitInterrupted, "RG1718", message, err)
	}
	return strings.TrimSpace(answer), nil
}

func parseChoice(answer string, count int) (int, error) {
	choice, err := strconv.Atoi(strings.TrimSpace(answer))
	if err != nil || choice < 1 || choice > count {
		return 0, errs.New(errs.ExitUsage, "RG1719", "selection is out of range")
	}
	return choice - 1, nil
}

func parseChoices(answer string, count int) ([]int, error) {
	normalized := strings.TrimSpace(answer)
	if normalized == "" || strings.EqualFold(normalized, "all") {
		indexes := make([]int, count)
		for index := range indexes {
			indexes[index] = index
		}
		return indexes, nil
	}
	var indexes []int
	seen := map[int]bool{}
	for _, part := range strings.Split(normalized, ",") {
		index, err := parseChoice(part, count)
		if err != nil {
			return nil, err
		}
		if seen[index] {
			continue
		}
		seen[index] = true
		indexes = append(indexes, index)
	}
	return indexes, nil
}

func promptIndex(command *cobra.Command, opt *options, noun string, labels []string) (int, error) {
	if err := requireInteractive(command, opt, noun+" selection requires an interactive terminal or an explicit selector"); err != nil {
		return 0, err
	}
	if len(labels) == 0 {
		return 0, errs.New(errs.ExitConflict, "RG1713", "no "+noun+" choices were available")
	}
	writeChoices(command, labels)
	_, _ = fmt.Fprintf(command.OutOrStdout(), "Select %s [1-%d]: ", noun, len(labels))
	answer, err := readPromptLine(command, "read "+noun+" selection")
	if err != nil {
		return 0, err
	}
	return parseChoice(answer, len(labels))
}

func promptIndexes(command *cobra.Command, opt *options, noun string, labels []string) ([]int, error) {
	if err := requireInteractive(command, opt, noun+" selection requires an interactive terminal or an explicit selector"); err != nil {
		return nil, err
	}
	if len(labels) == 0 {
		return nil, errs.New(errs.ExitConflict, "RG1713", "no "+noun+" choices were available")
	}
	writeChoices(command, labels)
	_, _ = fmt.Fprintf(command.OutOrStdout(), "Select %s [1-%d, comma, or all]: ", noun, len(labels))
	answer, err := readPromptLine(command, "read "+noun+" selection")
	if err != nil {
		return nil, err
	}
	return parseChoices(answer, len(labels))
}

func promptConfirm(command *cobra.Command, opt *options, question string, defaultYes bool) (bool, error) {
	if err := requireInteractive(command, opt, "confirmation requires an interactive terminal or --yes"); err != nil {
		return false, err
	}
	hint := "[y/N]"
	if defaultYes {
		hint = "[Y/n]"
	}
	_, _ = fmt.Fprintf(command.OutOrStdout(), "%s %s ", question, hint)
	answer, err := readPromptLine(command, "read confirmation")
	if err != nil {
		return false, err
	}
	if answer == "" {
		return defaultYes, nil
	}
	switch strings.ToLower(answer) {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return false, errs.New(errs.ExitUsage, "RG1721", "confirmation is invalid")
	}
}

func writeChoices(command *cobra.Command, labels []string) {
	for index, label := range labels {
		_, _ = fmt.Fprintf(command.OutOrStdout(), "%d) %s\n", index+1, label)
	}
}
