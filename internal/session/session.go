package session

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// SlashCommand represents a recognised slash command in the session loop.
type SlashCommand string

const (
	CmdExit   SlashCommand = "/exit"
	CmdHelp   SlashCommand = "/help"
	CmdResume SlashCommand = "/resume"
	CmdPlan   SlashCommand = "/plan"
	CmdRun    SlashCommand = "/run"
	CmdReview SlashCommand = "/review"
	CmdMode   SlashCommand = "/mode"
	CmdScore  SlashCommand = "/score"
	CmdAgents SlashCommand = "/agents"
	CmdDiff   SlashCommand = "/diff"
)

// PromptHandler is called for each non-slash-command prompt entered by the user.
type PromptHandler func(prompt string) error

// ResumeHandler is called when the user issues /resume in a session.
type ResumeHandler func() error

// CommandHandler handles custom slash commands.
type CommandHandler func(command SlashCommand, args string) error

// PromptRenderer returns the current prompt label.
type PromptRenderer func() string

// Session holds the state for an interactive cloadex session.
type Session struct {
	In  io.Reader
	Out io.Writer

	OnPrompt  PromptHandler
	OnResume  ResumeHandler
	OnCommand CommandHandler

	PromptRenderer PromptRenderer
	Header         string
}

// Run starts the interactive session loop, reading prompts until /exit or EOF.
func (s *Session) Run() error {
	in := s.In
	if in == nil {
		in = os.Stdin
	}
	out := s.Out
	if out == nil {
		out = os.Stdout
	}

	if s.Header != "" {
		fmt.Fprintln(out, s.Header)
	}

	scanner := bufio.NewScanner(in)
	for {
		fmt.Fprint(out, s.prompt())

		if !scanner.Scan() {
			fmt.Fprintln(out)
			return scanner.Err()
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "/") {
			command, args := parseCommand(line)
			switch command {
			case CmdExit:
				fmt.Fprintln(out, "Goodbye!")
				return nil
			case CmdHelp:
				printSessionHelp(out)
				continue
			case CmdResume:
				if s.OnResume != nil {
					if err := s.OnResume(); err != nil {
						fmt.Fprintf(out, "Resume error: %s\n", err)
					}
				} else {
					fmt.Fprintln(out, "No resume handler configured.")
				}
				continue
			default:
				if s.OnCommand != nil {
					if err := s.OnCommand(command, args); err != nil {
						fmt.Fprintf(out, "Command error: %s\n", err)
					}
				} else {
					fmt.Fprintf(out, "Unknown command: %s (type /help for available commands)\n", command)
				}
				continue
			}
		}

		if s.OnPrompt != nil {
			if err := s.OnPrompt(line); err != nil {
				fmt.Fprintf(out, "Error: %s\n", err)
			}
		}
	}
}

func (s *Session) prompt() string {
	if s.PromptRenderer != nil {
		return s.PromptRenderer()
	}
	return "cloadex> "
}

func parseCommand(line string) (SlashCommand, string) {
	fields := strings.Fields(line)
	command := SlashCommand(fields[0])
	if len(fields) <= 1 {
		return command, ""
	}
	return command, strings.TrimSpace(strings.TrimPrefix(line, fields[0]))
}

func printSessionHelp(w io.Writer) {
	fmt.Fprintln(w, "Available commands:")
	fmt.Fprintln(w, "  /help            Show this help message")
	fmt.Fprintln(w, "  /exit            Exit the session")
	fmt.Fprintln(w, "  /resume          Resume the most recent interrupted run")
	fmt.Fprintln(w, "  /mode <name>     Switch mode: chat, plan, run, review")
	fmt.Fprintln(w, "  /plan [prompt]   Show last approved plan or run plan mode immediately")
	fmt.Fprintln(w, "  /run [prompt]    Execute directly or switch to run mode")
	fmt.Fprintln(w, "  /review [prompt] Review the workspace or switch to review mode")
	fmt.Fprintln(w, "  /score           Show AI scores")
	fmt.Fprintln(w, "  /agents          Show agent labels and roles")
	fmt.Fprintln(w, "  /diff            Show current git diff summary")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Or type any prompt to continue the active session.")
}
