package session

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseQuestion(t *testing.T) {
	q, ok := ParseQuestion(`{"kind":"question","mode":"single","prompt":"Choose one","options":["A","B"],"allow_custom":true}`)
	if !ok || q == nil {
		t.Fatal("expected question payload to parse")
	}
	if q.Mode != QuestionSingle || q.Prompt != "Choose one" {
		t.Fatalf("unexpected question: %#v", q)
	}
}

func TestAskQuestionSingle(t *testing.T) {
	in := strings.NewReader("2\n")
	var out bytes.Buffer
	answer, err := AskQuestion(in, &out, Question{
		Kind:    "question",
		Mode:    QuestionSingle,
		Prompt:  "Choose one",
		Options: []string{"A", "B"},
	})
	if err != nil {
		t.Fatalf("AskQuestion: %v", err)
	}
	if answer != "B" {
		t.Fatalf("answer = %q, want B", answer)
	}
}

func TestAskQuestionMultiWithCustom(t *testing.T) {
	in := strings.NewReader("1,0\ncustom\n")
	var out bytes.Buffer
	answer, err := AskQuestion(in, &out, Question{
		Kind:        "question",
		Mode:        QuestionMulti,
		Prompt:      "Choose many",
		Options:     []string{"A", "B"},
		AllowCustom: true,
	})
	if err != nil {
		t.Fatalf("AskQuestion: %v", err)
	}
	if answer != "A, custom" {
		t.Fatalf("answer = %q, want %q", answer, "A, custom")
	}
}
