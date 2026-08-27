// Command admin provides bounded operator-only maintenance workflows.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/CORTA-11/core-api/internal/identity"
	"github.com/CORTA-11/core-api/internal/repository/publicdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/term"
)

const maximumCLIInputBytes = 1024

var (
	errUsage            = errors.New("usage: admin user create --email <email> --display-name <name> [--password-stdin]")
	errDatabaseURL      = errors.New("DATABASE_URL is required")
	errTerminalRequired = errors.New("interactive password input requires a terminal")
	errStdinTerminal    = errors.New("--password-stdin refuses terminal input")
	errPasswordInput    = errors.New("password input must be one bounded line")
	errPasswordMismatch = errors.New("password confirmation does not match")
	errInvalidEmail     = errors.New("email is invalid")
	errInvalidName      = errors.New("display name is invalid")
	errInvalidPassword  = errors.New("password does not satisfy policy")
	errEmailExists      = errors.New("email already exists")
	errCreateFailed     = errors.New("could not create user")
)

type fileInput interface {
	io.Reader
	Fd() uintptr
}

type terminalAccess interface {
	IsTerminal(uintptr) bool
	ReadPassword(uintptr) ([]byte, error)
}

type systemTerminal struct{}

func (systemTerminal) IsTerminal(fd uintptr) bool { return term.IsTerminal(int(fd)) }

func (systemTerminal) ReadPassword(fd uintptr) ([]byte, error) {
	return term.ReadPassword(int(fd))
}

type accountCreateFunc func(
	context.Context,
	string,
	identity.CanonicalEmail,
	string,
	string,
) (uuid.UUID, error)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Getenv, os.Stdin, os.Stdout, os.Stderr, systemTerminal{}, createAccount); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(
	ctx context.Context,
	args []string,
	getenv func(string) string,
	input fileInput,
	output io.Writer,
	errorOutput io.Writer,
	terminal terminalAccess,
	create accountCreateFunc,
) error {
	if len(args) < 2 || args[0] != "user" || args[1] != "create" {
		return errUsage
	}
	for _, argument := range args[2:] {
		if strings.HasPrefix(argument, "--password") && argument != "--password-stdin" {
			return errUsage
		}
	}

	flags := flag.NewFlagSet("user create", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	emailValue := flags.String("email", "", "")
	displayNameValue := flags.String("display-name", "", "")
	passwordStdin := flags.Bool("password-stdin", false, "")
	if err := flags.Parse(args[2:]); err != nil || flags.NArg() != 0 || *emailValue == "" || *displayNameValue == "" {
		return errUsage
	}
	databaseURL := getenv("DATABASE_URL")
	if databaseURL == "" {
		return errDatabaseURL
	}

	canonicalEmail, err := (identity.EmailCanonicalizer{}).Canonicalize(*emailValue)
	if err != nil {
		return errInvalidEmail
	}
	displayName, err := identity.NormalizeDisplayName(*displayNameValue)
	if err != nil {
		return errInvalidName
	}

	passwordBytes, err := readPassword(input, errorOutput, terminal, *passwordStdin)
	if err != nil {
		return err
	}
	defer clearBytes(passwordBytes)
	normalizedPassword, err := (identity.PasswordPolicy{}).Normalize(string(passwordBytes))
	if err != nil {
		return errInvalidPassword
	}

	userID, err := create(ctx, databaseURL, canonicalEmail, displayName, normalizedPassword)
	if errors.Is(err, identity.ErrEmailAlreadyExists) {
		return errEmailExists
	}
	if err != nil {
		return errCreateFailed
	}
	_, _ = fmt.Fprintf(output, "created user %s\n", userID)
	return nil
}

func readPassword(input fileInput, output io.Writer, terminal terminalAccess, fromStdin bool) ([]byte, error) {
	if fromStdin {
		if terminal.IsTerminal(input.Fd()) {
			return nil, errStdinTerminal
		}
		return readPasswordLine(input)
	}
	if !terminal.IsTerminal(input.Fd()) {
		return nil, errTerminalRequired
	}
	_, _ = io.WriteString(output, "Password: ")
	first, err := terminal.ReadPassword(input.Fd())
	_, _ = io.WriteString(output, "\nConfirm password: ")
	if err != nil {
		clearBytes(first)
		return nil, errPasswordInput
	}
	second, secondErr := terminal.ReadPassword(input.Fd())
	_, _ = io.WriteString(output, "\n")
	defer clearBytes(second)
	if secondErr != nil || len(first) > maximumCLIInputBytes || len(second) > maximumCLIInputBytes {
		clearBytes(first)
		return nil, errPasswordInput
	}
	if !bytes.Equal(first, second) {
		clearBytes(first)
		return nil, errPasswordMismatch
	}
	return first, nil
}

func readPasswordLine(input io.Reader) ([]byte, error) {
	value, err := io.ReadAll(io.LimitReader(input, maximumCLIInputBytes+2))
	if err != nil || len(value) > maximumCLIInputBytes+1 {
		clearBytes(value)
		return nil, errPasswordInput
	}
	if bytes.HasSuffix(value, []byte{'\n'}) {
		value = value[:len(value)-1]
		if bytes.HasSuffix(value, []byte{'\r'}) {
			value = value[:len(value)-1]
		}
	}
	if len(value) > maximumCLIInputBytes || bytes.ContainsAny(value, "\r\n") {
		clearBytes(value)
		return nil, errPasswordInput
	}
	return value, nil
}

func clearBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}

func createAccount(
	ctx context.Context,
	databaseURL string,
	email identity.CanonicalEmail,
	displayName string,
	password string,
) (uuid.UUID, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return uuid.Nil, errCreateFailed
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return uuid.Nil, errCreateFailed
	}
	hasher, err := identity.NewPasswordHasher(identity.HashConfig{})
	if err != nil {
		return uuid.Nil, errCreateFailed
	}
	creator := identity.NewLocalAccountCreator(publicdb.New(pool), hasher)
	principal, err := creator.Create(ctx, email.Display, displayName, password)
	if err != nil {
		return uuid.Nil, err
	}
	return principal.UserPublicID, nil
}
