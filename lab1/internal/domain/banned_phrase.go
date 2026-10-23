package domain

type BannedPhrase struct {
	ID     int64  `json:"id"`
	Phrase string `json:"phrase"`
}

type BannedPhraseInput struct {
	Phrase string `json:"phrase"`
}
