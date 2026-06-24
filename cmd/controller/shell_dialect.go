package main

type ShellDialect interface {
	Newline() string
	QuoteArg(value string) string
	LocalizePath(value string) (string, error)
}

type ShellPlatform = ShellDialect
