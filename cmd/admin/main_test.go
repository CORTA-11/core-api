package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/CORTA-11/core-api/internal/identity"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testInput struct {
	*bytes.Reader
	fd uintptr
}

func (input *testInput) Fd() uintptr { return input.fd }

type terminalStub struct {
	isTerminal bool
	passwords  [][]byte
	readCalls  int
}

func (terminal *terminalStub) IsTerminal(uintptr) bool { return terminal.isTerminal }

func (terminal *terminalStub) ReadPassword(uintptr) ([]byte, error) {
	if terminal.readCalls >= len(terminal.passwords) {
		return nil, errors.New("unexpected terminal read")
	}
	password := append([]byte(nil), terminal.passwords[terminal.readCalls]...)
	terminal.readCalls++
	return password, nil
}

type createCapture struct {
	called      int
	email       identity.CanonicalEmail
	displayName string
	password    string
	err         error
}

func TestRunOwnerAssignUsesOnlyPublicIDsAndRedactsFailures(t *testing.T) {
	t.Parallel()
	organizationID := uuid.MustParse("16f71004-92bd-47dc-b083-203fbdd70f38")
	userID := uuid.MustParse("aaa0dd02-ac71-4cc8-96dd-b9782c1f7c9b")
	var calls int
	output := new(bytes.Buffer)
	err := runOwnerAssign(context.Background(), []string{
		"--org", organizationID.String(), "--user", userID.String(),
	}, func(string) string { return "database-url" }, output,
		func(_ context.Context, databaseURL string, gotOrganizationID, gotUserID uuid.UUID) error {
			calls++
			assert.Equal(t, "database-url", databaseURL)
			assert.Equal(t, organizationID, gotOrganizationID)
			assert.Equal(t, userID, gotUserID)
			return nil
		})
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.Equal(t, "assigned organization "+organizationID.String()+" user "+userID.String()+" role owner\n", output.String())
	assert.NotContains(t, output.String(), "org_id")

	secret := "database-internal-secret"
	err = runOwnerAssign(context.Background(), []string{
		"--org", organizationID.String(), "--user", userID.String(),
	}, func(string) string { return "database-url" }, new(bytes.Buffer),
		func(context.Context, string, uuid.UUID, uuid.UUID) error { return errors.New(secret) })
	assert.ErrorIs(t, err, errOwnerAssignFailed)
	assert.NotContains(t, err.Error(), secret)
}

func TestRunOwnerAssignRejectsInvalidOrAmbiguousTargets(t *testing.T) {
	t.Parallel()
	valid := uuid.NewString()
	for _, args := range [][]string{
		{"--org", "1", "--user", valid},
		{"--org", valid, "--user", "1"},
		{"--org", valid, "--user", valid, "extra"},
		{"--org", valid},
		{"--org", uuid.Nil.String(), "--user", valid},
	} {
		called := false
		err := runOwnerAssign(context.Background(), args, func(string) string { return "database-url" }, new(bytes.Buffer),
			func(context.Context, string, uuid.UUID, uuid.UUID) error { called = true; return nil })
		assert.ErrorIs(t, err, errOwnerUsage)
		assert.False(t, called)
	}
}

func (capture *createCapture) create(
	_ context.Context,
	_ string,
	email identity.CanonicalEmail,
	displayName string,
	password string,
) (uuid.UUID, error) {
	capture.called++
	capture.email = email
	capture.displayName = displayName
	capture.password = password
	return uuid.MustParse("b2f5628f-1028-4e2f-8c5f-2aac01f83f8c"), capture.err
}

func TestRunUserCreateRejectsSecretBearingAndExtraArguments(t *testing.T) {
	t.Parallel()

	secret := "secret-value-in-argv"
	tests := [][]string{
		{"user", "create", "--email", "user@example.com", "--display-name", "User", "--password", secret},
		{"user", "create", "--email", "user@example.com", "--display-name", "User", "--password=" + secret},
		{"user", "create", "--email", "user@example.com", "--display-name", "User", secret},
		{"user", "create", "--email", "user@example.com", "--display-name", "User", "extra", secret},
	}
	for _, args := range tests {
		capture := &createCapture{}
		err := run(context.Background(), args, func(string) string { return "database-url" },
			&testInput{Reader: bytes.NewReader(nil), fd: 10}, new(bytes.Buffer), new(bytes.Buffer),
			&terminalStub{isTerminal: true}, capture.create)
		assert.ErrorIs(t, err, errUsage)
		assert.NotContains(t, err.Error(), secret)
		assert.Zero(t, capture.called)
	}
}

func TestRunUserCreateReadsOneBoundedNonTerminalLine(t *testing.T) {
	t.Parallel()

	secret := "re\u0301search-password-value"
	input := &testInput{Reader: bytes.NewReader([]byte(secret + "\n")), fd: 11}
	output := new(bytes.Buffer)
	capture := &createCapture{}
	err := run(context.Background(), []string{
		"user", "create", "--email", " User@Example.COM ", "--display-name", " Research User ", "--password-stdin",
	}, func(name string) string {
		if name == "DATABASE_URL" {
			return "database-url"
		}
		return ""
	}, input, output, new(bytes.Buffer), &terminalStub{isTerminal: false}, capture.create)
	require.NoError(t, err)
	assert.Equal(t, 1, capture.called)
	assert.Equal(t, identity.CanonicalEmail{Display: "User@Example.COM", Key: "user@example.com"}, capture.email)
	assert.Equal(t, "Research User", capture.displayName)
	assert.Equal(t, "r\u00e9search-password-value", capture.password)
	assert.Contains(t, output.String(), "b2f5628f-1028-4e2f-8c5f-2aac01f83f8c")
	assert.NotContains(t, output.String(), secret)
	assert.NotContains(t, output.String(), "user@example.com")
}

func TestRunUserCreateRejectsTerminalAndInvalidLinesInStdinMode(t *testing.T) {
	t.Parallel()

	baseArgs := []string{"user", "create", "--email", "user@example.com", "--display-name", "User", "--password-stdin"}
	for _, test := range []struct {
		name       string
		input      string
		isTerminal bool
	}{
		{name: "terminal", input: "valid-password-value\n", isTerminal: true},
		{name: "multiple lines", input: "valid-password-value\nsecond-secret-value\n"},
		{name: "oversized", input: strings.Repeat("a", 1025)},
	} {
		t.Run(test.name, func(t *testing.T) {
			capture := &createCapture{}
			err := run(context.Background(), baseArgs, func(string) string { return "database-url" },
				&testInput{Reader: bytes.NewReader([]byte(test.input)), fd: 12}, new(bytes.Buffer), new(bytes.Buffer),
				&terminalStub{isTerminal: test.isTerminal}, capture.create)
			assert.Error(t, err)
			assert.NotContains(t, err.Error(), test.input)
			assert.Zero(t, capture.called)
		})
	}
}

func TestRunUserCreatePromptsTwiceWithoutEchoAndRejectsMismatch(t *testing.T) {
	t.Parallel()

	first := []byte("first-password-value")
	second := []byte("second-password-value")
	terminal := &terminalStub{isTerminal: true, passwords: [][]byte{first, second}}
	output := new(bytes.Buffer)
	errorsOutput := new(bytes.Buffer)
	capture := &createCapture{}
	err := run(context.Background(), []string{
		"user", "create", "--email", "user@example.com", "--display-name", "User",
	}, func(string) string { return "database-url" },
		&testInput{Reader: bytes.NewReader(nil), fd: 13}, output, errorsOutput, terminal, capture.create)
	assert.ErrorIs(t, err, errPasswordMismatch)
	assert.Equal(t, 2, terminal.readCalls)
	assert.Zero(t, capture.called)
	assert.NotContains(t, errorsOutput.String(), string(first))
	assert.NotContains(t, errorsOutput.String(), string(second))
}

func TestRunUserCreateRedactsCreatorFailure(t *testing.T) {
	t.Parallel()

	secret := "creator-secret-value"
	capture := &createCapture{err: errors.New(secret)}
	err := run(context.Background(), []string{
		"user", "create", "--email", "user@example.com", "--display-name", "User", "--password-stdin",
	}, func(string) string { return "database-url" },
		&testInput{Reader: bytes.NewReader([]byte("valid-password-value\n")), fd: 14},
		new(bytes.Buffer), new(bytes.Buffer), &terminalStub{}, capture.create)
	assert.ErrorIs(t, err, errCreateFailed)
	assert.NotContains(t, err.Error(), secret)
}
