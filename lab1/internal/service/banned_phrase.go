package service

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"lab1/internal/domain"
)

const maxPhraseLength = 50

type BannedPhraseService struct {
	phrases BannedPhraseRepository
}

func NewBannedPhraseService(phrases BannedPhraseRepository) *BannedPhraseService {
	return &BannedPhraseService{phrases: phrases}
}

func (s *BannedPhraseService) List(ctx context.Context) ([]domain.BannedPhrase, error) {
	return s.phrases.List(ctx)
}

// Add stores the phrase trimmed and lowercased so matching is case-insensitive.
func (s *BannedPhraseService) Add(ctx context.Context, in domain.BannedPhraseInput) (domain.BannedPhrase, error) {
	phrase := strings.ToLower(strings.TrimSpace(in.Phrase))
	if phrase == "" || utf8.RuneCountInString(phrase) > maxPhraseLength {
		return domain.BannedPhrase{}, &domain.ValidationError{Errors: []domain.FieldError{
			{Field: "phrase", Message: "must be between 1 and 50 characters long"},
		}}
	}

	p := domain.BannedPhrase{Phrase: phrase}
	if err := s.phrases.Create(ctx, &p); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return domain.BannedPhrase{}, &domain.ConflictError{FieldError: domain.FieldError{
				Field: "phrase", Message: "phrase is already banned",
			}}
		}
		return domain.BannedPhrase{}, err
	}
	return p, nil
}

func (s *BannedPhraseService) Delete(ctx context.Context, id int64) error {
	return s.phrases.Delete(ctx, id)
}
