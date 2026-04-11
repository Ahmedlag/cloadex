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

// ModeCycleHandler advances the current interactive mode.
type ModeCycleHandler func() error

// PromptRenderer returns the current prompt label.
type PromptRenderer func() string

// Session holds the state for an interactive cloadex session.
type Session struct {
	In  io.Reader
	Out io.Writer

	OnPrompt  PromptHandler
	OnResume  ResumeHandler
	OnCommand CommandHandler
	OnCycleMode ModeCycleHandler

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

	if file, ok := in.(*os.File); ok && IsInteractive(file) {
		if err := s.runInteractive(file, out); err == nil {
			return nil
		}
	}

	return s.runScanner(in, out)
}

func (s *Session) runScanner(in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	for {
		fmt.Fprint(out, s.prompt())

		if !scanner.Scan() {
			fmt.Fprintln(out)
			return scanner.Err()
		}

		if done, err := s.handleLine(out, scanner.Text()); done {
			return err
		}
	}
}

func (s *Session) runInteractive(in *os.File, out io.Writer) error {
	restore, err := enableRawInput(in)
	if err != nil {
		return err
	}
	defer restore()

	var line []byte
	for {
		renderInput(out, s.prompt(), string(line))

		var buf [1]byte
		n, err := in.Read(buf[:])
		if err != nil {
			fmt.Fprintln(out)
			return nil
		}
		if n == 0 {
			continue
		}

		switch buf[0] {
		case '\r', '\n':
			fmt.Fprint(out, "\r\n")
			if done, err := s.handleLine(out, string(line)); done {
				return err
			}
			line = line[:0]
		case 127, '\b':
			if len(line) > 0 {
				line = line[:len(line)-1]
			}
		case 27:
			seq, readErr := readEscapeSequence(in)
			if readErr != nil {
				continue
			}
			if seq == "[Z" && s.OnCycleMode != nil {
				if err := s.OnCycleMode(); err != nil {
					fmt.Fprintf(out, "\r\nMode error: %s\r\n", err)
				}
			}
		case 9:
			// Ignore plain tab for now.
		default:
			if buf[0] >= 32 {
				line = append(line, buf[0])
			}
		}
	}
}

func (s *Session) handleLine(out io.Writer, raw string) (bool, error) {
	line := strings.TrimSpace(raw)
	if line == "" {
		return false, nil
	}

	if isExitInput(line) {
		fmt.Fprintln(out, "Goodbye!")
		return true, nil
	}

	if strings.HasPrefix(line, "/") {
		command, args := parseCommand(line)
		switch command {
		case CmdExit:
			fmt.Fprintln(out, "Goodbye!")
			return true, nil
		case CmdHelp:
			printSessionHelp(out)
			return false, nil
		case CmdResume:
			if s.OnResume != nil {
				if err := s.OnResume(); err != nil {
					fmt.Fprintf(out, "Resume error: %s\n", err)
				}
			} else {
				fmt.Fprintln(out, "No resume handler configured.")
			}
			return false, nil
		default:
			if s.OnCommand != nil {
				if err := s.OnCommand(command, args); err != nil {
					fmt.Fprintf(out, "Command error: %s\n", err)
				}
			} else {
				fmt.Fprintf(out, "Unknown command: %s (type /help for available commands)\n", command)
			}
			return false, nil
		}
	}

	if s.OnPrompt != nil {
		if err := s.OnPrompt(line); err != nil {
			fmt.Fprintf(out, "Error: %s\n", err)
		}
	}
	return false, nil
}

func isExitInput(line string) bool {
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "exit", "quit":
		return true
	default:
		return false
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
	fmt.Fprintln(w, "  /mode <name>     Switch mode: chat, planning, execution")
	fmt.Fprintln(w, "  /plan [prompt]   Show last approved plan or run planning mode immediately")
	fmt.Fprintln(w, "  /run [prompt]    Execute directly or switch to execution mode")
	fmt.Fprintln(w, "  /review [prompt] Alias for read-only chat/review mode")
	fmt.Fprintln(w, "  /score           Show AI scores")
	fmt.Fprintln(w, "  /agents          Show agent labels and roles")
	fmt.Fprintln(w, "  /diff            Show current git diff summary")
	fmt.Fprintln(w, "  Shift-Tab        Cycle mode: chat -> planning -> execution")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Or type any prompt to continue the active session.")
}

func renderInput(out io.Writer, prompt string, line string) {
	fmt.Fprintf(out, "\r\033[K%s%s", prompt, line)
}

func readEscapeSequence(in *os.File) (string, error) {
	var seq [2]byte
	n, err := in.Read(seq[:])
	if err != nil {
		return "", err
	}
	return string(seq[:n]), nil
}
