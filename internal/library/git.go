package library

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"unicode/utf8"
)

type Runner interface {
	Run(ctx context.Context, dir string, args ...string) (string, error)
}

type CommandRunner struct{}

var credentialURL = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^\s/@:]+(?::[^\s/@]+)?@`)
var anyURL = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^\s]+`)
var authorizationValue = regexp.MustCompile(`(?i)(authorization\s*[:=]\s*).*$`)

const maxGitDiagnosticBytes = 1024

func (CommandRunner) Run(ctx context.Context, dir string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		return "", gitDiagnosticError(args, err, string(output))
	}
	return string(output), nil
}

func gitDiagnosticError(args []string, cause error, output string) error {
	message := fmt.Sprintf("git %s: %v: %s", sanitizedArgs(args), cause, sanitizeOutput(output))
	return errors.New(truncateUTF8(message, maxGitDiagnosticBytes))
}

func sanitizeOutput(output string) string {
	return sanitizeGitDiagnostic(output)
}

func sanitizeGitDiagnostic(diagnostic string) string {
	diagnostic = credentialURL.ReplaceAllString(diagnostic, `${1}redacted@`)
	diagnostic = redactURLs(diagnostic)
	lines := strings.Split(diagnostic, "\n")
	for i := range lines {
		lines[i] = authorizationValue.ReplaceAllString(lines[i], `${1}redacted`)
	}
	return truncateUTF8(strings.TrimSpace(strings.Join(lines, "\n")), maxGitDiagnosticBytes)
}

func sanitizedArgs(args []string) string {
	redacted := append([]string(nil), args...)
	for i, argument := range redacted {
		if parsed, err := url.Parse(argument); err == nil && parsed.User != nil {
			parsed.User = url.User("redacted")
			argument = parsed.String()
		}
		argument = redactURLs(argument)
		argument = authorizationValue.ReplaceAllString(argument, `${1}redacted`)
		redacted[i] = argument
	}
	return truncateUTF8(strings.Join(redacted, " "), maxGitDiagnosticBytes)
}

func redactURLs(value string) string {
	return anyURL.ReplaceAllStringFunc(value, func(raw string) string {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "redacted-url"
		}
		if parsed.User != nil {
			parsed.User = url.User("redacted")
		}
		parsed.RawQuery = ""
		parsed.ForceQuery = false
		parsed.Fragment = ""
		return parsed.String()
	})
}

func truncateUTF8(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	value = value[:maximum]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
