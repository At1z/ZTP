package unit

import (
	"errors"
	"strings"
	"testing"

	"lab1/internal/domain"
)

func TestPhraseIsNormalized(t *testing.T) {
	p, err := newEnv().phrases.Add(ctx, domain.BannedPhraseInput{Phrase: "  Spam "})
	if err != nil {
		t.Fatal(err)
	}
	if p.Phrase != "spam" {
		t.Fatalf("phrase = %q, want %q", p.Phrase, "spam")
	}
}

func TestInvalidPhrases(t *testing.T) {
	for name, phrase := range map[string]string{
		"empty":      "",
		"whitespace": "   ",
		"too long":   strings.Repeat("a", 51),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := newEnv().phrases.Add(ctx, domain.BannedPhraseInput{Phrase: phrase})
			expectInvalidField(t, err, "phrase")
		})
	}
}

func TestPhraseLengthCountsCharactersNotBytes(t *testing.T) {
	// 50 two-byte characters: valid, although it is 100 bytes long.
	if _, err := newEnv().phrases.Add(ctx, domain.BannedPhraseInput{Phrase: strings.Repeat("ż", 50)}); err != nil {
		t.Fatalf("50 characters should be accepted: %v", err)
	}
}

func TestDuplicatePhraseIsConflict(t *testing.T) {
	e := newEnv()
	e.phrases.Add(ctx, domain.BannedPhraseInput{Phrase: "spam"})

	_, err := e.phrases.Add(ctx, domain.BannedPhraseInput{Phrase: "SPAM"})
	var conflict *domain.ConflictError
	if !errors.As(err, &conflict) || conflict.Field != "phrase" {
		t.Fatalf("error = %v, want phrase conflict", err)
	}
}

func TestDeleteUnknownPhrase(t *testing.T) {
	if err := newEnv().phrases.Delete(ctx, 42); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}
