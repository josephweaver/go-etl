package main

import (
	"sort"
	"strings"
)

type Redactor struct {
	entries []redactorEntry
	next    int
}

type redactorEntry struct {
	secret string
	label  string
	order  int
}

func NewRedactor() *Redactor {
	return &Redactor{}
}

func (r *Redactor) Register(value SensitiveValue) {
	if r == nil {
		return
	}
	r.RegisterPlaintext(value.Plaintext(), value.RedactionLabel())
}

func (r *Redactor) RegisterPlaintext(secret, label string) {
	if r == nil || secret == "" {
		return
	}
	if label == "" {
		label = sensitiveValueRedactedLabel
	}
	for _, entry := range r.entries {
		if entry.secret == secret {
			return
		}
	}
	r.entries = append(r.entries, redactorEntry{
		secret: secret,
		label:  label,
		order:  r.next,
	})
	r.next++
}

func (r *Redactor) RedactString(text string) string {
	if r == nil || len(r.entries) == 0 || text == "" {
		return text
	}

	entries := append([]redactorEntry(nil), r.entries...)
	sort.SliceStable(entries, func(i, j int) bool {
		if len(entries[i].secret) == len(entries[j].secret) {
			return entries[i].order < entries[j].order
		}
		return len(entries[i].secret) > len(entries[j].secret)
	})

	redacted := text
	for _, entry := range entries {
		redacted = strings.ReplaceAll(redacted, entry.secret, entry.label)
	}
	return redacted
}

func (r *Redactor) RedactBytes(data []byte) []byte {
	if r == nil || len(r.entries) == 0 || len(data) == 0 {
		return data
	}
	return []byte(r.RedactString(string(data)))
}

func (r Redactor) MarshalJSON() ([]byte, error) {
	return []byte(`{}`), nil
}
