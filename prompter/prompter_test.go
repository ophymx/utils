package prompter

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

func newTestPrompter(t *testing.T, input string) (Prompter, *bytes.Buffer) {
	t.Helper()

	var out bytes.Buffer
	p, err := NewWithIO(strings.NewReader(input), &out)
	if err != nil {
		t.Fatalf("NewWithIO() error = %v", err)
	}
	return p, &out
}

func TestNewWithIO_Validation(t *testing.T) {
	_, err := NewWithIO(nil, io.Discard)
	if !errors.Is(err, ErrNilStdin) {
		t.Fatalf("expected ErrNilStdin, got %v", err)
	}

	_, err = NewWithIO(strings.NewReader(""), nil)
	if !errors.Is(err, ErrNilStdout) {
		t.Fatalf("expected ErrNilStdout, got %v", err)
	}
}

func TestString_DefaultAndEOFBehavior(t *testing.T) {
	t.Run("default on empty line", func(t *testing.T) {
		p, _ := newTestPrompter(t, "\n")
		got, err := p.String("Name: ", "guest")
		if err != nil {
			t.Fatalf("String() error = %v", err)
		}
		if got != "guest" {
			t.Fatalf("String() = %q, want %q", got, "guest")
		}
	})

	t.Run("returns line without trailing CRLF", func(t *testing.T) {
		p, _ := newTestPrompter(t, " value \r\n")
		got, err := p.String("Name: ")
		if err != nil {
			t.Fatalf("String() error = %v", err)
		}
		if got != " value " {
			t.Fatalf("String() = %q, want %q", got, " value ")
		}
	})

	t.Run("EOF with partial line", func(t *testing.T) {
		p, _ := newTestPrompter(t, "partial")
		got, err := p.String("Name: ")
		if err != nil {
			t.Fatalf("String() error = %v", err)
		}
		if got != "partial" {
			t.Fatalf("String() = %q, want %q", got, "partial")
		}
	})

	t.Run("EOF with no data", func(t *testing.T) {
		p, _ := newTestPrompter(t, "")
		_, err := p.String("Name: ")
		if !errors.Is(err, io.EOF) {
			t.Fatalf("expected io.EOF, got %v", err)
		}
	})

	t.Run("too many defaults", func(t *testing.T) {
		p, _ := newTestPrompter(t, "")
		_, err := p.String("Name: ", "a", "b")
		if !errors.Is(err, ErrTooManyDefaultValues) {
			t.Fatalf("expected ErrTooManyDefaultValues, got %v", err)
		}
	})
}

func TestPromptValue(t *testing.T) {
	_, err := PromptValue[int](nil, "v: ", func(s string) (int, error) { return 0, nil })
	if !errors.Is(err, ErrNilPrompter) {
		t.Fatalf("expected ErrNilPrompter, got %v", err)
	}

	p, _ := newTestPrompter(t, "\n")
	_, err = PromptValue[int](p, "v: ", nil)
	if !errors.Is(err, ErrNilParser) {
		t.Fatalf("expected ErrNilParser, got %v", err)
	}

	_, err = PromptValue[int](p, "v: ", func(s string) (int, error) { return 0, nil }, 1, 2)
	if !errors.Is(err, ErrTooManyDefaultValues) {
		t.Fatalf("expected ErrTooManyDefaultValues, got %v", err)
	}

	called := false
	got, err := PromptValue[int](p, "v: ", func(s string) (int, error) {
		called = true
		return 0, nil
	}, 42)
	if err != nil {
		t.Fatalf("PromptValue() error = %v", err)
	}
	if called {
		t.Fatalf("parser should not be called when default is used")
	}
	if got != 42 {
		t.Fatalf("PromptValue() = %d, want %d", got, 42)
	}

	p2, _ := newTestPrompter(t, "7\n")
	got, err = PromptValue[int](p2, "v: ", func(s string) (int, error) {
		if strings.TrimSpace(s) == "7" {
			return 7, nil
		}
		return 0, fmt.Errorf("unexpected input")
	})
	if err != nil {
		t.Fatalf("PromptValue() error = %v", err)
	}
	if got != 7 {
		t.Fatalf("PromptValue() = %d, want %d", got, 7)
	}
}

func TestPromptSecretBytes(t *testing.T) {
	_, err := PromptSecretBytes[string](nil, "secret: ", func(b []byte) (string, error) {
		return string(b), nil
	})
	if !errors.Is(err, ErrNilPrompter) {
		t.Fatalf("expected ErrNilPrompter, got %v", err)
	}

	p, _ := newTestPrompter(t, "secret\n")
	_, err = PromptSecretBytes[string](p, "secret: ", nil)
	if !errors.Is(err, ErrNilParser) {
		t.Fatalf("expected ErrNilParser, got %v", err)
	}

	var seen []byte
	res, err := PromptSecretBytes[string](p, "secret: ", func(b []byte) (string, error) {
		seen = b
		return string(b), nil
	})
	if err != nil {
		t.Fatalf("PromptSecretBytes() error = %v", err)
	}
	if res != "secret" {
		t.Fatalf("PromptSecretBytes() = %q, want %q", res, "secret")
	}
	for i, v := range seen {
		if v != 0 {
			t.Fatalf("secret byte at index %d was not zeroed", i)
		}
	}
}

func TestSecretBytes_NonTTY(t *testing.T) {
	t.Run("reads line and trims CRLF", func(t *testing.T) {
		p, out := newTestPrompter(t, "s3cr3t\r\n")
		got, err := p.SecretBytes("Password: ", false)
		if err != nil {
			t.Fatalf("SecretBytes() error = %v", err)
		}
		if string(got) != "s3cr3t" {
			t.Fatalf("SecretBytes() = %q, want %q", string(got), "s3cr3t")
		}
		if out.String() != "Password: " {
			t.Fatalf("prompt output = %q, want %q", out.String(), "Password: ")
		}
	})

	t.Run("EOF with partial line", func(t *testing.T) {
		p, _ := newTestPrompter(t, "s3cr3t")
		got, err := p.SecretBytes("Password: ", false)
		if err != nil {
			t.Fatalf("SecretBytes() error = %v", err)
		}
		if string(got) != "s3cr3t" {
			t.Fatalf("SecretBytes() = %q, want %q", string(got), "s3cr3t")
		}
	})

	t.Run("EOF with no data", func(t *testing.T) {
		p, _ := newTestPrompter(t, "")
		_, err := p.SecretBytes("Password: ", false)
		if !errors.Is(err, io.EOF) {
			t.Fatalf("expected io.EOF, got %v", err)
		}
	})

	t.Run("require TTY rejects non-TTY", func(t *testing.T) {
		p, _ := newTestPrompter(t, "s3cr3t\n")
		_, err := p.SecretBytes("Password: ", true)
		if !errors.Is(err, ErrSecretRequiresTTY) {
			t.Fatalf("expected ErrSecretRequiresTTY, got %v", err)
		}
	})

	t.Run("rejects oversized input", func(t *testing.T) {
		p, _ := newTestPrompter(t, strings.Repeat("x", maxInputLineBytes+1))
		_, err := p.SecretBytes("Password: ", false)
		if !errors.Is(err, ErrInputTooLong) {
			t.Fatalf("expected ErrInputTooLong, got %v", err)
		}
	})
}

func TestString_RejectsOversizedInput(t *testing.T) {
	p, _ := newTestPrompter(t, strings.Repeat("x", maxInputLineBytes+1))
	_, err := p.String("Name: ")
	if !errors.Is(err, ErrInputTooLong) {
		t.Fatalf("expected ErrInputTooLong, got %v", err)
	}
}

func TestChoices(t *testing.T) {
	t.Run("empty choices", func(t *testing.T) {
		p, _ := newTestPrompter(t, "anything\n")
		_, err := p.Choices("Mode: ", nil)
		if !errors.Is(err, ErrEmptyChoices) {
			t.Fatalf("expected ErrEmptyChoices, got %v", err)
		}
	})

	t.Run("too many defaults", func(t *testing.T) {
		p, _ := newTestPrompter(t, "")
		_, err := p.Choices("Mode: ", []string{"a", "b"}, "a", "b")
		if !errors.Is(err, ErrTooManyDefaultValues) {
			t.Fatalf("expected ErrTooManyDefaultValues, got %v", err)
		}
	})

	t.Run("invalid default choice", func(t *testing.T) {
		p, _ := newTestPrompter(t, "")
		_, err := p.Choices("Mode: ", []string{"a", "b"}, "c")
		if !errors.Is(err, ErrInvalidDefaultChoice) {
			t.Fatalf("expected ErrInvalidDefaultChoice, got %v", err)
		}
	})

	t.Run("uses default on blank input", func(t *testing.T) {
		p, _ := newTestPrompter(t, "\n")
		got, err := p.Choices("Mode: ", []string{"a", "b"}, "b")
		if err != nil {
			t.Fatalf("Choices() error = %v", err)
		}
		if got != "b" {
			t.Fatalf("Choices() = %q, want %q", got, "b")
		}
	})

	t.Run("re-prompts after invalid input", func(t *testing.T) {
		p, out := newTestPrompter(t, "x\na\n")
		got, err := p.Choices("Mode: ", []string{"a", "b"})
		if err != nil {
			t.Fatalf("Choices() error = %v", err)
		}
		if got != "a" {
			t.Fatalf("Choices() = %q, want %q", got, "a")
		}
		if !strings.Contains(out.String(), "invalid choice: \"x\"\n") {
			t.Fatalf("expected invalid-choice message in output, got %q", out.String())
		}
	})

	t.Run("returns EOF when input exhausted", func(t *testing.T) {
		p, _ := newTestPrompter(t, "x\n")
		_, err := p.Choices("Mode: ", []string{"a", "b"})
		if !errors.Is(err, io.EOF) {
			t.Fatalf("expected io.EOF, got %v", err)
		}
	})
}

func TestNumericAndDurationPrompts(t *testing.T) {
	t.Run("Int64", func(t *testing.T) {
		p, _ := newTestPrompter(t, " 123 \n")
		got, err := p.Int64("Count: ")
		if err != nil {
			t.Fatalf("Int64() error = %v", err)
		}
		if got != 123 {
			t.Fatalf("Int64() = %d, want %d", got, 123)
		}
	})

	t.Run("Int64 default", func(t *testing.T) {
		p, _ := newTestPrompter(t, "\n")
		got, err := p.Int64("Count: ", 9)
		if err != nil {
			t.Fatalf("Int64() error = %v", err)
		}
		if got != 9 {
			t.Fatalf("Int64() = %d, want %d", got, 9)
		}
	})

	t.Run("Float64", func(t *testing.T) {
		p, _ := newTestPrompter(t, " 1.5 \n")
		got, err := p.Float64("Ratio: ")
		if err != nil {
			t.Fatalf("Float64() error = %v", err)
		}
		if got != 1.5 {
			t.Fatalf("Float64() = %f, want %f", got, 1.5)
		}
	})

	t.Run("Float64 default", func(t *testing.T) {
		p, _ := newTestPrompter(t, "\n")
		got, err := p.Float64("Ratio: ", 2.5)
		if err != nil {
			t.Fatalf("Float64() error = %v", err)
		}
		if got != 2.5 {
			t.Fatalf("Float64() = %f, want %f", got, 2.5)
		}
	})

	t.Run("Duration", func(t *testing.T) {
		p, _ := newTestPrompter(t, " 1500ms \n")
		got, err := p.Duration("Timeout: ")
		if err != nil {
			t.Fatalf("Duration() error = %v", err)
		}
		if got != 1500*time.Millisecond {
			t.Fatalf("Duration() = %v, want %v", got, 1500*time.Millisecond)
		}
	})

	t.Run("Duration default", func(t *testing.T) {
		p, _ := newTestPrompter(t, "\n")
		want := 2 * time.Second
		got, err := p.Duration("Timeout: ", want)
		if err != nil {
			t.Fatalf("Duration() error = %v", err)
		}
		if got != want {
			t.Fatalf("Duration() = %v, want %v", got, want)
		}
	})
}
