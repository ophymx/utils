package prompter

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"
)

var (
	// ErrTooManyDefaultValues is returned when more than one optional default
	// value is provided.
	ErrTooManyDefaultValues = errors.New("expected at most one default value")
	// ErrInvalidDefaultChoice is returned when a default choice is not present in
	// the choices set.
	ErrInvalidDefaultChoice = errors.New("invalid default choice")
	// ErrEmptyChoices is returned when Choices is called with no choices.
	ErrEmptyChoices = errors.New("empty choices")
	// ErrSecretRequiresTTY is returned when secret input requires a terminal but
	// stdin is not a TTY.
	ErrSecretRequiresTTY = errors.New("secret input requires TTY")
	// ErrNilPrompter is returned when a nil Prompter is provided.
	ErrNilPrompter = errors.New("nil prompter")
	// ErrNilParser is returned when a nil parse function is provided.
	ErrNilParser = errors.New("nil parser")
	// ErrNilStdin is returned when NewWithIO receives a nil stdin reader.
	ErrNilStdin = errors.New("nil stdin")
	// ErrNilStdout is returned when NewWithIO receives a nil stdout writer.
	ErrNilStdout = errors.New("nil stdout")
	// ErrInputTooLong is returned when a single input line exceeds the maximum
	// accepted byte length.
	ErrInputTooLong = errors.New("input too long")
)

// maxInputLineBytes is the maximum accepted byte length for one logical input line.
const maxInputLineBytes = 64 * 1024

// ParseFunc parses a string input into a typed value.
type ParseFunc[T any] func(string) (T, error)

type Prompter interface {
	// String prompts for a string value.
	// If the entered value is empty (after TrimSpace) and a default is provided,
	// the default is returned.
	String(prompt string, defaultValue ...string) (string, error)
	// SecretBytes prompts for secret input and returns it as bytes.
	// On TTY stdin, input is read via term.ReadPassword (no echo).
	// If requireTTY is true and stdin is not a TTY, ErrSecretRequiresTTY is returned.
	// If requireTTY is false and stdin is not a TTY, one line is read up to
	// maxInputLineBytes. In this mode, bufio.Reader internal buffers may retain
	// secret data.
	SecretBytes(prompt string, requireTTY bool) ([]byte, error)
	// Choices prompts until the input matches one of the provided choices.
	// If the entered value is empty (after TrimSpace) and a default is provided,
	// the default is returned.
	Choices(prompt string, choices []string, defaultValue ...string) (string, error)
	// Int64 prompts for a signed 64-bit integer.
	Int64(prompt string, defaultValue ...int64) (int64, error)
	// Float64 prompts for a float64.
	Float64(prompt string, defaultValue ...float64) (float64, error)
	// Duration prompts for a time.Duration.
	Duration(prompt string, defaultValue ...time.Duration) (time.Duration, error)
}

// PromptValue prompts for a string and converts it to T via parse.
// If input is empty (after TrimSpace) and a default is provided, the default is
// returned and parse is not called.
func PromptValue[T any](p Prompter, prompt string, parse ParseFunc[T], defaultValue ...T) (T, error) {
	var zero T

	if p == nil {
		return zero, ErrNilPrompter
	}
	if parse == nil {
		return zero, ErrNilParser
	}
	if len(defaultValue) > 1 {
		return zero, ErrTooManyDefaultValues
	}

	input, err := p.String(prompt)
	if err != nil {
		return zero, err
	}
	if strings.TrimSpace(input) == "" && len(defaultValue) == 1 {
		return defaultValue[0], nil
	}

	return parse(input)
}

// ParseBytesFunc parses secret byte input into a typed value.
type ParseBytesFunc[T any] func([]byte) (T, error)

// PromptSecretBytes prompts for secret input and parses it with parse.
// It uses SecretBytes with requireTTY set to false.
// The returned secret byte slice is zeroed after parse returns.
func PromptSecretBytes[T any](p Prompter, prompt string, parse ParseBytesFunc[T]) (T, error) {
	var zero T

	if p == nil {
		return zero, ErrNilPrompter
	}
	if parse == nil {
		return zero, ErrNilParser
	}

	secret, err := p.SecretBytes(prompt, false)
	if err != nil {
		return zero, err
	}
	defer zeroBytes(secret)

	return parse(secret)
}

type prompter struct {
	stdin  io.Reader
	stdout io.Writer
	reader *bufio.Reader
}

// NewStdio creates a Prompter backed by os.Stdin and os.Stdout.
// This is the recommended constructor for interactive terminal use.
func NewStdio() (Prompter, error) {
	return NewWithIO(os.Stdin, os.Stdout)
}

// NewWithIO creates a Prompter from custom input and output streams.
// This is useful for tests and non-standard I/O wiring.
func NewWithIO(stdin io.Reader, stdout io.Writer) (Prompter, error) {
	if stdin == nil {
		return nil, ErrNilStdin
	}
	if stdout == nil {
		return nil, ErrNilStdout
	}

	return &prompter{
		stdin:  stdin,
		stdout: stdout,
		reader: bufio.NewReader(stdin),
	}, nil
}

func (p *prompter) String(prompt string, defaultValue ...string) (string, error) {
	if len(defaultValue) > 1 {
		return "", ErrTooManyDefaultValues
	}

	if err := p.writePrompt(prompt); err != nil {
		return "", err
	}

	line, err := readLineLimited(p.reader, maxInputLineBytes)
	if err != nil {
		return "", err
	}

	value := strings.TrimRight(string(line), "\r\n")
	if strings.TrimSpace(value) == "" && len(defaultValue) == 1 {
		return defaultValue[0], nil
	}

	return value, nil
}

func (p *prompter) asTerm() (int, bool) {
	if file, ok := p.stdin.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		return int(file.Fd()), true
	}
	return 0, false
}

func (p *prompter) SecretBytes(prompt string, requireTTY bool) ([]byte, error) {
	if err := p.writePrompt(prompt); err != nil {
		return nil, err
	}

	if fd, ok := p.asTerm(); ok {
		secret, err := term.ReadPassword(fd)
		_, _ = fmt.Fprintln(p.stdout)
		if err != nil {
			return nil, err
		}
		return secret, nil
	}

	if requireTTY {
		return nil, ErrSecretRequiresTTY
	}

	// Non-TTY path: warn that bufio buffer may retain secret data
	slog.Warn("reading secret from non-TTY input; bufio buffer may retain secret data")

	line, err := readLineLimited(p.reader, maxInputLineBytes)
	if err != nil {
		return nil, err
	}

	for len(line) > 0 {
		last := line[len(line)-1]
		if last != '\n' && last != '\r' {
			break
		}
		line = line[:len(line)-1]
	}

	return line, nil
}

func (p *prompter) Choices(prompt string, choices []string, defaultValue ...string) (string, error) {
	if len(defaultValue) > 1 {
		return "", ErrTooManyDefaultValues
	}
	if len(choices) == 0 {
		return "", ErrEmptyChoices
	}

	choiceSet := make(map[string]struct{}, len(choices))
	for _, c := range choices {
		choiceSet[c] = struct{}{}
	}

	if len(defaultValue) == 1 {
		if _, ok := choiceSet[defaultValue[0]]; !ok {
			return "", fmt.Errorf("%w: %q", ErrInvalidDefaultChoice, defaultValue[0])
		}
	}

	for {
		value, err := p.String(prompt)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(value) == "" && len(defaultValue) == 1 {
			return defaultValue[0], nil
		}
		if _, ok := choiceSet[value]; ok {
			return value, nil
		}
		if _, err := fmt.Fprintf(p.stdout, "invalid choice: %q\n", value); err != nil {
			return "", err
		}
	}
}

func (p *prompter) Int64(prompt string, defaultValue ...int64) (int64, error) {
	if len(defaultValue) > 1 {
		return 0, ErrTooManyDefaultValues
	}

	value, err := p.String(prompt)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(value) == "" && len(defaultValue) == 1 {
		return defaultValue[0], nil
	}
	return strconv.ParseInt(strings.TrimSpace(value), 10, 64)
}

func (p *prompter) Float64(prompt string, defaultValue ...float64) (float64, error) {
	if len(defaultValue) > 1 {
		return 0, ErrTooManyDefaultValues
	}

	value, err := p.String(prompt)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(value) == "" && len(defaultValue) == 1 {
		return defaultValue[0], nil
	}
	return strconv.ParseFloat(strings.TrimSpace(value), 64)
}

func (p *prompter) Duration(prompt string, defaultValue ...time.Duration) (time.Duration, error) {
	if len(defaultValue) > 1 {
		return 0, ErrTooManyDefaultValues
	}

	value, err := p.String(prompt)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(value) == "" && len(defaultValue) == 1 {
		return defaultValue[0], nil
	}
	return time.ParseDuration(strings.TrimSpace(value))
}

func (p *prompter) writePrompt(prompt string) error {
	if prompt == "" {
		return nil
	}
	_, err := io.WriteString(p.stdout, prompt)
	return err
}

func zeroBytes(v []byte) {
	for i := range v {
		v[i] = 0
	}
}

// readLineLimited reads one logical line (including optional trailing '\n') with
// a hard byte cap. It returns io.EOF only when no bytes are read.
func readLineLimited(r *bufio.Reader, maxBytes int) ([]byte, error) {
	line := make([]byte, 0, 128)

	for {
		fragment, err := r.ReadSlice('\n')
		if len(fragment) > 0 {
			if len(line)+len(fragment) > maxBytes {
				return nil, ErrInputTooLong
			}
			line = append(line, fragment...)
		}

		switch {
		case err == nil:
			return line, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF):
			if len(line) == 0 {
				return nil, io.EOF
			}
			return line, nil
		default:
			return nil, err
		}
	}
}
