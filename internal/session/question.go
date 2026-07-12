package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type QuestionMode string

const (
	QuestionSingle QuestionMode = "single"
	QuestionMulti  QuestionMode = "multi"
	QuestionText   QuestionMode = "text"
)

type Question struct {
	Kind        string       `json:"kind"`
	Mode        QuestionMode `json:"mode"`
	Prompt      string       `json:"prompt"`
	Options     []string     `json:"options,omitempty"`
	AllowCustom bool         `json:"allow_custom,omitempty"`
	Placeholder string       `json:"placeholder,omitempty"`
	MinSelect   int          `json:"min_select,omitempty"`
	MaxSelect   int          `json:"max_select,omitempty"`
}

func ParseQuestion(text string) (*Question, bool) {
	start := strings.Index(text, "{")
	if start == -1 {
		return nil, false
	}
	for i := start; i < len(text); i++ {
		if text[i] != '{' {
			continue
		}
		depth := 0
		for j := i; j < len(text); j++ {
			switch text[j] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					var q Question
					if err := json.Unmarshal([]byte(text[i:j+1]), &q); err == nil && q.Kind == "question" && q.Prompt != "" {
						if q.Mode == "" {
							q.Mode = QuestionText
						}
						return &q, true
					}
					break
				}
			}
		}
	}
	return nil, false
}

func AskQuestion(in io.Reader, out io.Writer, q Question) (string, error) {
	reader := bufio.NewReader(in)
	fmt.Fprintln(out)
	fmt.Fprintf(out, "%s\n", q.Prompt)

	switch q.Mode {
	case QuestionSingle:
		return askSingle(reader, out, q)
	case QuestionMulti:
		return askMulti(reader, out, q)
	default:
		return askText(reader, out, q)
	}
}

func askSingle(reader *bufio.Reader, out io.Writer, q Question) (string, error) {
	for i, option := range q.Options {
		fmt.Fprintf(out, "  %d. %s\n", i+1, option)
	}
	if q.AllowCustom {
		fmt.Fprintln(out, "  0. Custom answer...")
	}
	for {
		fmt.Fprint(out, "\nChoose one option: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		line = strings.TrimSpace(line)
		index, err := strconv.Atoi(line)
		if err == nil {
			if q.AllowCustom && index == 0 {
				return askText(reader, out, q)
			}
			if index >= 1 && index <= len(q.Options) {
				return q.Options[index-1], nil
			}
		}
		fmt.Fprintln(out, "Invalid choice. Enter the option number.")
	}
}

func askMulti(reader *bufio.Reader, out io.Writer, q Question) (string, error) {
	for i, option := range q.Options {
		fmt.Fprintf(out, "  [%d] %s\n", i+1, option)
	}
	if q.AllowCustom {
		fmt.Fprintln(out, "  [0] Custom answer...")
	}
	for {
		fmt.Fprint(out, "\nChoose one or more options (comma separated): ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		line = strings.TrimSpace(line)
		parts := strings.Split(line, ",")
		var selected []string
		custom := false
		valid := true
		seen := map[int]bool{}
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			index, err := strconv.Atoi(part)
			if err != nil {
				valid = false
				break
			}
			if q.AllowCustom && index == 0 {
				custom = true
				continue
			}
			if index < 1 || index > len(q.Options) {
				valid = false
				break
			}
			if !seen[index] {
				seen[index] = true
				selected = append(selected, q.Options[index-1])
			}
		}
		if !valid {
			fmt.Fprintln(out, "Invalid selection. Use comma-separated numbers.")
			continue
		}
		if len(selected) < max(1, q.MinSelect) && !custom {
			fmt.Fprintf(out, "Select at least %d option(s).\n", max(1, q.MinSelect))
			continue
		}
		if q.MaxSelect > 0 && len(selected) > q.MaxSelect {
			fmt.Fprintf(out, "Select at most %d option(s).\n", q.MaxSelect)
			continue
		}
		if custom {
			customAnswer, err := askText(reader, out, q)
			if err != nil {
				return "", err
			}
			if strings.TrimSpace(customAnswer) != "" {
				selected = append(selected, customAnswer)
			}
		}
		return strings.Join(selected, ", "), nil
	}
}

func askText(reader *bufio.Reader, out io.Writer, q Question) (string, error) {
	label := "Your answer"
	if strings.TrimSpace(q.Placeholder) != "" {
		label = q.Placeholder
	}
	fmt.Fprintf(out, "\n%s: ", label)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
