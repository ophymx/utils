# prompter

prompter is a small package for interactive CLI input.

## What It Provides

- String prompts with optional defaults.
- Choice prompts with validation.
- Int64, float64, and time.Duration prompts.
- Secret input with no echo on TTY.
- Generic helpers for custom parsing.

## Constructors

- NewStdio(): uses os.Stdin and os.Stdout (recommended for interactive CLI use).
- NewWithIO(stdin, stdout): uses custom streams (useful for tests and I/O wiring).

## Usage

```go
p, err := prompter.NewStdio()
if err != nil {
    return err
}

name, err := p.String("Name: ", "guest")
if err != nil {
    return err
}

timeout, err := p.Duration("Timeout: ", 2*time.Second)
if err != nil {
    return err
}

_ = name
_ = timeout
```

```go
mode, err := p.Choices("Mode [fast|safe]: ", []string{"fast", "safe"}, "safe")
if err != nil {
    return err
}

_ = mode
```

```go
port, err := prompter.PromptValue[int](p, "Port: ", func(s string) (int, error) {
    return strconv.Atoi(strings.TrimSpace(s))
}, 8080)
if err != nil {
    return err
}

_ = port
```

```go
token, err := prompter.PromptSecretBytes[string](p, "Token: ", func(b []byte) (string, error) {
    return strings.TrimSpace(string(b)), nil
})
if err != nil {
    return err
}

_ = token
```

## Security and Input Limits

- SecretBytes(prompt, requireTTY) uses term.ReadPassword when stdin is a TTY.
- If requireTTY is true and stdin is not a TTY, SecretBytes returns ErrSecretRequiresTTY.
- If requireTTY is false and stdin is not a TTY, SecretBytes reads one line from stdin and logs a warning; bufio internal buffers may retain secret data.
- PromptSecretBytes currently calls SecretBytes(prompt, false).
- PromptSecretBytes zeroes the returned secret byte slice after parsing.
- String and non-TTY SecretBytes enforce a per-line limit of 64 KiB; larger lines return ErrInputTooLong.

## Error Behavior

- ErrTooManyDefaultValues: more than one optional default was provided.
- ErrInvalidDefaultChoice: a default choice is not present in the choices list.
- ErrEmptyChoices: Choices was called with an empty choices list.
- ErrSecretRequiresTTY: secret input required a terminal but stdin is not a TTY.
- ErrInputTooLong: a single input line exceeded 64 KiB.
- ErrNilStdin and ErrNilStdout: invalid nil streams passed to NewWithIO.
- ErrNilPrompter and ErrNilParser: invalid nil arguments passed to helper functions.