package main

const localUnversionedCommit = "local-unversioned"

type SourceDocumentReference struct {
	Repository string `json:"repository"`
	Ref        string `json:"ref"`
	Path       string `json:"path"`
}
